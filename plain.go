package stripes

import "io"

// Plain renders content without any formatting or styling
func Plain(w io.Writer, r io.Reader, _ *Styles) {
	io.Copy(w, r)
}
