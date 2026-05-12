package stripes

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"golang.org/x/tools/txtar"
)

// Txtar renders a txtar archive: a free-text comment followed by one or
// more files, each introduced by a "-- name --" line.
//
// Each embedded file is dispatched through [Detect] + [Func] using its
// archive name, so a ".json" file is JSON-pretty-printed, a ".go" file
// is syntax-highlighted via chroma, and so on. Unknown formats fall back
// to [Plain].
//
// The archive comment is rendered as plain prose by default. If it looks
// like a [testscript] file — a leading non-comment, non-blank line whose
// first word is a known testscript command — the comment is highlighted
// with a small script grammar: `#` comments, testscript keywords,
// `--flag` arguments, quoted strings, and `[condition]` guards each get
// their own style. This makes Go-tooling txtar fixtures readable
// without changing the renderer for prose-style archives.
//
// [testscript]: https://pkg.go.dev/github.com/rogpeppe/go-internal/testscript
func Txtar(w io.Writer, r io.Reader, styles *Styles) {
	src, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}
	a := txtar.Parse(src)

	wroteAny := false
	if comment := strings.TrimRight(string(a.Comment), "\n"); comment != "" {
		renderTxtarComment(w, comment, styles)
		wroteAny = true
	}

	for _, f := range a.Files {
		if wroteAny {
			io.WriteString(w, "\n\n")
		}
		writeTxtarMarker(w, f.Name, styles)
		renderTxtarFile(w, f, styles)
		wroteAny = true
	}
}

// writeTxtarMarker emits a single "-- name --" header line, styled so the
// surrounding dashes read as syntax and the filename stands out as an
// anchor. No trailing newline — the caller positions the cursor for the
// content that follows.
func writeTxtarMarker(w io.Writer, name string, styles *Styles) {
	io.WriteString(w, styles.Syntax.Render("-- "))
	io.WriteString(w, styles.Anchor.Render(name))
	io.WriteString(w, styles.Syntax.Render(" --"))
}

// renderTxtarComment styles the archive's leading comment. Falls back to
// plain Text styling when the content doesn't look like testscript.
func renderTxtarComment(w io.Writer, comment string, styles *Styles) {
	if !looksLikeTestscript(comment) {
		var buf bytes.Buffer
		buf.WriteString(styles.Text.Render(comment))
		writeTrimmedLines(w, buf.Bytes())
		return
	}
	lines := strings.Split(comment, "\n")
	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		renderScriptLine(w, line, styles)
	}
}

// renderTxtarFile renders a single embedded file's contents using the
// renderer matched to its name. The output is buffered so we can:
//   - drop trailing whitespace (lipgloss block-renders, used by Text and a
//     few others, pad every line out to the longest line in the block;
//     that padding is invisible on a terminal but creates a stray blank
//     row and ragged right edges in the rendered txtar);
//   - guarantee the body ends without trailing newlines so the next
//     marker is separated by exactly one blank line.
func renderTxtarFile(w io.Writer, f txtar.File, styles *Styles) {
	if len(f.Data) == 0 {
		io.WriteString(w, "\n")
		return
	}
	peek := f.Data
	if len(peek) > 512 {
		peek = peek[:512]
	}
	renderer := Func(Detect(f.Name, peek), "")
	if renderer == nil {
		renderer = Plain
	}
	var buf bytes.Buffer
	renderer(&buf, bytes.NewReader(f.Data), styles)

	io.WriteString(w, "\n")
	writeTrimmedLines(w, buf.Bytes())
}

// writeTrimmedLines writes body to w with trailing whitespace stripped
// from each line and any trailing blank lines removed. ANSI escape
// sequences never contain '\n' or spaces past their final byte, so a
// simple per-line right-trim is safe even on colored output.
func writeTrimmedLines(w io.Writer, body []byte) {
	body = bytes.TrimRight(body, " \t\n")
	for len(body) > 0 {
		nl := bytes.IndexByte(body, '\n')
		var line []byte
		if nl < 0 {
			line, body = body, nil
		} else {
			line, body = body[:nl], body[nl+1:]
		}
		w.Write(bytes.TrimRight(line, " \t"))
		if nl >= 0 {
			io.WriteString(w, "\n")
		}
	}
}

