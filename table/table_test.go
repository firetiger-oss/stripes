package table

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestHeaderFromName(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{"Name", "NAME"},
		{"FirstName", "FIRST NAME"},
		{"CreatedAt", "CREATED AT"},
		{"UserID", "USER ID"},
		{"HTTPRequest", "HTTP REQUEST"},
		{"URL", "URL"},
		{"ID", "ID"},
		{"V2Token", "V 2 TOKEN"},
	}
	for _, c := range cases {
		if got := headerFromName(c.in); got != c.out {
			t.Errorf("headerFromName(%q) = %q, want %q", c.in, got, c.out)
		}
	}
}

func TestRenderBasicStruct(t *testing.T) {
	type Row struct {
		Name string
		Age  int
		City string
	}
	got := render([]Row{
		{Name: "Alice", Age: 30, City: "NYC"},
		{Name: "Bob", Age: 25, City: "LA"},
	})
	want := "NAME   AGE  CITY\nAlice   30  NYC \nBob     25  LA  "
	equal(t, got, want)
}

func TestRenderTimeAbsolute(t *testing.T) {
	type Row struct {
		Created time.Time
		Note    string
	}
	created := time.Date(2026, 5, 11, 10, 23, 45, 0, time.UTC)
	got := render([]Row{{Created: created, Note: "ok"}})
	want := "CREATED              NOTE\n2026-05-11 10:23:45  ok  "
	equal(t, got, want)
}

func TestRenderTimeAbsoluteZeroValue(t *testing.T) {
	type Row struct {
		Created time.Time
		Note    string
	}
	got := render([]Row{{Note: "missing"}})
	want := "CREATED  NOTE   \n         missing"
	equal(t, got, want)
}

func TestRenderTimeRelative(t *testing.T) {
	type Row struct {
		Created time.Time
		Note    string
	}
	now := time.Date(2026, 5, 11, 10, 30, 0, 0, time.UTC)
	rows := []Row{
		{Created: now.Add(-5 * time.Minute), Note: "a"},
		{Created: now.Add(-3 * time.Hour), Note: "b"},
		{Created: now.Add(-5 * 24 * time.Hour), Note: "c"},
	}
	got := render(rows, WithNow(func() time.Time { return now }))
	want := "CREATED  NOTE\n5m ago   a   \n3h ago   b   \n5d ago   c   "
	equal(t, got, want)
}

func TestRenderDuration(t *testing.T) {
	type Row struct {
		Step    string
		Latency time.Duration
	}
	got := render([]Row{
		{Step: "a", Latency: 12 * time.Millisecond},
		{Step: "b", Latency: 250 * time.Microsecond},
		{Step: "c", Latency: 5 * time.Nanosecond},
		{Step: "d", Latency: 10 * time.Minute},
		{Step: "e", Latency: 90 * time.Minute},
		{Step: "f", Latency: 5*24*time.Hour + 6*time.Hour},
	})
	want := "STEP  LATENCY\n" +
		"a        12ms\n" +
		"b       250µs\n" +
		"c         5ns\n" +
		"d         10m\n" +
		"e      1h 30m\n" +
		"f       5d 6h"
	equal(t, got, want)
}

// myMarshaler implements encoding.TextMarshaler.
type myMarshaler struct{ s string }

func (m myMarshaler) MarshalText() ([]byte, error) { return []byte("MT:" + m.s), nil }

func TestRenderTextMarshaler(t *testing.T) {
	type Row struct {
		Field myMarshaler
	}
	got := render([]Row{{Field: myMarshaler{s: "hello"}}})
	want := "FIELD   \nMT:hello"
	equal(t, got, want)
}

// myFormatter implements fmt.Formatter only (not Stringer/TextMarshaler).
type myFormatter struct{ s string }

func (m myFormatter) Format(f fmt.State, _ rune) { fmt.Fprint(f, "F:"+m.s) }

func TestRenderFormatter(t *testing.T) {
	type Row struct {
		Field myFormatter
	}
	got := render([]Row{{Field: myFormatter{s: "world"}}})
	want := "FIELD  \nF:world"
	equal(t, got, want)
}

