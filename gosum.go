package stripes

import (
	"io"
	"strings"
)

// GoSum renders a go.sum (or go.work.sum) file with ANSI styling. Each
// non-empty line has the shape:
//
//	<module-path> <version>[/go.mod] <hash>
//
// The module path is styled as [Styles.Anchor], the version as
// [Styles.Number] (with the optional `/go.mod` suffix as [Styles.Syntax]),
// and the hash as [Styles.Code]. Lines that don't match this shape fall
// back to plain [Styles.Text] rendering.
func GoSum(w io.Writer, r io.Reader, styles *Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}

	lines := splitLines(data)
	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		renderGoSumLine(w, line, styles)
	}
}

func renderGoSumLine(w io.Writer, line string, styles *Styles) {
	leading, rest := splitLeadingWhitespace(line)
	io.WriteString(w, leading)

	if rest == "" {
		return
	}

	// Split into three whitespace-separated fields. We can't use
	// strings.Fields because it collapses internal whitespace; we want
	// to preserve the original separators so the rendered output is a
	// byte-for-byte stylistic overlay of the input.
	field1Start := 0
	field1End := scanNonSpace(rest, field1Start)
	sep1End := scanSpace(rest, field1End)
	field2Start := sep1End
	field2End := scanNonSpace(rest, field2Start)
	sep2End := scanSpace(rest, field2End)
	field3Start := sep2End
	field3End := scanNonSpace(rest, field3Start)
	// Allow trailing whitespace after the hash (rare but legal).
	trailingStart := field3End

	if field1End == field1Start || field2End == field2Start || field3End == field3Start ||
		field3End < len(rest) && hasMoreTokens(rest[field3End:]) {
		io.WriteString(w, styles.Text.Render(rest))
		return
	}

	path := rest[field1Start:field1End]
	sep1 := rest[field1End:sep1End]
	version := rest[field2Start:field2End]
	sep2 := rest[field2End:sep2End]
	hash := rest[field3Start:field3End]
	trailing := rest[trailingStart:]

	io.WriteString(w, styles.Anchor.Render(path))
	io.WriteString(w, sep1)
	if i := strings.Index(version, "/go.mod"); i >= 0 {
		io.WriteString(w, styles.Number.Render(version[:i]))
		io.WriteString(w, styles.Syntax.Render(version[i:]))
	} else {
		io.WriteString(w, styles.Number.Render(version))
	}
	io.WriteString(w, sep2)
	io.WriteString(w, styles.Code.Render(hash))
	io.WriteString(w, trailing)
}

func scanNonSpace(s string, i int) int {
	for i < len(s) && !isHorizontalSpace(s[i]) {
		i++
	}
	return i
}

func scanSpace(s string, i int) int {
	for i < len(s) && isHorizontalSpace(s[i]) {
		i++
	}
	return i
}

// hasMoreTokens reports whether s contains any non-whitespace bytes.
func hasMoreTokens(s string) bool {
	for i := 0; i < len(s); i++ {
		if !isHorizontalSpace(s[i]) {
			return true
		}
	}
	return false
}