// testscriptCommands is the set of built-in testscript verbs recognized
// by rogpeppe/go-internal/testscript. It is used by looksLikeTestscript
// to gate the script highlighter and by renderScriptLine to style the
// leading command keyword. Updates to testscript itself may add new
// verbs; missing ones still render as the leading keyword of a line, so
// the failure mode is "less specific styling," not "broken output".
var testscriptCommands = map[string]bool{
	"cd":      true,
	"chmod":   true,
	"cmp":     true,
	"cmpenv":  true,
	"cp":      true,
	"env":     true,
	"exec":    true,
	"exists":  true,
	"go":      true,
	"grep":    true,
	"mkdir":   true,
	"mv":      true,
	"rm":      true,
	"skip":    true,
	"stdin":   true,
	"stdout":  true,
	"stderr":  true,
	"stop":    true,
	"symlink": true,
	"unquote": true,
	"wait":    true,
}

// looksLikeTestscript returns true when the first directive-shaped line
// of comment is a known testscript command. A directive line is one
// whose first non-whitespace, non-`#`, non-`[condition]`, non-`!`
// token names a command. Empty or all-comment archives return false so
// they render as plain prose.
func looksLikeTestscript(comment string) bool {
	for _, line := range strings.Split(comment, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip any combination of leading "[condition]" guards and "!"
		// negation prefixes in either order. testscript syntax allows
		// "[short] ! exec foo" and "! [short] foo", and a comment line
		// may even introduce a directive after several condition guards.
		for {
			line = strings.TrimLeft(line, " \t")
			if strings.HasPrefix(line, "!") {
				line = line[1:]
				continue
			}
			if strings.HasPrefix(line, "[") {
				if end := strings.Index(line, "]"); end >= 0 {
					line = line[end+1:]
					continue
				}
			}
			break
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		return testscriptCommands[fields[0]]
	}
	return false
}

// renderScriptLine highlights a single line of testscript source. Leading
// whitespace is preserved verbatim; `#` runs the rest of the line as a
// comment; otherwise the first token is the command keyword and the
// remaining tokens are tokenized.
func renderScriptLine(w io.Writer, line string, styles *Styles) {
	leading, rest := splitLeadingWhitespace(line)
	io.WriteString(w, leading)
	if rest == "" {
		return
	}
	if strings.HasPrefix(rest, "#") {
		io.WriteString(w, styles.Comment.Render(rest))
		return
	}

	// "!" negation prefix, with or without following space.
	if rest[0] == '!' {
		io.WriteString(w, styles.Syntax.Render("!"))
		rest = rest[1:]
		if len(rest) > 0 && (rest[0] == ' ' || rest[0] == '\t') {
			io.WriteString(w, rest[:1])
			rest = rest[1:]
		}
	}

	// "[condition]" guard.
	if strings.HasPrefix(rest, "[") {
		if end := strings.Index(rest, "]"); end >= 0 {
			io.WriteString(w, styles.Anchor.Render(rest[:end+1]))
			rest = rest[end+1:]
			lead, after := splitLeadingWhitespace(rest)
			io.WriteString(w, lead)
			rest = after
		}
	}

	renderScriptTokens(w, rest, styles)
}

// renderScriptTokens applies per-token styling to a testscript argument
// list. The first non-whitespace token is treated as the command keyword
// (Name style). After that, quoted strings get the String style, `#`
// introduces a trailing comment, numbers get the Number style, and
// everything else (including flag-shaped tokens like `-count=1`) falls
// back to plain Text — coloring every flag added too much noise to lines
// that are mostly flag arguments.
func renderScriptTokens(w io.Writer, s string, styles *Styles) {
	first := true
	for i := 0; i < len(s); {
		c := s[i]
		if c == ' ' || c == '\t' {
			j := i
			for j < len(s) && (s[j] == ' ' || s[j] == '\t') {
				j++
			}
			io.WriteString(w, s[i:j])
			i = j
			continue
		}
		if c == '#' {
			io.WriteString(w, styles.Comment.Render(s[i:]))
			return
		}
		if c == '\'' || c == '"' || c == '`' {
			j := scanQuoted(s, i)
			io.WriteString(w, styles.String.Render(s[i:j]))
			first = false
			i = j
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' && s[j] != '\t' && s[j] != '\'' && s[j] != '"' && s[j] != '`' {
			j++
		}
		tok := s[i:j]
		switch {
		case first:
			// The first token is the command keyword regardless of whether
			// we know it: rendering an unknown verb as Name still reads as
			// "this is the action" and degrades gracefully when testscript
			// adds new commands.
			io.WriteString(w, styles.Name.Render(tok))
		case isNumber(tok):
			io.WriteString(w, styles.Number.Render(tok))
		default:
			io.WriteString(w, styles.Text.Render(tok))
		}
		first = false
		i = j
	}
}