// myStringer implements fmt.Stringer only.
type myStringer struct{ s string }

func (m myStringer) String() string { return "S:" + m.s }

func TestRenderStringer(t *testing.T) {
	type Row struct {
		Field myStringer
	}
	got := render([]Row{{Field: myStringer{s: "tag"}}})
	want := "FIELD\nS:tag"
	equal(t, got, want)
}

func TestRenderPointerReceiverStringer(t *testing.T) {
	// *big.Int's String() is on the pointer receiver — exercises the
	// addressability path.
	type Row struct {
		Amount big.Int
	}
	var amt big.Int
	amt.SetString("123456789012345", 10)
	got := render([]Row{{Amount: amt}})
	want := "AMOUNT         \n123456789012345"
	equal(t, got, want)
}

func TestRenderTagHeaderOverride(t *testing.T) {
	type Row struct {
		FirstName string `table:"NAME"`
		Age       int
	}
	got := render([]Row{{FirstName: "Alice", Age: 30}})
	want := "NAME   AGE\nAlice   30"
	equal(t, got, want)
}

func TestRenderTagSkip(t *testing.T) {
	type Row struct {
		Visible string
		Hidden  string `table:"-"`
	}
	got := render([]Row{{Visible: "yes", Hidden: "no"}})
	want := "VISIBLE\nyes    "
	equal(t, got, want)
}

func TestRenderTagBytes(t *testing.T) {
	type Row struct {
		File string
		Size int64 `table:"SIZE,bytes"`
	}
	got := render([]Row{
		{File: "small", Size: 128},
		{File: "medium", Size: 3584},   // 3.5 KiB
		{File: "large", Size: 1048576}, // 1.0 MiB
	})
	want := "FILE    SIZE   \n" +
		"small     128 B\n" +
		"medium  3.5 KiB\n" +
		"large   1.0 MiB"
	equal(t, got, want)
}

func TestRenderTagBytesEmptyHeader(t *testing.T) {
	type Row struct {
		Size int64 `table:",bytes"`
	}
	got := render([]Row{{Size: 2048}})
	want := "SIZE   \n2.0 KiB"
	equal(t, got, want)
}

func TestRenderTagPercent(t *testing.T) {
	type Row struct {
		Ratio float64 `table:"RATIO,percent"`
	}
	got := render([]Row{
		{Ratio: 0.001},
		{Ratio: 1.0},
		{Ratio: 0.42},
	})
	want := "RATIO  \n  0.1 %\n100.0 %\n 42.0 %"
	equal(t, got, want)
}

func TestRenderTagBytesRejectsFloat(t *testing.T) {
	type Row struct {
		Size float64 `table:",bytes"`
	}
	_, err := renderWrite([]Row{{Size: 1.5}})
	equalErr(t, err, "field Size: 'bytes' modifier requires int or uint, got float64")
}

func TestRenderTagPercentRejectsInt(t *testing.T) {
	type Row struct {
		Pct int `table:",percent"`
	}
	_, err := renderWrite([]Row{{Pct: 50}})
	equalErr(t, err, "field Pct: 'percent' modifier requires float, got int")
}

func TestRenderTagUnknownModifier(t *testing.T) {
	type Row struct {
		X int `table:",bogus"`
	}
	_, err := renderWrite([]Row{{X: 1}})
	equalErr(t, err, `field X: unknown modifier "bogus"`)
}

func TestWriteMidStreamError(t *testing.T) {
	type Row struct{ Name string }
	sentinel := errors.New("kaput")
	seq := func(yield func(Row, error) bool) {
		if !yield(Row{Name: "alice"}, nil) {
			return
		}
		yield(Row{}, sentinel)
	}
	var buf bytes.Buffer
	err := Write[Row](&buf, seq)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no output on mid-stream error, got %q", buf.String())
	}
}

