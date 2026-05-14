// Package dockerfile registers the Dockerfile renderer with the stripes
// registry. Import for side effects to enable text/x-dockerfile support:
//
//	import _ "github.com/firetiger-oss/stripes/dockerfile"
package dockerfile

import (
	"bufio"
	"bytes"
	"io"
	"strconv"
	"strings"

	"github.com/firetiger-oss/stripes"
	"github.com/moby/buildkit/frontend/dockerfile/parser"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "dockerfile",
		ContentType: "text/x-dockerfile",
		Filenames:   []string{"Dockerfile", "Containerfile"},
		Extensions:  []string{".dockerfile"},
		Detect:      looksLikeDockerfile,
		RendererFor: stripes.Simple(Render),
	})
}

// looksLikeDockerfile inspects the first few non-blank lines of peek
// for Dockerfile-shaped content: a `# syntax=` / `# escape=` parser
// directive, or a leading FROM/ARG instruction.
func looksLikeDockerfile(peek []byte) bool {
	b := bytes.TrimLeft(peek, " \t\r\n")
	const maxScan = 4
	for i, line := 0, []byte(nil); i < maxScan; i++ {
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			line, b = b, nil
		} else {
			line, b = b[:nl], b[nl+1:]
		}
		line = bytes.TrimRight(line, " \t\r")
		if len(line) == 0 {
			if b == nil {
				break
			}
			continue
		}
		if line[0] == '#' {
			rest := bytes.TrimLeft(line[1:], " \t")
			lower := bytes.ToLower(rest)
			if bytes.HasPrefix(lower, []byte("syntax=")) ||
				bytes.HasPrefix(lower, []byte("escape=")) ||
				bytes.HasPrefix(lower, []byte("check=")) {
				return true
			}
			if b == nil {
				break
			}
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) == 0 {
			if b == nil {
				break
			}
			continue
		}
		head := bytes.ToUpper(fields[0])
		switch string(head) {
		case "FROM", "ARG":
			return true
		}
		return false
	}
	return false
}

// isNumber returns true when s parses as int, uint, or float — used to
// style numeric tokens in Dockerfile flag arguments.
func isNumber(s string) bool {
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

// Render writes a styled rendering of the Dockerfile (or Containerfile)
// read from r to w, with ANSI styling applied to instruction keywords,
// comments, flags, quoted strings, heredoc bodies, and line-continuation
// backslashes.
//
// Parsing is done via github.com/moby/buildkit/frontend/dockerfile/parser
// so that line continuations, escape directives, parser directives, and
// heredocs are handled the same way docker build itself handles them.
func Render(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}

	lines := splitLines(data)

	result, perr := parser.Parse(bytes.NewReader(data))
	if perr != nil {
		// Best-effort: render input verbatim through Text style.
		writeDockerfileLines(w, lines, func(_ int, line string) {
			io.WriteString(w, styles.Text.Render(line))
		})
		return
	}

	const (
		kindUnclassified = iota
		kindInstrStart
		kindInstrCont
		kindHeredocBody
	)

	n := len(lines)
	kind := make([]int, n+2) // 1-indexed; +2 to allow safe lookahead

	classifyDockerfileNodes(result.AST, lines, kind)

	writeDockerfileLines(w, lines, func(idx int, line string) {
		ln := idx + 1
		switch kind[ln] {
		case kindHeredocBody:
			io.WriteString(w, styles.String.Render(line))
		case kindInstrStart:
			renderDockerfileInstructionLine(w, line, styles)
		case kindInstrCont:
			renderDockerfileArgs(w, line, styles)
		default:
			trimmed := strings.TrimLeft(line, " \t")
			switch {
			case trimmed == "":
				io.WriteString(w, line)
			case strings.HasPrefix(trimmed, "#"):
				io.WriteString(w, styles.Comment.Render(line))
			default:
				io.WriteString(w, styles.Text.Render(line))
			}
		}
	})
}

