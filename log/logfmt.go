package log

import (
	"strings"

	"github.com/firetiger-oss/stripes"
)

// logfmtFormat renders logfmt-style structured logs (Go slog
// TextHandler, logrus's text formatter, Heroku-style):
//
//	time=2026-01-15T10:23:45Z level=INFO msg="user logged in" user=alice
//
// Detection is intentionally last in the registration order — the
// "two key=value tokens on the first line" predicate is permissive
// and would shadow more specific formats if it ran first.
//
// Reference: brandur.org/logfmt; Go slog package docs.
var logfmtFormat = LineFormat{
	Name:        "logfmt",
	ContentType: "application/vnd.logfmt",
	HasLevel:    true,
	Detect:      detectLogfmt,
	Format:      formatLogfmt,
}

func detectLogfmt(peek []byte) bool {
	lines := FirstNonEmptyLines(peek, 3)
	if len(lines) == 0 {
		return false
	}
	for _, line := range lines {
		if !looksLikeLogfmt(line) {
			return false
		}
	}
	return true
}

// looksLikeLogfmt returns true when line carries at least two
// key=value tokens with bare identifier keys (no spaces, no leading
// quotes/brackets/timestamps that would suggest a more specific
// format). Empty values and quoted values both count.
func looksLikeLogfmt(line string) bool {
	if line == "" {
		return false
	}
	c := line[0]
	if c == '[' || c == '<' || c == '{' || c == '"' {
		return false
	}
	// Reject lines that start with a timestamp pattern (already
	// claimed by python/log4j/go-log formats higher up).
	if isLikelyTimestampPrefix(line) {
		return false
	}
	pairs := parseLogfmtPairs(line)
	if len(pairs) < 2 {
		return false
	}
	good := 0
	for _, p := range pairs {
		if isBareIdent(p.key) {
			good++
		}
	}
	return good >= 2
}

// isLikelyTimestampPrefix returns true when line starts with one of
// the timestamp patterns claimed by the more specific log formats
// (Go-style slash dates, log4j-style dash dates, ISO 8601). Used by
// logfmt's Detect to step aside.
func isLikelyTimestampPrefix(line string) bool {
	if len(line) < 10 {
		return false
	}
	// Go: 2024/01/15 ...
	if line[4] == '/' && line[7] == '/' {
		return true
	}
	// ISO / log4j: 2024-01-15 ... or 2024-01-15T...
	if line[4] == '-' && line[7] == '-' && (line[10] == ' ' || line[10] == 'T') {
		return true
	}
	return false
}

// isBareIdent reports whether k is a plain identifier (alnum, dot,
// underscore, hyphen). Keys in logfmt are conventionally bare; any
// other shape signals a non-logfmt line that happened to contain "=".
func isBareIdent(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		c := k[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= 'A' && c <= 'Z':
		case c >= '0' && c <= '9':
		case c == '.' || c == '_' || c == '-':
		default:
			return false
		}
	}
	return true
}

type logfmtPair struct{ key, value string }

// parseLogfmtPairs splits line into key=value pairs. Quoted values
// (double-quoted, with backslash escapes) are unquoted in the
// returned value field. Tokens without "=" are skipped.
func parseLogfmtPairs(line string) []logfmtPair {
	var out []logfmtPair
	i := 0
	for i < len(line) {
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i >= len(line) {
			break
		}
		// key: up to '=' or space.
		ks := i
		for i < len(line) && line[i] != '=' && line[i] != ' ' {
			i++
		}
		key := line[ks:i]
		if i >= len(line) || line[i] != '=' {
			// bare token without =; skip
			continue
		}
		i++ // past '='
		if i >= len(line) {
			out = append(out, logfmtPair{key: key, value: ""})
			break
		}
		if line[i] == '"' {
			j := i + 1
			var b strings.Builder
			for j < len(line) {
				c := line[j]
				if c == '\\' && j+1 < len(line) {
					b.WriteByte(line[j+1])
					j += 2
					continue
				}
				if c == '"' {
					j++
					break
				}
				b.WriteByte(c)
				j++
			}
			out = append(out, logfmtPair{key: key, value: b.String()})
			i = j
			continue
		}
		vs := i
		for i < len(line) && line[i] != ' ' {
			i++
		}
		out = append(out, logfmtPair{key: key, value: line[vs:i]})
	}
	return out
}

func formatLogfmt(line string, styles *stripes.Styles) (Row, bool) {
	pairs := parseLogfmtPairs(line)
	if len(pairs) == 0 {
		return Row{}, false
	}

	row := Row{Attrs: make([]KV, 0, len(pairs))}
	for _, p := range pairs {
		switch p.key {
		case "time", "ts", "timestamp":
			row.Timestamp = p.value
		case "level", "lvl", "severity":
			row.Level = p.value
		case "msg", "message":
			row.Message = StyleText(styles).Render(p.value)
		default:
			val := p.value
			if val == "" {
				val = `""`
			}
			row.Attrs = append(row.Attrs, KV{Key: p.key, Value: val})
		}
	}
	return row, true
}