func TestRenderPointerElement(t *testing.T) {
	type Row struct {
		Name string
		Age  int
	}
	rows := []*Row{
		{Name: "Alice", Age: 30},
		nil,
		{Name: "Bob", Age: 25},
	}
	got := render(rows)
	want := "NAME   AGE\n" +
		"Alice   30\n" +
		"          \n" +
		"Bob     25"
	equal(t, got, want)
}

func TestRenderRejectsNonStruct(t *testing.T) {
	_, err := renderWrite([]int{1, 2, 3})
	equalErr(t, err, "row type must be a struct or slice/array, got int (consider WithColumns / WithHeaders)")
}

func TestRenderNestedSliceAsJSON(t *testing.T) {
	type Row struct {
		Name string
		Tags []string
	}
	got := render([]Row{
		{Name: "alice", Tags: []string{"admin", "ops"}},
		{Name: "bob", Tags: nil},
	})
	want := "NAME   TAGS           \n" +
		"alice  [\"admin\",\"ops\"]\n" +
		"bob    null           "
	equal(t, got, want)
}

func TestRenderNestedMapAsJSON(t *testing.T) {
	type Row struct {
		Name  string
		Attrs map[string]int
	}
	got := render([]Row{
		{Name: "alice", Attrs: map[string]int{"a": 1, "b": 2}},
	})
	want := "NAME   ATTRS        \nalice  {\"a\":1,\"b\":2}"
	equal(t, got, want)
}

func TestRenderNestedStructAsJSON(t *testing.T) {
	type Inner struct {
		X int
		Y int
	}
	type Row struct {
		Name string
		Pos  Inner
	}
	got := render([]Row{{Name: "p", Pos: Inner{X: 3, Y: 4}}})
	want := "NAME  POS          \np     {\"X\":3,\"Y\":4}"
	equal(t, got, want)
}

func TestRenderEmptyInput(t *testing.T) {
	type Row struct {
		Name string
		Age  int
	}
	got := render([]Row{})
	want := "NAME  AGE"
	equal(t, got, want)
}

func TestRenderRightAlignedNumeric(t *testing.T) {
	type Row struct {
		Latency int
	}
	got := render([]Row{
		{Latency: 1},
		{Latency: 9999},
	})
	want := "LATENCY\n      1\n   9999"
	equal(t, got, want)
}

func TestRenderHeaderAlwaysLeftAligned(t *testing.T) {
	type Row struct {
		N int
	}
	got := render([]Row{{N: 999999}})
	want := "N     \n999999"
	equal(t, got, want)
}

func TestRenderBorderlessByDefault(t *testing.T) {
	type Row struct {
		Name string
	}
	got := render([]Row{{Name: "alice"}})
	want := "NAME \nalice"
	equal(t, got, want)
}

func TestRenderWithBorder(t *testing.T) {
	type Row struct {
		Name string
	}
	got := render([]Row{{Name: "alice"}}, WithBorder(lipgloss.NormalBorder()))
	want := "┌───────┐\n│ NAME  │\n├───────┤\n│ alice │\n└───────┘"
	equal(t, got, want)
}

func widthStyles(w int) *stripes.Styles {
	s := stripes.DefaultStyles.Clone()
	s.Width = w
	return s
}

func TestFitTruncatesWidestColumn(t *testing.T) {
	type Row struct {
		Name string
		Tags []string
	}
	rows := []Row{
		{Name: "alice", Tags: []string{"administrator", "operations", "billing", "support"}},
		{Name: "bob", Tags: []string{"viewer"}},
	}
	got := render(rows, WithStyles(widthStyles(40)))
	want := "NAME   TAGS                             \n" +
		"alice  [\"administrator\",\"operations\",...\n" +
		"bob    [\"viewer\"]                       "
	equal(t, got, want)
}

func TestFitNoOpWhenTableFits(t *testing.T) {
	type Row struct {
		Name string
		Age  int
	}
	got := render([]Row{
		{Name: "alice", Age: 30},
		{Name: "bob", Age: 25},
	}, WithStyles(widthStyles(80)))
	want := "NAME   AGE\nalice   30\nbob     25"
	equal(t, got, want)
}

