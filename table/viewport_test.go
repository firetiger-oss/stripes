package table

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

type vpRow struct {
	Name string
	Age  int
}

func vpRows(n int) []vpRow {
	out := make([]vpRow, n)
	for i := range out {
		out[i] = vpRow{Name: "row" + strDigit(i), Age: i}
	}
	return out
}

// strDigit avoids importing strconv into a test helper; n is always small.
func strDigit(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestRowSelectorMarksMatchingRow(t *testing.T) {
	got := render(vpRows(3),
		WithRowSelector(func(row int) bool { return row == 1 }),
	)
	want := " NAME  AGE\n" +
		" row0    0\n" +
		"❯row1    1\n" +
		" row2    2"
	equal(t, got, want)
}

func TestRowSelectorMultipleSelectorsOR(t *testing.T) {
	got := render(vpRows(3),
		WithRowSelector(func(row int) bool { return row == 0 }),
		WithRowSelector(func(row int) bool { return row == 2 }),
	)
	want := " NAME  AGE\n" +
		"❯row0    0\n" +
		" row1    1\n" +
		"❯row2    2"
	equal(t, got, want)
}

func TestRowSelectorCustomIndicator(t *testing.T) {
	got := render(vpRows(2),
		WithRowSelector(func(row int) bool { return row == 0 }),
		WithSelectedIndicator("→"),
	)
	want := " NAME  AGE\n" +
		"→row0    0\n" +
		" row1    1"
	equal(t, got, want)
}

func TestRowSelectorEmptyTableStillShowsGutter(t *testing.T) {
	got := render([]vpRow{},
		WithRowSelector(func(row int) bool { return true }),
	)
	want := " NAME  AGE"
	equal(t, got, want)
}

func TestRowSelectorReceivesAbsoluteIndex(t *testing.T) {
	var seen []int
	got := render(vpRows(5),
		WithViewport(2, 2),
		WithRowSelector(func(row int) bool {
			seen = append(seen, row)
			return row == 3
		}),
	)
	// Selector is only invoked for visible rows (absolute indices 2, 3).
	if !reflect.DeepEqual(seen, []int{2, 3}) {
		t.Errorf("selector received %v, want [2 3]", seen)
	}
	// totalRows=5 height=2 top=2 → thumbSize=round(2*2/5)=1,
	// thumbStart = floor(2*2/5)=0. Thumb at visible row 0 (row2).
	// Row 3 (visible offset 1) matches the selector.
	want := " NAME  AGE│\n" +
		" row2    2▌\n" +
		"❯row3    3│"
	equal(t, got, want)
}

func TestViewportNoCropWhenDataFits(t *testing.T) {
	// height >= totalRows: no cropping, no scrollbar — identical to no-viewport.
	withVP := render(vpRows(3), WithViewport(5, 0))
	noVP := render(vpRows(3))
	equal(t, withVP, noVP)
}

func TestViewportCropsRows(t *testing.T) {
	got := render(vpRows(5), WithViewport(2, 1))
	// totalRows=5 height=2 top=1 → thumbSize=round(2*2/5)=round(0.8)=1
	// thumbStart = floor(1*2/5)=0. Thumb at visible row 0 (row1).
	want := "NAME  AGE│\n" +
		"row1    1▌\n" +
		"row2    2│"
	equal(t, got, want)
}

func TestViewportTopClamped(t *testing.T) {
	// top=10 clamps to totalRows-height = 5-2 = 3.
	got := render(vpRows(5), WithViewport(2, 10))
	// totalRows=5 height=2 top=3 → thumbSize=1, thumbStart=floor(3*2/5)=1.
	// Thumb at visible row 1.
	want := "NAME  AGE│\n" +
		"row3    3│\n" +
		"row4    4▌"
	equal(t, got, want)
}

func TestViewportScrollbarThumbAtTop(t *testing.T) {
	got := render(vpRows(10), WithViewport(3, 0))
	// thumbSize = round(3*3/10) = round(0.9) = 1
	// thumbStart = floor(0*3/10) = 0 → thumb at visible row 0.
	want := "NAME  AGE│\n" +
		"row0    0▌\n" +
		"row1    1│\n" +
		"row2    2│"
	equal(t, got, want)
}

func TestViewportScrollbarThumbAtBottom(t *testing.T) {
	got := render(vpRows(10), WithViewport(3, 7))
	// thumbSize = 1, thumbStart = floor(7*3/10) = 2 → thumb at visible row 2.
	want := "NAME  AGE│\n" +
		"row7    7│\n" +
		"row8    8│\n" +
		"row9    9▌"
	equal(t, got, want)
}

func TestViewportScrollbarThumbMid(t *testing.T) {
	got := render(vpRows(10), WithViewport(3, 4))
	// thumbSize = 1, thumbStart = floor(4*3/10) = 1 → thumb at visible row 1.
	want := "NAME  AGE│\n" +
		"row4    4│\n" +
		"row5    5▌\n" +
		"row6    6│"
	equal(t, got, want)
}

func TestSelectorAndViewportCompose(t *testing.T) {
	got := render(vpRows(5),
		WithViewport(3, 1),
		WithRowSelector(func(row int) bool { return row == 2 }),
	)
	// Visible window is rows 1,2,3; row 2 is the middle visible row.
	// totalRows=5 height=3 top=1 → thumbSize=round(3*3/5)=round(1.8)=2.
	// thumbStart = floor(1*3/5) = 0 → thumb at visible rows 0..1.
	want := " NAME  AGE│\n" +
		" row1    1▌\n" +
		"❯row2    2▌\n" +
		" row3    3│"
	equal(t, got, want)
}

func TestRowStyleReceivesAbsoluteIndexWithViewport(t *testing.T) {
	seen := map[int]bool{}
	render(vpRows(5),
		WithViewport(3, 2),
		WithRowStyle(func(row int) lipgloss.Style {
			seen[row] = true
			return lipgloss.NewStyle()
		}),
	)
	// lipgloss calls StyleFunc multiple times per cell during measurement
	// and rendering; only the distinct row indices matter. Visible rows
	// are at absolute indices 2, 3, 4.
	want := map[int]bool{2: true, 3: true, 4: true}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("WithRowStyle saw rows %v, want %v", seen, want)
	}
}

