package gomod

import (
	"io"

	"github.com/firetiger-oss/stripes"
)

// GoWork renders a go.work file with ANSI styling applied to directive
// keywords (`go`, `toolchain`, `use`, `replace`, `godebug`), module and
// directory paths, versions, the `=>` replace operator, block
// parentheses, and `//` comments.
//
// Parsing is done via golang.org/x/mod/modfile.ParseWork; the token
// styling logic is shared with [GoMod].
func GoWork(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}
	renderModFile(w, data, styles, true)
}
