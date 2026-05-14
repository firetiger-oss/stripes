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

	printYAMLNode(w, &node, 0, styles)
}

func printYAMLNode(w io.Writer, node *yaml.Node, depth int, styles *stripes.Styles) {
	switch node.Kind {
	case yaml.DocumentNode:
		// Document node, render its content
		for _, child := range node.Content {
			printYAMLNode(w, child, depth, styles)
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

			// Handle anchor definitions for the value
			if valueNode.Anchor != "" {
				io.WriteString(w, " ")
				io.WriteString(w, styles.Anchor.Render("&"+valueNode.Anchor))
			}

			// Render value
			if valueNode.Kind == yaml.MappingNode || valueNode.Kind == yaml.SequenceNode {
				if valueNode.Anchor == "" {
					io.WriteString(w, " ")
				}
				io.WriteString(w, "\n")
				indentStr := styles.Indent
				if indentStr == "" {
					indentStr = "  "
				}
				writer := stripes.NewPrefixWriter(w, indentStr)
				printYAMLNode(writer, valueNode, depth+1, styles)
			} else {
				io.WriteString(w, " ")
				printYAMLNode(w, valueNode, depth, styles)
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
			if child.Anchor != "" {
				io.WriteString(w, styles.Anchor.Render("&"+child.Anchor))
				io.WriteString(w, " ")
			}

			if child.Kind == yaml.MappingNode || child.Kind == yaml.SequenceNode {
				io.WriteString(w, "\n")
				indentStr := styles.Indent
				if indentStr == "" {
					indentStr = "  "
				}
				writer := stripes.NewPrefixWriter(w, indentStr)
				printYAMLNode(writer, child, depth+1, styles)
			} else {
				printYAMLNode(w, child, depth, styles)
			}
		}

	case yaml.ScalarNode:
		// Scalar value (string, number, boolean, null)
		value := node.Value
		var styledValue string

		// Determine the appropriate style based on the value type
		switch {
		case value == "null" || value == "~" || value == "":
			styledValue = styles.Null.Render(value)
		case value == "true" || value == "false":
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
				// For regular strings, apply appropriate styling based on quote style
				if node.Style == yaml.DoubleQuotedStyle || node.Style == yaml.SingleQuotedStyle {
					styledValue = styles.String.Render(value)
				} else {
					styledValue = styles.Text.Render(value)
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