func TestSelectorWithBorderRenders(t *testing.T) {
	got := render(vpRows(2),
		WithBorder(lipgloss.NormalBorder()),
		WithRowSelector(func(row int) bool { return row == 0 }),
	)
	// Smoke test: the indicator and the box-drawing chars should both
	// appear. v1 prefixes every line, so the indicator lands next to a
	// border line — acceptable behaviour, just pinned here.
	if !strings.Contains(got, "❯") {
		t.Errorf("expected indicator ❯ in output, got:\n%s", got)
	}
	if !strings.Contains(got, "┌") || !strings.Contains(got, "└") {
		t.Errorf("expected border characters in output, got:\n%s", got)
	}
}

func TestScrollbarThumbBoundsClampsLargeThumb(t *testing.T) {
	// Edge case: total just barely > height. Thumb must stay within window.
	start, size := scrollbarThumbBounds(4, 3, 1)
	// thumbSize = round(3*3/4) = round(2.25) = 2
	// thumbStart = floor(1*3/4) = 0
	if start != 0 || size != 2 {
		t.Errorf("got (%d,%d), want (0,2)", start, size)
	}
	// Max scroll top=1 (=total-height) — thumbStart pushed so thumb hits bottom.
	start, size = scrollbarThumbBounds(4, 3, 1)
	if start+size > 3 {
		t.Errorf("thumb overflows window: start=%d size=%d height=3", start, size)
	}
}

func benchmarkViewport(b *testing.B, n, height, top int, opts ...Option) {
	rows := vpRows(n)
	allOpts := append([]Option{WithViewport(height, top)}, opts...)
	w := NewWriter[vpRow](allOpts...)
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var sink bytes.Buffer
		sink.Grow(height * 32)
		if err := w(&sink, seq2Of(rows)); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkRenderWithViewport(b *testing.B) {
	benchmarkViewport(b, 1000, 20, 100)
}

func BenchmarkRenderWithSelector(b *testing.B) {
	rows := vpRows(100)
	w := NewWriter[vpRow](WithRowSelector(func(row int) bool { return row == 50 }))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		var sink bytes.Buffer
		sink.Grow(100 * 32)
		if err := w(&sink, seq2Of(rows)); err != nil {
			b.Fatal(err)
		}
	}
}
