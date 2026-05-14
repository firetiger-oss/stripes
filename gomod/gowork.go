package gomod

import (
	"io"

	"github.com/firetiger-oss/stripes"
)

// RenderWork writes a styled rendering of the go.work file read from r
// to w, with ANSI styling applied to directive keywords (`go`,
// `toolchain`, `use`, `replace`, `godebug`), module and directory
// paths, versions, the `=>` replace operator, block parentheses, and
// `//` comments.
//
// Parsing is done via golang.org/x/mod/modfile.ParseWork; the token
// styling logic is shared with [RenderMod].
func RenderWork(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}
	renderModFile(w, data, styles, true)
}