func TestFitZeroWidthDisablesFitting(t *testing.T) {
	type Row struct {
		Name string
	}
	long := strings.Repeat("x", 200)
	got := render([]Row{{Name: long}}, WithStyles(widthStyles(0)))
	want := "NAME" + strings.Repeat(" ", 200-4) + "\n" + long
	equal(t, got, want)
}

func TestFitShrinksProportionally(t *testing.T) {
	// Two long columns with very different natural widths: the widest
	// (B) is drained first, so the narrower one (A) keeps more content.
	type Row struct {
		A string
		B string
	}
	short := strings.Repeat("a", 20)
	long := strings.Repeat("b", 40)
	got := render([]Row{{A: short, B: long}}, WithStyles(widthStyles(30)))
	want := "A               B             \n" +
		"aaaaaaaaaaa...  bbbbbbbbbbb..."
	equal(t, got, want)
}

func TestTruncateHelper(t *testing.T) {
	cases := []struct {
		in, want string
		width    int
	}{
		{"hello", "hello", 10},
		{"hello", "hello", 5},
		{"helloworld", "he...", 5},
		{"helloworld", "hel", 3},
		{"hello", "he", 2},
		{"", "", 5},
		{"abcdefghij", "abcdefghij", 10},
		{"abcdefghij", "abcdef...", 9},
	}
	for _, c := range cases {
		got := truncate(c.in, c.width)
		if got != c.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", c.in, c.width, got, c.want)
		}
	}
}

