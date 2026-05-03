package stripes

import (
	"io"
	"strings"

	"golang.org/x/net/html"
)

func HTML(w io.Writer, r io.Reader, styles *Styles) {
	doc, err := html.Parse(r)
	if err != nil {
		// Fallback to copying if parsing fails
		io.Copy(w, r)
		return
	}

	renderHTMLNode(w, doc, 0, styles)
}

func renderHTMLNode(w io.Writer, n *html.Node, depth int, styles *Styles) {
	switch n.Type {
	case html.DocumentNode:
		// Render all children of the document node with proper spacing
		var prevNodeRendered bool
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			// Skip whitespace-only text nodes
			if c.Type == html.TextNode && strings.TrimSpace(c.Data) == "" {
				continue
			}

			// Add newline between top-level nodes (except for the first one)
			if prevNodeRendered {
				io.WriteString(w, "\n")
			}

			renderHTMLNode(w, c, depth, styles)
			prevNodeRendered = true
		}

	case html.ElementNode:
		renderHTMLElement(w, n, depth, styles)

	case html.TextNode:
		text := strings.TrimSpace(n.Data)
		if text != "" {
			io.WriteString(w, styles.Text.Render(text))
		}
		// Don't render whitespace-only text nodes

	case html.CommentNode:
		io.WriteString(w, styles.Syntax.Render("<!--"))
		io.WriteString(w, styles.Comment.Render(n.Data))
		io.WriteString(w, styles.Syntax.Render("-->"))

	case html.DoctypeNode:
		io.WriteString(w, "<!DOCTYPE ")
		io.WriteString(w, n.Data)
		io.WriteString(w, ">")
	}
}

func renderHTMLElement(w io.Writer, n *html.Node, depth int, styles *Styles) {
	// Add indentation for nested elements
	if depth > 0 {
		io.WriteString(w, "\n")
		writeHTMLIndent(w, depth)
	}

	// Start building the opening tag
	io.WriteString(w, styles.Syntax.Render("<"))
	io.WriteString(w, styles.Anchor.Render(n.Data))

	// Add attributes
	for _, attr := range n.Attr {
		io.WriteString(w, " ")
		io.WriteString(w, styles.Name.Render(attr.Key))
		io.WriteString(w, styles.Syntax.Render("="))
		io.WriteString(w, styles.String.Render(`"`+attr.Val+`"`))
	}

	// Check if this is a void element
	if isVoidHTMLElement(n.Data) {
		io.WriteString(w, styles.Syntax.Render(">"))
		return
	}

	// Check for content
	hasChildren := n.FirstChild != nil
	hasTextContent := false
	hasElementChildren := false

	// Check what types of children we have
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode && strings.TrimSpace(c.Data) != "" {
			hasTextContent = true
		} else if c.Type == html.ElementNode || c.Type == html.CommentNode {
			hasElementChildren = true
		}
	}

	if !hasChildren {
		// Empty element
		io.WriteString(w, styles.Syntax.Render("></"))
		io.WriteString(w, styles.Anchor.Render(n.Data))
		io.WriteString(w, styles.Syntax.Render(">"))
		return
	}

	// Element with children
	io.WriteString(w, styles.Syntax.Render(">"))

	// Render children based on their types
	if hasElementChildren {
		// Special case: if we have both text and void elements (like <br>), render inline
		if hasTextContent && hasOnlyVoidElements(n) {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					text := strings.TrimSpace(c.Data)
					if text != "" {
						io.WriteString(w, styles.Text.Render(text))
					}
				} else if c.Type == html.ElementNode && isVoidHTMLElement(c.Data) {
					renderHTMLNode(w, c, 0, styles) // No depth for inline void elements
				} else if c.Type == html.ElementNode {
					renderHTMLNode(w, c, depth+1, styles)
				} else if c.Type == html.CommentNode {
					io.WriteString(w, "\n")
					writeHTMLIndent(w, depth+1)
					io.WriteString(w, styles.Syntax.Render("<!--"))
					io.WriteString(w, styles.Comment.Render(c.Data))
					io.WriteString(w, styles.Syntax.Render("-->"))
				}
			}
		} else {
			// Has element children - render with proper indentation
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.TextNode {
					text := strings.TrimSpace(c.Data)
					if text != "" {
						io.WriteString(w, styles.Text.Render(text))
					}
				} else if c.Type == html.ElementNode {
					renderHTMLNode(w, c, depth+1, styles)
				} else if c.Type == html.CommentNode {
					io.WriteString(w, "\n")
					writeHTMLIndent(w, depth+1)
					io.WriteString(w, styles.Syntax.Render("<!--"))
					io.WriteString(w, styles.Comment.Render(c.Data))
					io.WriteString(w, styles.Syntax.Render("-->"))
				}
			}
			if !hasTextContent {
				io.WriteString(w, "\n")
				writeHTMLIndent(w, depth)
			}
		}
	} else {
		// Only text content - render inline
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				text := strings.TrimSpace(c.Data)
				if text != "" {
					io.WriteString(w, styles.Text.Render(text))
				}
			}
		}
	}

	// Closing tag
	io.WriteString(w, styles.Syntax.Render("</"))
	io.WriteString(w, styles.Anchor.Render(n.Data))
	io.WriteString(w, styles.Syntax.Render(">"))
}

func writeHTMLIndent(w io.Writer, depth int) {
	for range depth {
		io.WriteString(w, "  ")
	}
}

// isVoidHTMLElement checks if an HTML element is a void element (self-closing)
func isVoidHTMLElement(tagName string) bool {
	voidElements := map[string]bool{
		"area": true, "base": true, "br": true, "col": true, "embed": true,
		"hr": true, "img": true, "input": true, "link": true, "meta": true,
		"param": true, "source": true, "track": true, "wbr": true,
	}
	return voidElements[strings.ToLower(tagName)]
}

// hasOnlyVoidElements checks if all element children are void elements
func hasOnlyVoidElements(n *html.Node) bool {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && !isVoidHTMLElement(c.Data) {
			return false
		}
	}
	return true
}
