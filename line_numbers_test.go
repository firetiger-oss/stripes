package stripes

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// passThrough is a minimal Renderer that copies src to w verbatim. It
// gives the tests deterministic content to wrap.
func passThrough(w io.Writer, src io.Reader, _ *Styles) {
	io.Copy(w, src)
}

func TestWithLineNumbersSingleLine(t *testing.T) {
	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader("hello\n"), &Styles{})
	got := buf.String()
	want := "1│ hello\n"
	if got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWithLineNumbersMultiLine(t *testing.T) {
	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader("a\nb\nc\n"), &Styles{})
	got := buf.String()
	want := "1│ a\n2│ b\n3│ c\n"
	if got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWithLineNumbersWidthBoundary(t *testing.T) {
	// 10 lines forces 2-digit column width; line 1 should be right-aligned.
	var src strings.Builder
	for i := 1; i <= 10; i++ {
		src.WriteString("x\n")
	}

	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader(src.String()), &Styles{})
	got := buf.String()

	wantLines := []string{
		" 1│ x",
		" 2│ x",
		" 3│ x",
		" 4│ x",
		" 5│ x",
		" 6│ x",
		" 7│ x",
		" 8│ x",
		" 9│ x",
		"10│ x",
	}
	want := strings.Join(wantLines, "\n") + "\n"
	if got != want {
		t.Fatalf("width-boundary output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWithLineNumbersEmptyInput(t *testing.T) {
	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader(""), &Styles{})
	if got := buf.String(); got != "" {
		t.Fatalf("empty input should produce empty output, got %q", got)
	}
}

func TestWithLineNumbersNoTrailingNewline(t *testing.T) {
	// Input doesn't end with \n. The adapter should still number every
	// non-empty line and emit a trailing newline.
	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader("a\nb"), &Styles{})
	got := buf.String()
	want := "1│ a\n2│ b\n"
	if got != want {
		t.Fatalf("no-trailing-newline output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWithLineNumbersTrailingNewlineNotPhantomLine(t *testing.T) {
	// "a\n" should produce only 1 line, not 1 + a phantom empty 2.
	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader("a\n"), &Styles{})
	got := buf.String()
	want := "1│ a\n"
	if got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestWithLineNumbersStyledOutputContainsANSI(t *testing.T) {
	styles := DefaultStyles.Clone()
	var buf bytes.Buffer
	WithLineNumbers(passThrough)(&buf, strings.NewReader("hello\n"), styles)

	if !bytes.Contains(buf.Bytes(), []byte{0x1b}) {
		t.Fatalf("expected ANSI escapes around line number, got plain: %q", buf.String())
	}
	stripped := ansi.Strip(buf.String())
	if stripped != "1│ hello\n" {
		t.Fatalf("stripped output mismatch\n got: %q\nwant: %q", stripped, "1│ hello\n")
	}
}

func TestWithLineNumbersPreservesInnerANSI(t *testing.T) {
	// Inner renderer that emits its own styled content. We verify that the
	// adapter doesn't strip or corrupt the inner ANSI sequences.
	innerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	inner := func(w io.Writer, _ io.Reader, _ *Styles) {
		io.WriteString(w, innerStyle.Render("red")+"\n")
	}

	var buf bytes.Buffer
	WithLineNumbers(inner)(&buf, strings.NewReader(""), DefaultStyles)

	stripped := ansi.Strip(buf.String())
	if stripped != "1│ red\n" {
		t.Fatalf("stripped output mismatch\n got: %q\nwant: %q", stripped, "1│ red\n")
	}
	// Make sure both line-number and inner styling are present (two
	// distinct ANSI runs separated by content).
	if bytes.Count(buf.Bytes(), []byte{0x1b}) < 2 {
		t.Fatalf("expected at least two ANSI escapes (gutter + content), got %q", buf.String())
	}
}

func TestDigitWidth(t *testing.T) {
	cases := []struct {
		in   int
		want int
	}{
		{0, 1},
		{1, 1},
		{9, 1},
		{10, 2},
		{99, 2},
		{100, 3},
		{999, 3},
		{1000, 4},
		{100000, 6},
	}
	for _, c := range cases {
		if got := digitWidth(c.in); got != c.want {
			t.Errorf("digitWidth(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}

// fixedRenderer returns a Renderer that copies a pre-built input verbatim
// to its output. Used by benchmarks so the inner renderer's cost is a
// flat io.Copy, isolating the adapter's overhead.
func fixedRenderer(payload []byte) Renderer {
	return func(w io.Writer, _ io.Reader, _ *Styles) {
		w.Write(payload)
	}
}

// benchPayload builds a buffer of n short lines.
func benchPayload(n int) []byte {
	var buf bytes.Buffer
	buf.Grow(n * 16)
	for i := 0; i < n; i++ {
		buf.WriteString("hello world line\n")
	}
	return buf.Bytes()
}

func benchmarkWithLineNumbers(b *testing.B, n int, styles *Styles) {
	payload := benchPayload(n)
	r := WithLineNumbers(fixedRenderer(payload))
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sink bytes.Buffer
		sink.Grow(len(payload) * 2)
		r(&sink, nil, styles)
	}
}

func BenchmarkWithLineNumbersSmall(b *testing.B) {
	benchmarkWithLineNumbers(b, 10, DefaultStyles)
}

func BenchmarkWithLineNumbersMedium(b *testing.B) {
	benchmarkWithLineNumbers(b, 1_000, DefaultStyles)
}

func BenchmarkWithLineNumbersLarge(b *testing.B) {
	benchmarkWithLineNumbers(b, 100_000, DefaultStyles)
}

func BenchmarkWithLineNumbersNoStyle(b *testing.B) {
	// Zero-value Styles → unstyled lipgloss.Style → no ANSI emitted by
	// Render. Isolates the cost of the gutter formatting itself.
	benchmarkWithLineNumbers(b, 1_000, &Styles{})
}