func TestRenderHeaderBoldANSI(t *testing.T) {
	forceColor(t)
	type Row struct {
		Name string
	}
	var buf bytes.Buffer
	if err := Write[Row](&buf, seq2Of([]Row{{Name: "alice"}})); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[1mNAME\x1b[0m \nalice"
	equal(t, buf.String(), want)
}

func TestRenderJSONCellHasInlineANSI(t *testing.T) {
	forceColor(t)
	type Row struct {
		Tags []string
	}
	var buf bytes.Buffer
	if err := Write[Row](&buf, seq2Of([]Row{{Tags: []string{"admin", "ops"}}})); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[1mTAGS\x1b[0m           \n" +
		"\x1b[1;37m[\x1b[0m\x1b[32m\"admin\"\x1b[0m\x1b[1;37m,\x1b[0m\x1b[32m\"ops\"\x1b[0m\x1b[1;37m]\x1b[0m"
	equal(t, buf.String(), want)
}

func TestColorizeJSONTolerantOfTruncation(t *testing.T) {
	// The colorizer must round-trip its input when ANSI is stripped, even
	// for malformed / truncated JSON. Tested via the existing helper.
	styles := stripes.DefaultStyles
	cases := []string{
		`{"a":1,"b":"hel...`,
		`["foo","bar"...`,
		`true...`,
		``,
		`{`,
	}
	for _, in := range cases {
		out := colorizeJSON(in, styles)
		stripped := ansi.Strip(out)
		if stripped != in {
			t.Errorf("colorizeJSON round-trip: in=%q stripped=%q (raw=%q)", in, stripped, out)
		}
	}
}

func TestRenderJSONCellWithTruncation(t *testing.T) {
	forceColor(t)
	type Row struct {
		Tags []string
	}
	rows := []Row{
		{Tags: []string{"administrator", "operations", "billing", "support"}},
		{Tags: []string{"viewer"}},
	}
	var buf bytes.Buffer
	if err := Write[Row](&buf, seq2Of(rows), WithStyles(widthStyles(30))); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[1mTAGS\x1b[0m                          \n" +
		"\x1b[1;37m[\x1b[0m\x1b[32m\"administrator\"\x1b[0m\x1b[1;37m,\x1b[0m\x1b[32m\"operation...\x1b[0m\n" +
		"\x1b[1;37m[\x1b[0m\x1b[32m\"viewer\"\x1b[0m\x1b[1;37m]\x1b[0m                    "
	equal(t, buf.String(), want)
}

func TestRenderColumnStyleOnStruct(t *testing.T) {
	forceColor(t)
	type Row struct {
		Name   string
		Status string
		Count  int
	}
	rows := []Row{
		{Name: "alice", Status: "ok", Count: 3},
		{Name: "bob", Status: "fail", Count: 1},
	}
	got := Format[Row](seqOf(rows),
		WithColumnStyle(func(col int, val string) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Background(lipgloss.Color("#ff0066"))
			}
			return lipgloss.NewStyle()
		}),
	)
	want := "\x1b[1mNAME\x1b[0m   \x1b[1mSTATUS\x1b[0m  \x1b[1mCOUNT\x1b[0m\n" +
		"alice  \x1b[48;2;255;0;102mok\x1b[0m\x1b[48;2;255;0;102m  \x1b[0m\x1b[48;2;255;0;102m    \x1b[0m    3\n" +
		"bob    \x1b[48;2;255;0;102mfail\x1b[0m\x1b[48;2;255;0;102m  \x1b[0m\x1b[48;2;255;0;102m  \x1b[0m    1"
	equal(t, got, want)
}

func TestRenderColumnStyleOnSlice(t *testing.T) {
	forceColor(t)
	rows := [][]string{
		{"alice", "ok"},
		{"bob", "fail"},
	}
	got := Format[[]string](seqOf(rows),
		WithHeaders("NAME", "STATUS"),
		WithColumnStyle(func(col int, val string) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Background(lipgloss.Color("#ff0066"))
			}
			return lipgloss.NewStyle()
		}),
	)
	want := "\x1b[1mNAME\x1b[0m   \x1b[1mSTATUS\x1b[0m\n" +
		"alice  \x1b[48;2;255;0;102mok\x1b[0m\x1b[48;2;255;0;102m    \x1b[0m\n" +
		"bob    \x1b[48;2;255;0;102mfail\x1b[0m\x1b[48;2;255;0;102m  \x1b[0m"
	equal(t, got, want)
}

func TestRenderColumnStyleComposesWithColorizeJSON(t *testing.T) {
	forceColor(t)
	type Row struct {
		Tags []string
	}
	got := Format[Row](seqOf([]Row{{Tags: []string{"ops"}}}),
		WithColumnStyle(func(col int, val string) lipgloss.Style {
			return lipgloss.NewStyle().Background(lipgloss.Color("#3366ff"))
		}),
	)
	// JSON-token ANSI must still be present inside the cell, AND the
	// outer lipgloss background must wrap it.
	if !strings.Contains(got, "\x1b[32m\"ops\"\x1b[0m") {
		t.Errorf("missing JSON string-token ANSI in output: %q", got)
	}
	if !strings.Contains(got, "\x1b[48;2;51;102;255m") {
		t.Errorf("missing per-column background ANSI in output: %q", got)
	}
}

func TestRenderHeaderStyleInheritsBold(t *testing.T) {
	forceColor(t)
	type Row struct {
		Name   string
		Status string
	}
	got := Format[Row](seqOf([]Row{{Name: "alice", Status: "ok"}}),
		WithHeaderStyle(func(col int, val string) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Background(lipgloss.Color("#3366ff"))
			}
			return lipgloss.NewStyle()
		}),
	)
	// Header col 0 keeps plain bold; col 1 is bold + background (Inherit
	// composes the user's background with the default bold).
	want := "\x1b[1mNAME\x1b[0m   \x1b[1;48;2;51;102;255mSTATUS\x1b[0m\nalice  ok    "
	equal(t, got, want)
}

func TestRenderRowStyleAlternating(t *testing.T) {
	forceColor(t)
	type Row struct {
		Name string
	}
	rows := []Row{{Name: "a"}, {Name: "b"}, {Name: "c"}, {Name: "d"}}
	got := Format[Row](seqOf(rows),
		WithRowStyle(func(row int) lipgloss.Style {
			if row%2 == 1 {
				return lipgloss.NewStyle().Background(lipgloss.Color("#222222"))
			}
			return lipgloss.NewStyle()
		}),
	)
	want := "\x1b[1mNAME\x1b[0m\n" +
		"a   \n" +
		"\x1b[48;2;34;34;34mb\x1b[0m\x1b[48;2;34;34;34m   \x1b[0m\n" +
		"c   \n" +
		"\x1b[48;2;34;34;34md\x1b[0m\x1b[48;2;34;34;34m   \x1b[0m"
	equal(t, got, want)
}

