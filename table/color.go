package table

import (
	"strings"

	"github.com/firetiger-oss/stripes"
)

// colorizeJSON applies inline ANSI styling per token to compact JSON. It is
// tolerant of malformed / truncated input (which happens when fitToWidth
// trims a cell mid-value): unrecognised bytes are emitted unchanged so the
// caller sees something readable even when the JSON parser would refuse.
//
// Token → style mapping follows stripes.JSON:
//
//	{}[],:  → Syntax     (punctuation)
//	keys    → Name       (object property names)
//	strings → String     (string values)
//	numbers → Number
//	bool    → Boolean
//	null    → Null
func colorizeJSON(s string, styles *stripes.Styles) string {
	if s == "" {
		return s
	}
	var (
		out      strings.Builder
		objStack []bool // top of stack: true → next string is a key
	)
	out.Grow(len(s) + 32)
	n := len(s)
	i := 0
	for i < n {
		c := s[i]
		switch {
		case c == '{':
			out.WriteString(styles.Syntax.Render("{"))
			objStack = append(objStack, true)
			i++
		case c == '}':
			out.WriteString(styles.Syntax.Render("}"))
			if len(objStack) > 0 {
				objStack = objStack[:len(objStack)-1]
			}
			i++
		case c == '[':
			out.WriteString(styles.Syntax.Render("["))
			i++
		case c == ']':
			out.WriteString(styles.Syntax.Render("]"))
			i++
		case c == ',':
			out.WriteString(styles.Syntax.Render(","))
			if len(objStack) > 0 {
				objStack[len(objStack)-1] = true
			}
			i++
		case c == ':':
			out.WriteString(styles.Syntax.Render(":"))
			if len(objStack) > 0 {
				objStack[len(objStack)-1] = false
			}
			i++
		case c == '"':
			end := i + 1
			for end < n {
				if s[end] == '\\' && end+1 < n {
					end += 2
					continue
				}
				if s[end] == '"' {
					end++
					break
				}
				end++
			}
			tok := s[i:end]
			isKey := len(objStack) > 0 && objStack[len(objStack)-1]
			if isKey {
				out.WriteString(styles.Name.Render(tok))
			} else {
				out.WriteString(styles.String.Render(tok))
			}
			i = end
		case strings.HasPrefix(s[i:], "true"):
			out.WriteString(styles.Boolean.Render("true"))
			i += 4
		case strings.HasPrefix(s[i:], "false"):
			out.WriteString(styles.Boolean.Render("false"))
			i += 5
		case strings.HasPrefix(s[i:], "null"):
			out.WriteString(styles.Null.Render("null"))
			i += 4
		case c == '-' || c == '+' || (c >= '0' && c <= '9'):
			end := i + 1
			for end < n {
				cc := s[end]
				if (cc >= '0' && cc <= '9') || cc == '.' || cc == 'e' || cc == 'E' || cc == '+' || cc == '-' {
					end++
					continue
				}
				break
			}
			out.WriteString(styles.Number.Render(s[i:end]))
			i = end
		default:
			// Unknown rune (whitespace, ellipsis after truncation, etc.).
			out.WriteByte(c)
			i++
		}
	}
	return out.String()
}
