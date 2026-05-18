// Package diff registers the unified / git diff renderer with the
// stripes registry. Import for side effects to enable text/x-diff
// support:
//
//	import _ "github.com/firetiger-oss/stripes/diff"
package diff

import (
	"bytes"
	"io"

	"charm.land/lipgloss/v2"
	"github.com/bluekeyes/go-gitdiff/gitdiff"
	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "diff",
		ContentType: "text/x-diff",
		Extensions:  []string{".diff", ".patch"},
		Detect:      looksLikeDiff,
		RendererFor: stripes.Simple(Render),
	})
}

// Render writes a styled rendering of the unified or git diff read from
// r to w. Hunk headers, file headers, additions, deletions, context
// lines, and the "\ No newline at end of file" marker are each styled
// from the matching slot in styles.
//
// Parsing is delegated to github.com/bluekeyes/go-gitdiff so we don't
// reinvent unified-diff parsing; rendering is owned by stripes so the
// color slots stay consistent with the rest of the library. When the
// input does not parse as a diff at all (no files recognized), Render
// falls back to writing the bytes plain — the yaml renderer uses the
// same fall-back ethos. On successful parse the renderer colors the
// original input bytes line-by-line rather than re-serializing the
// parsed structure: this preserves quirks of the source (path
// prefixes, hunk-header comments, exact spacing) and avoids
// gitdiff's canonicalization of plain unified diffs.
func Render(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}
	files, _, _ := gitdiff.Parse(bytes.NewReader(data))
	if len(files) == 0 {
		w.Write(data)
		return
	}
	colorize(w, data, styles)
}

// colorize scans data line-by-line and writes each line to w under the
// style that matches its diff role (insertion, deletion, hunk header,
// file header, etc.). Trailing newlines on lines are preserved
// verbatim; the final line is emitted without a synthetic newline when
// the input did not end with one.
func colorize(w io.Writer, data []byte, styles *stripes.Styles) {
	for len(data) > 0 {
		nl := bytes.IndexByte(data, '\n')
		var line []byte
		if nl < 0 {
			line, data = data, nil
		} else {
			line, data = data[:nl], data[nl+1:]
		}
		io.WriteString(w, classify(line, styles).Render(string(line)))
		if nl >= 0 {
			io.WriteString(w, "\n")
		}
	}
}

// classify picks the style for one diff line by inspecting its prefix.
// The check for "+++" / "---" precedes the bare "+" / "-" checks so
// file-header lines stay in the Title style instead of being colored
// as additions or deletions.
func classify(line []byte, styles *stripes.Styles) lipgloss.Style {
	switch {
	case bytes.HasPrefix(line, []byte("+++ ")), bytes.HasPrefix(line, []byte("--- ")):
		return styles.Title
	case bytes.HasPrefix(line, []byte("+")):
		return styles.Insertion
	case bytes.HasPrefix(line, []byte("-")):
		return styles.Deletion
	case bytes.HasPrefix(line, []byte("@@")):
		return styles.Name
	case bytes.HasPrefix(line, []byte("\\ ")):
		return styles.Comment
	case isFileHeaderPrefix(line):
		return styles.Title
	default:
		return styles.Text
	}
}

// fileHeaderPrefixes are the leading tokens git uses on the metadata
// lines that frame a file's diff (everything other than the +++/---
// markers, which classify handles first).
var fileHeaderPrefixes = [][]byte{
	[]byte("diff "),
	[]byte("index "),
	[]byte("new file"),
	[]byte("deleted file"),
	[]byte("old mode"),
	[]byte("new mode"),
	[]byte("similarity index"),
	[]byte("dissimilarity index"),
	[]byte("rename from"),
	[]byte("rename to"),
	[]byte("copy from"),
	[]byte("copy to"),
	[]byte("Binary files"),
	[]byte("GIT binary patch"),
}

func isFileHeaderPrefix(line []byte) bool {
	for _, p := range fileHeaderPrefixes {
		if bytes.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// looksLikeDiff returns true when peek begins with content recognizable
// as unified or git diff: a `diff --git ` line, a `--- ` line that is
// followed by `+++ `, or a bare hunk header `@@ -... +... @@`.
// Heuristic on purpose; the parser does the real validation at render
// time.
func looksLikeDiff(peek []byte) bool {
	b := bytes.TrimLeft(peek, " \t\r\n")
	const maxScan = 4
	for i := 0; i < maxScan && len(b) > 0; i++ {
		nl := bytes.IndexByte(b, '\n')
		var line []byte
		if nl < 0 {
			line, b = b, nil
		} else {
			line, b = b[:nl], b[nl+1:]
		}
		line = bytes.TrimRight(line, " \t\r")
		if len(line) == 0 {
			continue
		}
		if bytes.HasPrefix(line, []byte("diff --git ")) {
			return true
		}
		if bytes.HasPrefix(line, []byte("--- ")) {
			next := bytes.TrimLeft(b, "\r\n")
			return bytes.HasPrefix(next, []byte("+++ "))
		}
		if isHunkHeader(line) {
			return true
		}
		return false
	}
	return false
}

// isHunkHeader matches `@@ -L[,N] +L[,N] @@[ section]`.
func isHunkHeader(line []byte) bool {
	if !bytes.HasPrefix(line, []byte("@@ -")) {
		return false
	}
	rest := line[len("@@ -"):]
	rest, ok := skipLineRange(rest)
	if !ok {
		return false
	}
	if !bytes.HasPrefix(rest, []byte(" +")) {
		return false
	}
	rest = rest[len(" +"):]
	rest, ok = skipLineRange(rest)
	if !ok {
		return false
	}
	return bytes.HasPrefix(rest, []byte(" @@"))
}

// skipLineRange consumes a `<digits>[,<digits>]` token at the head of b
// and returns the remainder. The optional second number is preceded by
// a comma. Returns ok=false when no leading digits are present.
func skipLineRange(b []byte) ([]byte, bool) {
	n := countDigits(b)
	if n == 0 {
		return b, false
	}
	b = b[n:]
	if len(b) > 0 && b[0] == ',' {
		b = b[1:]
		n = countDigits(b)
		if n == 0 {
			return b, false
		}
		b = b[n:]
	}
	return b, true
}

func countDigits(b []byte) int {
	i := 0
	for i < len(b) && b[i] >= '0' && b[i] <= '9' {
		i++
	}
	return i
}

