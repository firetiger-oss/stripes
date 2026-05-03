package stripes

import (
	"io"
	"strings"

	"github.com/muesli/reflow/wordwrap"
)

func Text(w io.Writer, r io.Reader, styles *Styles) {
	s := new(strings.Builder)
	io.Copy(s, r)

	text := s.String()
	if styles.Width > 0 {
		text = wordwrap.String(text, styles.Width)
	}

	io.WriteString(w, styles.Text.Render(text))
}
