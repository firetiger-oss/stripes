package gomod

import (
	"io"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// RenderVendorModules writes a styled rendering of the vendor/modules.txt
// file read from r to w (the manifest produced by `go mod vendor`).
//
// Line shapes:
//   - `## <annotation>` — annotation lines like `## explicit` or
//     `## explicit; go 1.21`, styled as comments.
//   - `# <module> <version> [=> <replacement> [<version>]]` — module
//     declaration lines; the `#` and `=>` are syntax, paths are anchors,
//     versions are numbers.
//   - other non-blank lines — package import paths, styled as plain text.
//   - blank lines — preserved verbatim.
func RenderVendorModules(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}

	lines := splitLines(data)
	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		renderModulesTxtLine(w, line, styles)
	}
}

func renderModulesTxtLine(w io.Writer, line string, styles *stripes.Styles) {
	leading, rest := splitLeadingWhitespace(line)
	io.WriteString(w, leading)

	if rest == "" {
		return
	}

	if strings.HasPrefix(rest, "##") {
		io.WriteString(w, styles.Comment.Render(rest))
		return
	}

	if strings.HasPrefix(rest, "#") {
		renderModulesTxtModule(w, rest, styles)
		return
	}

	io.WriteString(w, styles.Text.Render(rest))
}

// renderModulesTxtModule styles a `# path version [=> repl [version]]`
// line. It walks the line token-by-token so original whitespace is
// preserved.
func renderModulesTxtModule(w io.Writer, s string, styles *stripes.Styles) {
	i := 0
	tokenIdx := 0
	sawArrow := false
	for i < len(s) {
		if isHorizontalSpace(s[i]) {
			j := scanSpace(s, i)
			io.WriteString(w, s[i:j])
			i = j
			continue
		}
		j := scanNonSpace(s, i)
		tok := s[i:j]
		i = j

		switch {
		case tok == "#":
			io.WriteString(w, styles.Syntax.Render(tok))
		case tok == "=>":
			io.WriteString(w, styles.Syntax.Render(tok))
			sawArrow = true
		case isModVersion(tok):
			io.WriteString(w, styles.Number.Render(tok))
		case tokenIdx == 1 || sawArrow && isModulePath(tok):
			io.WriteString(w, styles.Anchor.Render(tok))
		default:
			io.WriteString(w, styles.Text.Render(tok))
		}
		tokenIdx++
	}
}
