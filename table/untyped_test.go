package table

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestUntypedStringSlice(t *testing.T) {
	rows := [][]string{
		{"alice", "admin", "ny"},
		{"bob", "viewer", "la"},
	}
	got := ansi.Strip(Format[[]string](seqOf(rows), WithHeaders("NAME", "ROLE", "CITY")))
	want := "NAME   ROLE    CITY\nalice  admin   ny  \nbob    viewer  la  "
	equal(t, got, want)
}

func TestUntypedFloatSlice(t *testing.T) {
	rows := [][]float64{
		{1.5, 2.5},
		{10.0, 0.25},
	}
	got := ansi.Strip(Format[[]float64](seqOf(rows), WithHeaders("X", "Y")))
	want := "X    Y   \n1.5   2.5\n 10  0.25"
	equal(t, got, want)
}

func TestUntypedInt64BytesModifier(t *testing.T) {
	rows := [][]int64{
		{128},
		{3584},
		{1048576},
	}
	got := ansi.Strip(Format[[]int64](seqOf(rows), WithColumns(Column{Header: "SIZE", Modifier: "bytes"})))
	want := "SIZE  \n  128B\n3.5KiB\n1.0MiB"
	equal(t, got, want)
}

func TestUntypedInt64CountModifier(t *testing.T) {
	rows := [][]int64{
		{500},
		{1500},
		{1_500_000},
	}
	got := ansi.Strip(Format[[]int64](seqOf(rows), WithColumns(Column{Header: "REQS", Modifier: "count"})))
	want := "REQS\n 500\n1.5K\n1.5M"
	equal(t, got, want)
}

func TestUntypedAnyMixedTypes(t *testing.T) {
	now := time.Date(2026, 5, 11, 10, 23, 45, 0, time.UTC)
	rows := [][]any{
		{42, "hello", now},
		{int64(7), "world", time.Time{}},
	}
	got := ansi.Strip(Format[[]any](seqOf(rows), WithHeaders("N", "S", "T")))
	want := "N   S      T                  \n" +
		"42  hello  2026/05/11 10:23:45\n" +
		" 7  world                     "
	equal(t, got, want)
}

func TestUntypedAnyNestedSlice(t *testing.T) {
	rows := [][]any{
		{1, []string{"admin", "ops"}},
		{2, map[string]int{"a": 1, "b": 2}},
	}
	got := ansi.Strip(Format[[]any](seqOf(rows), WithHeaders("ID", "EXTRA")))
	want := "ID  EXTRA          \n 1  [\"admin\",\"ops\"]\n 2  {\"a\":1,\"b\":2}  "
	equal(t, got, want)
}

func TestUntypedAnyJSONColorize(t *testing.T) {
	// Nested cells inside []any rows pick up the JSON token colorizer
	// even though the column's static type is `any`: anyCellFormatter
	// wraps the per-cell formatter with colorizeJSON when the dynamic
	// type is a JSON-fallback shape (slice/array/map/struct).
	var buf bytes.Buffer
	rows := [][]any{{[]string{"x", "y"}}}
	if err := Write[[]any](&buf, seq2Of(rows), WithHeaders("V")); err != nil {
		t.Fatal(err)
	}
	want := "\x1b[1mV\x1b[m        \n" +
		"\x1b[1;37m[\x1b[m\x1b[32m\"x\"\x1b[m\x1b[1;37m,\x1b[m\x1b[32m\"y\"\x1b[m\x1b[1;37m]\x1b[m"
	equal(t, buf.String(), want)
}

func TestUntypedRowPaddedWhenShort(t *testing.T) {
	rows := [][]string{
		{"alice", "admin", "ny"},
		{"bob", "viewer"}, // only 2 cells; column 3 should pad empty
	}
	got := ansi.Strip(Format[[]string](seqOf(rows), WithHeaders("NAME", "ROLE", "CITY")))
	want := "NAME   ROLE    CITY\nalice  admin   ny  \nbob    viewer      "
	equal(t, got, want)
}

func TestUntypedRowExtraCellsDropped(t *testing.T) {
	rows := [][]string{
		{"alice", "admin", "ny", "extra"}, // 4 cells; column count is 3
	}
	got := ansi.Strip(Format[[]string](seqOf(rows), WithHeaders("NAME", "ROLE", "CITY")))
	want := "NAME   ROLE   CITY\nalice  admin  ny  "
	equal(t, got, want)
}

func TestUntypedRejectsStructWithColumns(t *testing.T) {
	// time.Time is a struct; Columns with it should fail because struct
	// rows take headers from struct tags, not Options.Columns.
	_, err := renderWrite([]time.Time{time.Now()}, WithHeaders("WHEN"))
	equalErr(t, err, "WithColumns / WithHeaders is for slice/array rows; use struct tags to customise struct columns")
}

func TestUntypedRejectsScalarWithoutColumns(t *testing.T) {
	// True scalar (primitive) without Columns: error must point at
	// WithColumns / WithHeaders.
	_, err := renderWrite([]int{1, 2})
	equalErr(t, err, "row type must be a struct or slice/array, got int (consider WithColumns / WithHeaders)")
}

func TestUntypedRejectsScalarWithColumns(t *testing.T) {
	// True scalar with Columns: not a struct, not a slice — reject.
	_, err := renderWrite([]int{1, 2}, WithHeaders("N"))
	equalErr(t, err, "row type must be a struct or slice/array, got int")
}

func TestUntypedRejectsExportedStructWithColumns(t *testing.T) {
	type Row struct {
		Name string
	}
	_, err := renderWrite([]Row{{Name: "x"}}, WithHeaders("A"))
	equalErr(t, err, "WithColumns / WithHeaders is for slice/array rows; use struct tags to customise struct columns")
}

func TestUntypedSliceMissingColumns(t *testing.T) {
	_, err := renderWrite([][]string{{"a"}})
	equalErr(t, err, "row type must be a struct or slice/array, got []string (consider WithColumns / WithHeaders)")
}

func TestUntypedUnknownModifier(t *testing.T) {
	_, err := renderWrite([][]string{{"x"}},
		WithColumns(Column{Header: "X", Modifier: "bogus"}))
	equalErr(t, err, `column 0 ("X"): unknown modifier "bogus"`)
}

func TestUntypedBytesOnNonInt(t *testing.T) {
	_, err := renderWrite([][]string{{"x"}},
		WithColumns(Column{Header: "X", Modifier: "bytes"}))
	equalErr(t, err, `column 0 ("X"): 'bytes' modifier requires int or uint, got string`)
}

func TestUntypedCountOnNonInt(t *testing.T) {
	_, err := renderWrite([][]string{{"x"}},
		WithColumns(Column{Header: "X", Modifier: "count"}))
	equalErr(t, err, `column 0 ("X"): 'count' modifier requires int or uint, got string`)
}

func TestUntypedPointerElementSlice(t *testing.T) {
	one, two := 1, 2
	rows := [][]*int{
		{&one, &two},
		{nil, &one},
	}
	got := ansi.Strip(Format[[]*int](seqOf(rows), WithHeaders("A", "B")))
	want := "A  B\n1  2\n   1"
	equal(t, got, want)
}

func TestUntypedWidthFittingApplies(t *testing.T) {
	rows := [][]string{
		{strings.Repeat("x", 60), "tail"},
	}
	s := stripes.DefaultStyles.Clone()
	s.Width = 30
	got := ansi.Strip(Format[[]string](seqOf(rows),
		WithHeaders("LONG", "SHORT"),
		WithStyles(s)))
	want := "LONG                     SHORT\nxxxxxxxxxxxxxxxxxxxx...  tail "
	equal(t, got, want)
}
