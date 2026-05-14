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
func Text(w io.Writer, r io.Reader, styles *Styles) {
	s := new(strings.Builder)
	io.Copy(s, r)

	text := s.String()
	if styles.Width > 0 {
		text = wordwrap.String(text, styles.Width)
	}

	io.WriteString(w, styles.Text.Render(text))
}