// classifyDockerfileNodes walks the parser's AST and labels each input
// line with what it represents (instruction start, continuation, or
// heredoc body). Lines outside any node range stay unclassified and are
// treated as comments or blanks at render time.
func classifyDockerfileNodes(root *parser.Node, lines []string, kind []int) {
	if root == nil {
		return
	}

	const (
		kindInstrStart  = 1
		kindInstrCont   = 2
		kindHeredocBody = 3
	)

	totalLines := len(lines)

	for _, node := range root.Children {
		if node.StartLine <= 0 || node.StartLine > totalLines {
			continue
		}
		kind[node.StartLine] = kindInstrStart
		end := node.EndLine
		if end > totalLines {
			end = totalLines
		}
		for ln := node.StartLine + 1; ln <= end; ln++ {
			kind[ln] = kindInstrCont
		}

		// For each heredoc, find the closing-delimiter line and mark
		// the lines between (start, close) as heredoc body. The closing
		// delimiter line itself stays as kindInstrCont.
		for _, hd := range node.Heredocs {
			if hd.Name == "" {
				continue
			}
			closeLine := findHeredocClose(lines, node.StartLine+1, end, hd.Name)
			if closeLine < 0 {
				continue
			}
			for ln := node.StartLine + 1; ln < closeLine; ln++ {
				if kind[ln] == kindInstrCont {
					kind[ln] = kindHeredocBody
				}
			}
		}
	}
}

func findHeredocClose(lines []string, startLine, endLine int, name string) int {
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	for ln := startLine; ln <= endLine; ln++ {
		trimmed := strings.TrimSpace(lines[ln-1])
		if trimmed == name {
			return ln
		}
	}
	return -1
}

// renderDockerfileInstructionLine emits a line that starts an
// instruction: leading whitespace, keyword styled with Name, then args.
func renderDockerfileInstructionLine(w io.Writer, line string, styles *stripes.Styles) {
	leading, rest := splitLeadingWhitespace(line)
	io.WriteString(w, leading)

	end := 0
	for end < len(rest) && !isHorizontalSpace(rest[end]) {
		end++
	}
	if end == 0 {
		// No keyword found — fall back to args rendering.
		renderDockerfileArgs(w, rest, styles)
		return
	}
	io.WriteString(w, styles.Name.Render(rest[:end]))
	renderDockerfileArgs(w, rest[end:], styles)
}

// renderDockerfileArgs walks args byte-by-byte, preserving whitespace
// runs verbatim and applying styles per token.
func renderDockerfileArgs(w io.Writer, s string, styles *stripes.Styles) {
	nextIsStageAlias := false

	i := 0
	for i < len(s) {
		c := s[i]

		if isHorizontalSpace(c) {
			j := i
			for j < len(s) && isHorizontalSpace(s[j]) {
				j++
			}
			io.WriteString(w, s[i:j])
			i = j
			continue
		}

		// Trailing line-continuation marker (\ or `): when it's the
		// last non-whitespace byte of the line, color it as syntax.
		if (c == '\\' || c == '`') && isLineContinuationByte(s, i) {
			io.WriteString(w, styles.Syntax.Render(string(c)))
			io.WriteString(w, s[i+1:])
			return
		}

		if c == '"' || c == '\'' {
			j := scanQuoted(s, i)
			io.WriteString(w, styles.String.Render(s[i:j]))
			i = j
			continue
		}

		j := i
		for j < len(s) && !isHorizontalSpace(s[j]) && s[j] != '"' && s[j] != '\'' {
			j++
		}
		tok := s[i:j]

		switch {
		case strings.HasPrefix(tok, "--"):
			io.WriteString(w, styles.Anchor.Render(tok))
		case strings.EqualFold(tok, "AS"):
			io.WriteString(w, styles.Name.Render(tok))
			nextIsStageAlias = true
			i = j
			continue
		case nextIsStageAlias:
			io.WriteString(w, styles.Anchor.Render(tok))
		case isNumber(tok):
			io.WriteString(w, styles.Number.Render(tok))
		default:
			io.WriteString(w, styles.Text.Render(tok))
		}
		nextIsStageAlias = false
		i = j
	}
}

// writeDockerfileLines invokes render for each line, separating with
// "\n". No trailing newline is written; the CLI's trailingNewlineWriter
// adds one if needed.
func writeDockerfileLines(w io.Writer, lines []string, render func(idx int, line string)) {
	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		render(i, line)
	}
}

// splitLines splits input on '\n' preserving every line including
// blanks. A trailing newline produces a trailing empty line which we
// drop so we don't emit a spurious blank line at the end.
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

func isLineContinuationByte(s string, i int) bool {
	for j := i + 1; j < len(s); j++ {
		if !isHorizontalSpace(s[j]) {
			return false
		}
	}
	return true
}

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
