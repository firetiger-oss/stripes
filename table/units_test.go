package table

import (
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestHumanBytes(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0 B"},
		{1, "1 B"},
		{1023, "1023 B"},
		{1024, "1.0 KiB"},
		{1536, "1.5 KiB"},
		{3584, "3.5 KiB"},
		{1024 * 1024, "1.0 MiB"},
		{1024*1024 + 512*1024, "1.5 MiB"},
		{1024 * 1024 * 1024, "1.0 GiB"},
		{1024 * 1024 * 1024 * 1024, "1.0 TiB"},
		{int64(1024) * 1024 * 1024 * 1024 * 1024, "1.0 PiB"},
		{int64(1024) * 1024 * 1024 * 1024 * 1024 * 1024, "1.0 EiB"},
		{-1024, "-1.0 KiB"},
		{-1, "-1 B"},
	}
	for _, c := range cases {
		if got := humanBytes(c.in); got != c.want {
			t.Errorf("humanBytes(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanizeDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{0, "0ns"},
		{1 * time.Nanosecond, "1ns"},
		{999 * time.Nanosecond, "999ns"},
		{1 * time.Microsecond, "1µs"},
		{250 * time.Microsecond, "250µs"},
		{999 * time.Microsecond, "999µs"},
		{1 * time.Millisecond, "1ms"},
		{12 * time.Millisecond, "12ms"},
		{999 * time.Millisecond, "999ms"},
		{1 * time.Second, "1s"},
		{45 * time.Second, "45s"},
		{1 * time.Minute, "1m"},
		{1*time.Minute + 30*time.Second, "1m 30s"},
		{10 * time.Minute, "10m"},
		{1 * time.Hour, "1h"},
		{1*time.Hour + 30*time.Minute, "1h 30m"},
		{24 * time.Hour, "1d"},
		{5*24*time.Hour + 6*time.Hour, "5d 6h"},
		{-5 * time.Minute, "-5m"},
		{-1 * time.Hour, "-1h"},
	}
	for _, c := range cases {
		if got := humanizeDuration(c.in); got != c.want {
			t.Errorf("humanizeDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestHumanRelative(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{500 * time.Millisecond, "<1s ago"},
		{1 * time.Second, "1s ago"},
		{30 * time.Second, "30s ago"},
		{1 * time.Minute, "1m ago"},
		{59 * time.Minute, "59m ago"},
		{1 * time.Hour, "1h ago"},
		{23 * time.Hour, "23h ago"},
		{24 * time.Hour, "1d ago"},
		{5 * 24 * time.Hour, "5d ago"},
		{-1 * time.Minute, "in 1m"},
		{-1 * time.Hour, "in 1h"},
	}
	for _, c := range cases {
		if got := humanRelative(c.in); got != c.want {
			t.Errorf("humanRelative(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseTag(t *testing.T) {
	cases := []struct {
		in   string
		name string
		mods []string
	}{
		{"", "", nil},
		{"NAME", "NAME", []string{}},
		{"NAME,bytes", "NAME", []string{"bytes"}},
		{",bytes", "", []string{"bytes"}},
		{",bytes,percent", "", []string{"bytes", "percent"}},
		{"NAME,bytes,percent", "NAME", []string{"bytes", "percent"}},
	}
	for _, c := range cases {
		name, mods := parseTag(c.in)
		if name != c.name {
			t.Errorf("parseTag(%q) name = %q, want %q", c.in, name, c.name)
		}
		if !reflect.DeepEqual(mods, c.mods) && !(len(mods) == 0 && len(c.mods) == 0) {
			t.Errorf("parseTag(%q) mods = %v, want %v", c.in, mods, c.mods)
		}
	}
}

func TestAlignmentForType(t *testing.T) {
	cases := []struct {
		in   reflect.Type
		want align
	}{
		{reflect.TypeFor[int](), alignRight},
		{reflect.TypeFor[int64](), alignRight},
		{reflect.TypeFor[uint8](), alignRight},
		{reflect.TypeFor[float32](), alignRight},
		{reflect.TypeFor[float64](), alignRight},
		{reflect.TypeFor[*int](), alignRight},
		{reflect.TypeFor[string](), alignLeft},
		{reflect.TypeFor[bool](), alignLeft},
		{reflect.TypeFor[[]string](), alignLeft},
		{reflect.TypeFor[map[string]int](), alignLeft},
		{reflect.TypeFor[time.Time](), alignLeft},
		{reflect.TypeFor[time.Duration](), alignRight}, // Duration is int64 under the hood
	}
	for _, c := range cases {
		if got := alignmentForType(c.in); got != c.want {
			t.Errorf("alignmentForType(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestPercentFormatter(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0.0 %"},
		{0.001, "0.1 %"},
		{0.1, "10.0 %"},
		{0.42, "42.0 %"},
		{1.0, "100.0 %"},
		{1.5, "150.0 %"},
		{-0.25, "-25.0 %"},
		{math.NaN(), "NaN %"},
	}
	for _, c := range cases {
		v := reflect.ValueOf(c.in)
		got := percentFormatter(v)
		if got != c.want {
			t.Errorf("percentFormatter(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestBytesFormatterIntAndUint(t *testing.T) {
	intFmt := bytesFormatter(reflect.Int64)
	uintFmt := bytesFormatter(reflect.Uint64)
	if got := intFmt(reflect.ValueOf(int64(1024))); got != "1.0 KiB" {
		t.Errorf("intFmt(1024) = %q", got)
	}
	if got := uintFmt(reflect.ValueOf(uint64(1024))); got != "1.0 KiB" {
		t.Errorf("uintFmt(1024) = %q", got)
	}
	// huge uint that overflows int64 should be clamped to MaxInt64.
	if got := uintFmt(reflect.ValueOf(uint64(math.MaxUint64))); got == "" {
		t.Errorf("uintFmt(MaxUint64) returned empty string")
	}
}

func TestColorizeJSONShapes(t *testing.T) {
	cases := []string{
		`{}`,
		`[]`,
		`{"a":1}`,
		`{"a":1,"b":"hi","c":[true,null]}`,
		`[1,2,3]`,
		`{"nested":{"deep":[1,{"k":"v"}]}}`,
		`"hello"`,
		`123`,
		`-12.5e3`,
		`true`,
		`false`,
		`null`,
	}
	for _, in := range cases {
		out := colorizeJSON(in, stripes.DefaultStyles)
		stripped := ansi.Strip(out)
		if stripped != in {
			t.Errorf("colorize round-trip: in=%q stripped=%q (raw=%q)", in, stripped, out)
		}
	}
}

func TestColorizeJSONEmitsPerToken(t *testing.T) {
	forceColor(t)
	got := colorizeJSON(`{"a":1,"b":true,"c":null,"d":"x"}`, stripes.DefaultStyles)
	want := "\x1b[1;37m{\x1b[0m\x1b[1;94m\"a\"\x1b[0m\x1b[1;37m:\x1b[0m\x1b[37m1\x1b[0m\x1b[1;37m,\x1b[0m" +
		"\x1b[1;94m\"b\"\x1b[0m\x1b[1;37m:\x1b[0m\x1b[37mtrue\x1b[0m\x1b[1;37m,\x1b[0m" +
		"\x1b[1;94m\"c\"\x1b[0m\x1b[1;37m:\x1b[0m\x1b[90mnull\x1b[0m\x1b[1;37m,\x1b[0m" +
		"\x1b[1;94m\"d\"\x1b[0m\x1b[1;37m:\x1b[0m\x1b[32m\"x\"\x1b[0m\x1b[1;37m}\x1b[0m"
	equal(t, got, want)
}

func TestFitToWidthBoundaries(t *testing.T) {
	headers := []string{"A", "B"}
	rows := [][]string{{"foo", "bar"}}
	// target so small that chrome alone exceeds it → no-op.
	got := fitToWidth(headers, rows, 1, false)
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("expected no-op when target < chrome, got %v", got)
	}
	// natural fit → no-op.
	got = fitToWidth(headers, rows, 100, false)
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("expected no-op when natural fits, got %v", got)
	}
	// zero columns → no-op.
	got = fitToWidth(nil, rows, 100, false)
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("expected no-op when no headers, got %v", got)
	}
	// zero width → no-op.
	got = fitToWidth(headers, rows, 0, false)
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("expected no-op when target=0, got %v", got)
	}
}

func TestFitToWidthHeadersAlone(t *testing.T) {
	// Long header + small budget: shrinkable is 0, must return rows
	// unchanged rather than shrinking the header.
	headers := []string{"LONGHEADERLONG"}
	rows := [][]string{{"x"}}
	got := fitToWidth(headers, rows, 8, false)
	if !reflect.DeepEqual(got, rows) {
		t.Errorf("expected rows untouched when headers alone overflow, got %v", got)
	}
}

func TestFitToWidthRespectsBorderChrome(t *testing.T) {
	headers := []string{"A"}
	rows := [][]string{{strings.Repeat("x", 20)}}
	// Bordered chrome for n=1: 1 left-pad + 1 right-pad + 2 verticals = 4.
	// Target 10 → budget = 10 - 4 = 6. So the cell should be truncated to ≤ 6.
	got := fitToWidth(headers, rows, 10, true)
	if got[0][0] == rows[0][0] {
		t.Fatalf("expected truncation under bordered chrome, got %q", got[0][0])
	}
	if w := ansi.StringWidth(got[0][0]); w > 6 {
		t.Errorf("truncated cell wider than budget: %d > 6 (cell=%q)", w, got[0][0])
	}
}

func TestRenderUnexportedFieldsSkipped(t *testing.T) {
	type Row struct {
		Public  string
		private string //nolint:unused
	}
	got := render([]Row{{Public: "yes", private: "hidden"}})
	want := "PUBLIC\nyes   "
	equal(t, got, want)
}

func TestRenderEmptyStructError(t *testing.T) {
	type Empty struct{}
	_, err := renderWrite([]Empty{{}})
	equalErr(t, err, "struct table.Empty has no exported fields to render")
}

func TestRenderNilPointerSchemaCell(t *testing.T) {
	type Inner struct {
		X int
	}
	type Row struct {
		Ptr *Inner
	}
	got := render([]Row{{Ptr: nil}, {Ptr: &Inner{X: 7}}})
	want := "PTR    \n       \n{\"X\":7}"
	equal(t, got, want)
}

func TestRenderIntegerKinds(t *testing.T) {
	type Row struct {
		I   int
		I8  int8
		I16 int16
		I32 int32
		I64 int64
		U   uint
		U8  uint8
		U16 uint16
		U32 uint32
		U64 uint64
		F32 float32
		F64 float64
	}
	got := render([]Row{{I: 1, I8: 2, I16: 3, I32: 4, I64: 5, U: 6, U8: 7, U16: 8, U32: 9, U64: 10, F32: 1.5, F64: 2.5}})
	want := "I  I 8  I 16  I 32  I 64  U  U 8  U 16  U 32  U 64  F 32  F 64\n" +
		"1    2     3     4     5  6    7     8     9    10   1.5   2.5"
	equal(t, got, want)
}
