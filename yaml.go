package stripes

import (
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

func YAML(w io.Writer, r io.Reader, styles *Styles) {
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

func printYAMLNode(w io.Writer, node *yaml.Node, depth int, styles *Styles) {
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
				writer := NewPrefixWriter(w, indentStr)
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
				writer := NewPrefixWriter(w, indentStr)
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
