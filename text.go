package stripes

import (
	"io"
	"strings"

	"github.com/muesli/reflow/wordwrap"
)

// The root package registers text/plain itself: Text is the built-in
// fallback renderer, always available without importing a sub-package.
// This also lets [Detect] resolve the ".txt" extension.
func init() {
	Register(Format{
		Name:        "text",
		ContentType: "text/plain",
		Extensions:  []string{".txt"},
		RendererFor: Simple(Text),
	})
}

// Text renders a stream as plain styled text, word-wrapping to
// [Styles.Width] when it is positive.
//
// Rendering walks the input one newline-delimited segment at a time. A
// single Render call on multi-line input pads every line with trailing
// spaces to match the longest line in the block (lipgloss v2 behaviour),
// which then wraps in narrow terminals and looks like a blank line between
// every source line. Per-line Render avoids that.
func Text(w io.Writer, r io.Reader, styles *Styles) {
	s := new(strings.Builder)
	io.Copy(s, r)

	text := s.String()
	if styles.Width > 0 {
		text = wordwrap.String(text, styles.Width)
	}

	for {
		nl := strings.IndexByte(text, '\n')
		if nl < 0 {
			if text != "" {
				io.WriteString(w, styles.Text.Render(text))
			}
			return
		}
		if nl > 0 {
			io.WriteString(w, styles.Text.Render(text[:nl]))
		}
		io.WriteString(w, "\n")
		text = text[nl+1:]
	}
}
