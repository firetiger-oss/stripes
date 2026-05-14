package stripes

import (
	"bytes"
	"io"
	"strings"

	"charm.land/lipgloss/v2"
)

// WithLineNumbers returns a Renderer that wraps r and prepends a
// right-aligned, styled line number to every output line. Each rendered
// line is prefixed with the styled gutter "<digits>│" followed by a
// single unstyled space before the content, e.g. " 12│ content". The
// column width is sized to fit the largest line number, so all numbers
// align in a single fixed column. Styling is taken from
// styles.LineNumber.
//
// For renderers whose output preserves a 1:1 line correspondence with the
// input (Code, Text, Plain, Dockerfile, Markdown, HTML, XML, YAML), the
// numbers correspond to source input lines. For reformatting renderers
// (JSON pretty-print, CSV table) the numbers run sequentially over the
// rendered output.
//
// The inner renderer's output is buffered in memory to determine the
// column width before any bytes are written to w. This is consistent with
// how stripes typically renders human-viewed files.
func WithLineNumbers(r Renderer) Renderer {
	return func(w io.Writer, src io.Reader, styles *Styles) {
		var buf bytes.Buffer
		r(&buf, src, styles)

		data := buf.Bytes()
		if len(data) == 0 {
			return
		}
		// Don't number a phantom empty line if the output ends with \n.
		if data[len(data)-1] == '\n' {
			data = data[:len(data)-1]
		}

		lineCount := 1
		for _, b := range data {
			if b == '\n' {
				lineCount++
			}
		}
		width := digitWidth(lineCount)

		// Reusable scratch buffer holding "<digits>⏐". The unstyled space
		// after the separator is written separately so it stays plain.
		const sep = "│"
		numBuf := make([]byte, width+len(sep))
		copy(numBuf[width:], sep)

		// Pre-render the ANSI prefix/suffix once. For simple styles
		// (foreground/background only, no layout transformations) lipgloss
		// emits content sandwiched between an SGR open sequence and a
		// reset, so we can write the digits directly between the two and
		// avoid allocating a fresh string per line.
		ansiPrefix, ansiSuffix, ok := splitGutterStyle(styles.LineNumber, len(numBuf))

		n := 0
		for i := 0; i < len(data); {
			n++
			v := n
			for j := width - 1; j >= 0; j-- {
				if v > 0 {
					numBuf[j] = byte('0' + v%10)
					v /= 10
				} else {
					numBuf[j] = ' '
				}
			}
			if ok {
				io.WriteString(w, ansiPrefix)
				w.Write(numBuf)
				io.WriteString(w, ansiSuffix)
			} else {
				io.WriteString(w, styles.LineNumber.Render(string(numBuf)))
			}
			io.WriteString(w, " ")

			end := bytes.IndexByte(data[i:], '\n')
			if end < 0 {
				w.Write(data[i:])
				io.WriteString(w, "\n")
				return
			}
			w.Write(data[i : i+end+1])
			i += end + 1
		}
	}
}

// splitGutterStyle pre-renders style around a sentinel of the gutter's
// expected width and, if the result is a simple "prefix + content +
// suffix" wrapping, returns those parts so the caller can stream digits
// without re-running lipgloss for every line. ok is false when the style
// does anything content-dependent (padding, width, alignment, transform).
func splitGutterStyle(style lipgloss.Style, contentLen int) (prefix, suffix string, ok bool) {
	sentinel := strings.Repeat("\x07", contentLen)
	rendered := style.Render(sentinel)
	idx := strings.Index(rendered, sentinel)
	if idx < 0 {
		return "", "", false
	}
	if strings.Contains(rendered[idx+len(sentinel):], sentinel) {
		// Sentinel appears more than once → can't safely split.
		return "", "", false
	}
	return rendered[:idx], rendered[idx+len(sentinel):], true
}

func digitWidth(n int) int {
	if n < 1 {
		return 1
	}
	w := 0
	for n > 0 {
		w++
		n /= 10
	}
	return w
}
