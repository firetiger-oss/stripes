// Package yaml registers the YAML renderer with the stripes registry.
// Import for side effects to enable application/yaml support:
//
//	import _ "github.com/firetiger-oss/stripes/yaml"
package yaml

import (
	"bytes"
	"io"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/clipperhouse/displaywidth"
	"github.com/firetiger-oss/stripes"
	"gopkg.in/yaml.v3"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "yaml",
		ContentType: "application/yaml",
		Extensions:  []string{".yaml", ".yml"},
		MagicBytes:  [][]byte{[]byte("---")},
		Detect:      looksLikeYAML,
		RendererFor: stripes.Simple(Render),
	})
}

// Render writes a styled rendering of the YAML read from r to w.
// Comments, anchors, aliases, flow- and block-styles, folded and
// literal scalars are all preserved.
func Render(w io.Writer, r io.Reader, styles *stripes.Styles) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}

	var node yaml.Node
	err = yaml.Unmarshal(data, &node)
	if err != nil {
		// If we can't parse as YAML, fall back to plain text
		w.Write(data)
		return
	}

	printYAMLNode(w, &node, 0, 0, styles)
}

func printYAMLNode(w io.Writer, node *yaml.Node, depth, inlinePrefix int, styles *stripes.Styles) {
	switch node.Kind {
	case yaml.DocumentNode:
		// Document node, render its content
		for _, child := range node.Content {
			printYAMLNode(w, child, depth, 0, styles)
		}

	case yaml.MappingNode:
		// Object/mapping
		for i := 0; i < len(node.Content); i += 2 {
			if i > 0 {
				io.WriteString(w, "\n")
			}

			keyNode := node.Content[i]
			valueNode := node.Content[i+1]

			// Render key with head comment
			if keyNode.HeadComment != "" {
				comment := keyNode.HeadComment
				if !strings.HasPrefix(comment, "#") {
					comment = "# " + comment
				}
				io.WriteString(w, styles.Comment.Render(comment))
				io.WriteString(w, "\n")
			}

			io.WriteString(w, styles.Name.Render(keyNode.Value))
			io.WriteString(w, styles.Syntax.Render(":"))

			// Columns consumed on the current row before an inline scalar
			// value would start: key + ":" + " " (+ "&anchor " if any).
			valuePrefix := ansi.StringWidth(keyNode.Value) + 2

			// Handle anchor definitions for the value
			if valueNode.Anchor != "" {
				io.WriteString(w, " ")
				io.WriteString(w, styles.Anchor.Render("&"+valueNode.Anchor))
				valuePrefix += 1 + 1 + len(valueNode.Anchor) // " " + "&" + anchor name
			}

			// Render value
			if valueNode.Kind == yaml.MappingNode || valueNode.Kind == yaml.SequenceNode {
				if valueNode.Anchor == "" {
					io.WriteString(w, " ")
				}
				io.WriteString(w, "\n")
				writer := stripes.NewPrefixWriter(w, indentStr(styles))
				printYAMLNode(writer, valueNode, depth+1, 0, styles)
			} else {
				io.WriteString(w, " ")
				printYAMLNode(w, valueNode, depth, valuePrefix, styles)
			}

			// Line comment for the key-value pair
			if keyNode.LineComment != "" {
				comment := keyNode.LineComment
				if !strings.HasPrefix(comment, "#") {
					comment = "# " + comment
				}
				io.WriteString(w, " ")
				io.WriteString(w, styles.Comment.Render(comment))
			}
		}

	case yaml.SequenceNode:
		// Array/sequence
		for i, child := range node.Content {
			if i > 0 {
				io.WriteString(w, "\n")
			}
			io.WriteString(w, styles.Syntax.Render("- "))

			// Handle anchor definitions for sequence items
			itemPrefix := 2 // "- "
			if child.Anchor != "" {
				io.WriteString(w, styles.Anchor.Render("&"+child.Anchor))
				io.WriteString(w, " ")
				itemPrefix += 1 + len(child.Anchor) + 1 // "&" + anchor + " "
			}

			if child.Kind == yaml.MappingNode || child.Kind == yaml.SequenceNode {
				io.WriteString(w, "\n")
				writer := stripes.NewPrefixWriter(w, indentStr(styles))
				printYAMLNode(writer, child, depth+1, 0, styles)
			} else {
				printYAMLNode(w, child, depth, itemPrefix, styles)
			}
		}

	case yaml.ScalarNode:
		// Scalar value (string, number, boolean, null)
		value := node.Value
		var styledValue string

		// Determine the appropriate style based on the value type.
		// Booleans match the YAML 1.1 keyword set (true/false plus
		// yes/no/on/off in any case) since real-world configs still use
		// those even when parsed under YAML 1.2.
		switch {
		case value == "null" || value == "~" || value == "":
			styledValue = styles.Null.Render(value)
		case isBoolKeyword(value):
			styledValue = styles.Boolean.Render(value)
		case isNumber(value):
			styledValue = styles.Number.Render(value)
		default:
			// Handle special YAML styles for text
			switch node.Style {
			case yaml.FoldedStyle:
				// Folded style (>)
				lines := strings.Split(value, "\n")
				var result strings.Builder
				result.WriteString(styles.Syntax.Render(">"))
				for _, line := range lines {
					if line != "" {
						result.WriteString("\n  " + styles.String.Render(line))
					} else {
						result.WriteString("\n")
					}
				}
				styledValue = result.String()
			case yaml.LiteralStyle:
				// Literal style (|)
				lines := strings.Split(value, "\n")
				var result strings.Builder
				result.WriteString(styles.Syntax.Render("|"))
				for _, line := range lines {
					result.WriteString("\n  " + styles.String.Render(line))
				}
				styledValue = result.String()
			default:
				// All non-special string scalars (plain and quoted alike)
				// render with the String style — they are strings in YAML
				// regardless of how the author chose to quote them.
				style := styles.String
				indentW := displaywidth.String(indentStr(styles))
				inlineWidth := depth*indentW + inlinePrefix + displaywidth.String(value)
				if styles.Width > 0 && inlineWidth > styles.Width {
					// Once we commit to folded style the value sits on its
					// own rows, so the wrap budget is independent of the
					// key length: Width minus the PrefixWriter indent
					// chain (depth*indentW) and the 2-char continuation
					// indent the fold layout adds itself.
					wrapWidth := styles.Width - depth*indentW - 2
					styledValue = foldScalar(value, style, wrapWidth, styles)
				} else {
					styledValue = style.Render(value)
				}
			}
		}

		io.WriteString(w, styledValue)

		// Handle comments
		if node.HeadComment != "" {
			comment := node.HeadComment
			if !strings.HasPrefix(comment, "#") {
				comment = " # " + comment
			} else {
				comment = " " + comment
			}
			io.WriteString(w, styles.Comment.Render(comment))
		}
		if node.LineComment != "" {
			comment := node.LineComment
			if !strings.HasPrefix(comment, "#") {
				comment = " # " + comment
			} else {
				comment = " " + comment
			}
			io.WriteString(w, styles.Comment.Render(comment))
		}

	case yaml.AliasNode:
		// YAML alias (*ref)
		io.WriteString(w, styles.Anchor.Render("*"+node.Value))
	}

	// Foot comment
	if node.FootComment != "" {
		comment := node.FootComment
		if !strings.HasPrefix(comment, "#") {
			comment = "# " + comment
		}
		io.WriteString(w, "\n")
		io.WriteString(w, styles.Comment.Render(comment))
	}
}

