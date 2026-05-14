// Package xml registers the XML renderer with the stripes registry.
// Import for side effects to enable application/xml support:
//
//	import _ "github.com/firetiger-oss/stripes/xml"
package xml

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"

	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "xml",
		ContentType: "application/xml",
		Extensions:  []string{".xml"},
		Detect:      detectXML,
		RendererFor: stripes.Simple(XML),
	})
}

// detectXML returns true when peek looks like XML — a leading "<?xml"
// declaration or a generic "<" tag — but excludes the HTML doctypes so
// the html sub-package's detector wins on shared "<" prefixes.
func detectXML(peek []byte) bool {
	trimmed := bytes.TrimLeft(peek, " \t\r\n")
	if len(trimmed) == 0 || trimmed[0] != '<' {
		return false
	}
	lower := bytes.ToLower(trimmed)
	if bytes.HasPrefix(lower, []byte("<?xml")) {
		return true
	}
	if bytes.HasPrefix(lower, []byte("<!doctype html")) ||
		bytes.HasPrefix(lower, []byte("<html")) {
		return false
	}
	return true
}

// XML renders an XML document with ANSI styling.
func XML(w io.Writer, r io.Reader, styles *stripes.Styles) {
	d := xml.NewDecoder(r)

	for {
		t, err := d.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return
		}
		switch token := t.(type) {
		case xml.StartElement:
			printXMLElement(w, d, token, styles)
		case xml.ProcInst:
			io.WriteString(w, styles.Syntax.Render("<?"))
			io.WriteString(w, styles.Anchor.Render(token.Target))
			if len(token.Inst) > 0 {
				io.WriteString(w, " ")
				io.WriteString(w, styles.Text.Render(string(token.Inst)))
			}
			io.WriteString(w, styles.Syntax.Render("?>"))
		case xml.Comment:
			io.WriteString(w, styles.Syntax.Render("<!--"))
			io.WriteString(w, styles.Comment.Render(string(token)))
			io.WriteString(w, styles.Syntax.Render("-->"))
		}
	}
}

func printXMLElement(w io.Writer, d *xml.Decoder, start xml.StartElement, styles *stripes.Styles) {
	// Start building the opening tag
	io.WriteString(w, styles.Syntax.Render("<"))
	io.WriteString(w, styles.Anchor.Render(start.Name.Local))

	// Add attributes
	for _, attr := range start.Attr {
		io.WriteString(w, " ")
		io.WriteString(w, styles.Name.Render(attr.Name.Local))
		io.WriteString(w, styles.Syntax.Render("="))
		io.WriteString(w, styles.String.Render(`"`+attr.Value+`"`))
	}

	// Collect all content first
	var allTextContent strings.Builder
	var childrenCount int

	// Process content until we hit the end element
	for {
		t, err := d.Token()
		if err != nil {
			break
		}

		if endElement, ok := t.(xml.EndElement); ok && endElement.Name == start.Name {
			break
		}

		switch token := t.(type) {
		case xml.CharData:
			allTextContent.Write(token)

		case xml.StartElement:
			if childrenCount == 0 {
				// First child - close opening tag and write all text content first
				io.WriteString(w, styles.Syntax.Render(">"))
				if text := strings.TrimSpace(allTextContent.String()); text != "" {
					io.WriteString(w, styles.Text.Render(text))
				}
			}

			// Create fresh indented writer for each child
			childWriter := stripes.NewPrefixWriter(w, "  ")
			io.WriteString(w, "\n")
			printXMLElement(childWriter, d, token, styles)
			childrenCount++

		case xml.Comment:
			if childrenCount == 0 {
				// First child - close opening tag and write all text content first
				io.WriteString(w, styles.Syntax.Render(">"))
				if text := strings.TrimSpace(allTextContent.String()); text != "" {
					io.WriteString(w, styles.Text.Render(text))
				}
			}

			// Create fresh indented writer for each comment
			childWriter := stripes.NewPrefixWriter(w, "  ")
			io.WriteString(w, "\n")
			io.WriteString(childWriter, styles.Syntax.Render("<!--"))
			io.WriteString(childWriter, styles.Comment.Render(string(token)))
			io.WriteString(childWriter, styles.Syntax.Render("-->"))
			childrenCount++
		}
	}

	text := strings.TrimSpace(allTextContent.String())
	hasText := text != ""
	hasChildren := childrenCount > 0

	// Handle different cases
	if !hasText && !hasChildren {
		// Empty element
		io.WriteString(w, styles.Syntax.Render(" />"))
	} else if hasText && !hasChildren {
		// Text-only element
		io.WriteString(w, styles.Syntax.Render(">"))
		io.WriteString(w, styles.Text.Render(text))
		io.WriteString(w, styles.Syntax.Render("</"))
		io.WriteString(w, styles.Anchor.Render(start.Name.Local))
		io.WriteString(w, styles.Syntax.Render(">"))
	} else if hasChildren {
		// Element with children - write final newline and closing tag
		if childrenCount > 0 {
			io.WriteString(w, "\n")
		}
		io.WriteString(w, styles.Syntax.Render("</"))
		io.WriteString(w, styles.Anchor.Render(start.Name.Local))
		io.WriteString(w, styles.Syntax.Render(">"))
	}
}