func TestRenderRowAndColumnStyleCompose(t *testing.T) {
	forceColor(t)
	type Row struct {
		Name   string
		Status string
	}
	rows := []Row{{Name: "a", Status: "ok"}, {Name: "b", Status: "fail"}}
	got := Format[Row](seqOf(rows),
		WithRowStyle(func(row int) lipgloss.Style {
			if row%2 == 1 {
				return lipgloss.NewStyle().Background(lipgloss.Color("#222222"))
			}
			return lipgloss.NewStyle()
		}),
		WithColumnStyle(func(col int, val string) lipgloss.Style {
			if col == 1 {
				return lipgloss.NewStyle().Background(lipgloss.Color("#ff0066"))
			}
			return lipgloss.NewStyle()
		}),
	)
	// Order of layering means column wins on col 1 for both rows, and
	// row-style still paints col 0 on the odd row.
	row0Bg := "\x1b[48;2;34;34;34m" // dark gray
	colBg := "\x1b[48;2;255;0;102m" // pink
	if !strings.Contains(got, colBg) {
		t.Errorf("missing column background in output: %q", got)
	}
	if !strings.Contains(got, row0Bg) {
		t.Errorf("missing row background in output: %q", got)
	}
	// Row 1 col 0 should carry the row background (no column-style wins
	// here because column 0 returns a zero style).
	lines := strings.Split(got, "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %q", len(lines), got)
	}
	if !strings.HasPrefix(lines[2], row0Bg) {
		t.Errorf("odd data row should start with row background; line=%q", lines[2])
	}
	// And the same odd row's col 1 should carry the column background,
	// not the row background.
	if !strings.Contains(lines[2], colBg+"fail") {
		t.Errorf("odd data row should carry column bg on col 1; line=%q", lines[2])
	}
}

func benchmarkWrite(b *testing.B, n int) {
	type Row struct {
		Name    string
		Count   int
		Size    int64 `table:",bytes"`
		Latency time.Duration
	}
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{
			Name:    "row",
			Count:   i,
			Size:    int64(i) * 1024,
			Latency: time.Duration(i) * time.Millisecond,
		}
	}
	w := NewWriter[Row]()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var sink bytes.Buffer
		sink.Grow(n * 32)
		if err := w(&sink, seq2Of(rows)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkWriteSmall(b *testing.B)  { benchmarkWrite(b, 10) }
func BenchmarkWriteMedium(b *testing.B) { benchmarkWrite(b, 100) }
func BenchmarkWriteLarge(b *testing.B)  { benchmarkWrite(b, 1000) }

func benchmarkStyleHooks(b *testing.B, n int, opts ...Option) {
	type Row struct {
		Name   string
		Status string
		Count  int
	}
	rows := make([]Row, n)
	for i := range rows {
		rows[i] = Row{Name: "row", Status: "ok", Count: i}
	}
	w := NewWriter[Row](opts...)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var sink bytes.Buffer
		sink.Grow(n * 32)
		if err := w(&sink, seq2Of(rows)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderNoStyleHooks(b *testing.B) {
	benchmarkStyleHooks(b, 100)
}

func BenchmarkRenderColumnStyle(b *testing.B) {
	red := lipgloss.NewStyle().Background(lipgloss.Color("#ff0066"))
	benchmarkStyleHooks(b, 100, WithColumnStyle(func(col int, _ string) lipgloss.Style {
		if col == 1 {
			return red
		}
		return lipgloss.NewStyle()
	}))
}

func BenchmarkRenderRowStyle(b *testing.B) {
	dim := lipgloss.NewStyle().Background(lipgloss.Color("#222222"))
	benchmarkStyleHooks(b, 100, WithRowStyle(func(row int) lipgloss.Style {
		if row%2 == 1 {
			return dim
		}
		return lipgloss.NewStyle()
	}))
}
