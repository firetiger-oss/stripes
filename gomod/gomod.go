// Package gomod registers stripes renderers for the Go toolchain's
// flat-file formats: go.mod, go.sum, go.work, and vendor/modules.txt.
// Import for side effects:
//
//	import _ "github.com/firetiger-oss/stripes/gomod"
package gomod

import (
	"bufio"
	"bytes"
	"io"
	"path/filepath"
	"strings"

	"github.com/firetiger-oss/stripes"
	"golang.org/x/mod/modfile"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "gomod",
		ContentType: "text/x-go-mod",
		Filenames:   []string{"go.mod"},
		RendererFor: stripes.Simple(RenderMod),
	})
	stripes.Register(stripes.Format{
		Name:        "gosum",
		ContentType: "text/x-go-sum",
		Filenames:   []string{"go.sum", "go.work.sum"},
		RendererFor: stripes.Simple(RenderSum),
	})
	stripes.Register(stripes.Format{
		Name:        "gowork",
		ContentType: "text/x-go-work",
		Filenames:   []string{"go.work"},
		RendererFor: stripes.Simple(RenderWork),
	})
	stripes.Register(stripes.Format{
		Name:        "modulestxt",
		ContentType: "text/x-go-vendor-modules",
		MatchPath: func(p string) bool {
			return filepath.Base(p) == "modules.txt" && filepath.Base(filepath.Dir(p)) == "vendor"
		},
		RendererFor: stripes.Simple(RenderVendorModules),
	})
}

// splitLines splits input on '\n', preserving every line including
// blanks; a trailing newline does not produce an extra empty line.
func splitLines(data []byte) []string {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	return lines
}

func splitLeadingWhitespace(line string) (leading, rest string) {
	i := 0
	for i < len(line) && isHorizontalSpace(line[i]) {
		i++
	}
	return line[:i], line[i:]
}

func isHorizontalSpace(c byte) bool {
	return c == ' ' || c == '\t'
}

// scanQuoted returns the index past the closing quote of a string
// opened at s[i]. The opening quote character at s[i] determines the
// closer; backslash escapes inside are consumed. If no closing quote is
// found, len(s) is returned.
func scanQuoted(s string, i int) int {
	quote := s[i]
	j := i + 1
	for j < len(s) {
		if s[j] == '\\' && j+1 < len(s) {
			j += 2
			continue
		}
		if s[j] == quote {
			return j + 1
		}
		j++
	}
	return len(s)
}

// RenderMod writes a styled rendering of the go.mod file read from r to
// w, with ANSI styling applied to directive keywords, module paths,
// versions, the `=>` replace operator, block parentheses, and `//`
// comments.
//
// Parsing is done via golang.org/x/mod/modfile (lax mode), which handles
// block syntax, factored require/replace/exclude/retract/tool lists,
// inline comments, quoted module paths, and `// indirect` markers the
// same way the Go toolchain does.
func RenderMod(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}
	renderModFile(w, data, styles, false)
}

// renderModFile is the shared implementation for both go.mod and go.work
// rendering. work selects modfile.ParseWork; the token styling is
// identical between the two grammars.
func renderModFile(w io.Writer, data []byte, styles *stripes.Styles, work bool) {
	lines := splitLines(data)

	var syntax *modfile.FileSyntax
	if work {
		if f, _ := modfile.ParseWork("", data, nil); f != nil {
			syntax = f.Syntax
		}
	} else {
		if f, _ := modfile.ParseLax("", data, nil); f != nil {
			syntax = f.Syntax
		}
	}

	directive := make([]string, len(lines)+1) // 1-indexed
	if syntax != nil {
		classifyModFileStmts(syntax.Stmt, directive)
	}

	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		renderModFileLine(w, line, directive[i+1], styles)
	}
}

// classifyModFileStmts walks the parsed AST and labels each input line
// with the directive keyword that owns it. A line with a non-empty
// directive is the line that introduces a directive (e.g. `require`,
// `module`); body lines inside `(` blocks get an empty directive and are
// rendered without keyword styling.
func classifyModFileStmts(stmts []modfile.Expr, directive []string) {
	for _, stmt := range stmts {
		switch s := stmt.(type) {
		case *modfile.Line:
			if s.InBlock {
				continue
			}
			if ln := s.Start.Line; ln >= 1 && ln < len(directive) && len(s.Token) > 0 {
				directive[ln] = s.Token[0]
			}
		case *modfile.LineBlock:
			if ln := s.Start.Line; ln >= 1 && ln < len(directive) && len(s.Token) > 0 {
				directive[ln] = s.Token[0]
			}
		}
	}
}