// isBoolKeyword reports whether s is one of YAML 1.1's boolean
// keywords (true/false, yes/no, on/off) in any letter case. yaml.v3
// parses under 1.2 — where only true/false are booleans — but real
// config files still use the 1.1 keywords, and rendering them with the
// Boolean style helps the reader spot toggles at a glance.
func isBoolKeyword(s string) bool {
	switch strings.ToLower(s) {
	case "true", "false", "yes", "no", "on", "off":
		return true
	}
	return false
}

// indentStr returns styles.Indent, defaulting to two spaces. The yaml
// renderer threads this through PrefixWriter for nested depth and uses
// its display width when computing fold-width budgets.
func indentStr(styles *stripes.Styles) string {
	if styles.Indent == "" {
		return "  "
	}
	return styles.Indent
}

// foldScalar renders value in YAML folded style: a leading ">" marker
// followed by each wrapped line on its own indented row. wrapWidth is
// the budget for the wrapped content (excludes the 2-char continuation
// indent that the folded layout adds itself). Style is applied
// per-line; ansi.Wrap operates on the raw text and is grapheme- and
// display-width-aware so the resulting lines fit a terminal of
// wrapWidth columns even with wide runes (em-dash, CJK, emoji).
func foldScalar(value string, style lipgloss.Style, wrapWidth int, styles *stripes.Styles) string {
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	wrapped := ansi.Wrap(value, wrapWidth, "")
	var b strings.Builder
	b.WriteString(styles.Syntax.Render(">"))
	for _, line := range strings.Split(wrapped, "\n") {
		if line == "" {
			b.WriteString("\n")
		} else {
			b.WriteString("\n  ")
			b.WriteString(style.Render(line))
		}
	}
	return b.String()
}

// isNumber checks if a string represents a numeric scalar (int, uint,
// or float). Used to pick the Number style for YAML scalars.
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

// looksLikeYAML returns true when peek shows YAML's `key:` shape on the
// first non-comment, non-blank line. Permissive on purpose; keep this
// last in the detection chain.
func looksLikeYAML(peek []byte) bool {
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
		if len(line) == 0 || line[0] == '#' {
			if b == nil {
				break
			}
			continue
		}
		if !isASCIIIdentStart(line[0]) {
			return false
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			return false
		}
		for j := 0; j < colon; j++ {
			c := line[j]
			if !isASCIIIdent(c) {
				return false
			}
		}
		after := line[colon+1:]
		if len(after) == 0 || after[0] == ' ' || after[0] == '\t' {
			return true
		}
		return false
	}
	return false
}

func isASCIIIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isASCIIIdent(c byte) bool {
	return isASCIIIdentStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.'
}
