package table

import (
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestDetectNumericAlignmentAllNumeric(t *testing.T) {
	rows := [][]string{
		{"alice", "100"},
		{"bob", "75"},
	}
	a := []align{alignLeft, alignLeft}
	detectNumericAlignment(rows, a)
	if a[0] != alignLeft {
		t.Errorf("col 0 (text) should stay left, got %d", a[0])
	}
	if a[1] != alignRight {
		t.Errorf("col 1 (numeric) should flip to right, got %d", a[1])
	}
}

func TestDetectNumericAlignmentUnits(t *testing.T) {
	rows := [][]string{
		{"50.0 %", "1h", "1.0 KiB"},
		{"99.9 %", "30m", "256 B"},
	}
	a := []align{alignLeft, alignLeft, alignLeft}
	detectNumericAlignment(rows, a)
	for i, got := range a {
		if got != alignRight {
			t.Errorf("col %d (numeric with unit) should be right, got %d", i, got)
		}
	}
}

func TestDetectNumericAlignmentMixed(t *testing.T) {
	rows := [][]string{
		{"100"},
		{"low"},
		{"200"},
	}
	a := []align{alignLeft}
	detectNumericAlignment(rows, a)
	if a[0] != alignLeft {
		t.Errorf("mixed text+numeric column should stay left, got %d", a[0])
	}
}

func TestDetectNumericAlignmentAllEmpty(t *testing.T) {
	rows := [][]string{
		{""},
		{""},
	}
	a := []align{alignLeft}
	detectNumericAlignment(rows, a)
	if a[0] != alignLeft {
		t.Errorf("all-empty column should stay left, got %d", a[0])
	}
}

func TestDetectNumericAlignmentEmptyMixedNumeric(t *testing.T) {
	rows := [][]string{
		{""},
		{"100"},
		{""},
		{"200"},
	}
	a := []align{alignLeft}
	detectNumericAlignment(rows, a)
	if a[0] != alignRight {
		t.Errorf("numeric column with some empty cells should be right, got %d", a[0])
	}
}

func TestDetectNumericAlignmentDoesNotDemote(t *testing.T) {
	// A column already marked alignRight by static type must not be
	// changed even if its data cells look non-numeric.
	rows := [][]string{
		{"alice"},
	}
	a := []align{alignRight}
	detectNumericAlignment(rows, a)
	if a[0] != alignRight {
		t.Errorf("already-right column must stay right, got %d", a[0])
	}
}

func TestRenderCSVLikeSliceDetectsNumeric(t *testing.T) {
	// Slice-row rendering should right-align the numeric column.
	rows := [][]string{
		{"Widget", "100"},
		{"Gadget", "75"},
	}
	got := ansi.Strip(Format[[]string](seqOf(rows), WithHeaders("Product", "Quantity")))
	want := "Product  Quantity\nWidget        100\nGadget         75"
	equal(t, got, want)
}

func TestRenderAnySliceDetectsNumeric(t *testing.T) {
	rows := [][]any{
		{"alice", 30},
		{"bob", 25},
	}
	got := ansi.Strip(Format[[]any](seqOf(rows), WithHeaders("NAME", "AGE")))
	want := "NAME   AGE\nalice   30\nbob     25"
	equal(t, got, want)
}

func TestRenderStructStringFieldStaysLeft(t *testing.T) {
	// A struct row with a string field that happens to hold numeric
	// values must NOT be auto-right-aligned. The caller picked string.
	type Row struct {
		Quantity string
	}
	got := ansi.Strip(Format[Row](seqOf([]Row{{Quantity: "100"}, {Quantity: "75"}})))
	want := "QUANTITY\n100     \n75      "
	equal(t, got, want)
}