// renderModFileLine emits a single line of a go.mod / go.work file with
// styling applied. directive is the keyword that introduces this line
// (empty for block body lines, comments, and blanks). The original
// whitespace of the input line is preserved verbatim.
func renderModFileLine(w io.Writer, line, directive string, styles *stripes.Styles) {
	leading, rest := splitLeadingWhitespace(line)
	io.WriteString(w, leading)

	if rest == "" {
		return
	}

	// Trailing comment: split at the first `//` that isn't inside a
	// string. Module paths in go.mod files don't contain `//`, so a
	// simple search is sufficient.
	code, comment := splitModFileComment(rest)

	firstTokenIsKeyword := directive != ""
	emitModFileTokens(w, code, firstTokenIsKeyword, styles)
	if comment != "" {
		io.WriteString(w, styles.Comment.Render(comment))
	}
}

// splitModFileComment splits a line into its code prefix and trailing
// `// ...` comment (if any). The comment, when present, includes the
// leading whitespace before `//`.
func splitModFileComment(s string) (code, comment string) {
	inString := false
	for i := 0; i < len(s)-1; i++ {
		c := s[i]
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if c == '/' && s[i+1] == '/' {
			// Include any whitespace immediately before `//` in the
			// comment so the styled segment looks contiguous.
			j := i
			for j > 0 && (s[j-1] == ' ' || s[j-1] == '\t') {
				j--
			}
			return s[:j], s[j:]
		}
	}
	return s, ""
}

// emitModFileTokens walks the code portion of a line token-by-token and
// applies styling. Whitespace runs are emitted verbatim; non-whitespace
// runs (including standalone `(` `)` and `=>`) are classified by shape
// and rendered through the matching style.
func emitModFileTokens(w io.Writer, s string, firstTokenIsKeyword bool, styles *stripes.Styles) {
	first := firstTokenIsKeyword
	i := 0
	for i < len(s) {
		if isHorizontalSpace(s[i]) {
			j := i
			for j < len(s) && isHorizontalSpace(s[j]) {
				j++
			}
			io.WriteString(w, s[i:j])
			i = j
			continue
		}

		if s[i] == '"' {
			j := scanQuoted(s, i)
			io.WriteString(w, styles.String.Render(s[i:j]))
			i = j
			first = false
			continue
		}

		j := i
		for j < len(s) && !isHorizontalSpace(s[j]) && s[j] != '"' {
			j++
		}
		tok := s[i:j]
		i = j

		switch {
		case first:
			io.WriteString(w, styles.Name.Render(tok))
		case tok == "(" || tok == ")":
			io.WriteString(w, styles.Syntax.Render(tok))
		case tok == "=>":
			io.WriteString(w, styles.Syntax.Render(tok))
		case isModVersion(tok):
			io.WriteString(w, styles.Number.Render(tok))
		case isModulePath(tok):
			io.WriteString(w, styles.Anchor.Render(tok))
		default:
			io.WriteString(w, styles.Text.Render(tok))
		}
		first = false
	}
}

// isModVersion reports whether tok looks like a semver or pseudo-version
// (e.g. v1.2.3, v1.2.3-rc1, v0.0.0-20240101120000-abcdef123456) or a
// bare Go-language version (e.g. 1.22, 1.22.3). It also accepts version
// intervals used by retract directives ([v1, v2]).
func isModVersion(tok string) bool {
	if tok == "" {
		return false
	}
	// Stripped version intervals like [v1, or v2] in retract lines.
	switch tok[0] {
	case '[', ']':
		return false
	}
	if tok[0] == 'v' && len(tok) > 1 && tok[1] >= '0' && tok[1] <= '9' {
		return true
	}
	// Bare Go language version: digit(.digit)+
	if tok[0] >= '0' && tok[0] <= '9' {
		hasDot := false
		for _, c := range tok {
			switch {
			case c >= '0' && c <= '9':
			case c == '.':
				hasDot = true
			default:
				return false
			}
		}
		return hasDot
	}
	return false
}

// isModulePath reports whether tok looks like a module path or a path
// to a local module (relative `./...` or `../...`). Module paths in
// go.mod always contain at least one `/`; relative replace targets are
// rooted at `.` or `..`.
func isModulePath(tok string) bool {
	if strings.HasPrefix(tok, "./") || strings.HasPrefix(tok, "../") {
		return true
	}
	if tok == "." || tok == ".." {
		return true
	}
	return strings.Contains(tok, "/")
}
