package stripes

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/wordwrap"
)

// jsonContext tracks the current rendering context for intelligent wrapping
type jsonContext struct {
	indentLevel int    // Current nesting level
	indentStr   string // Indentation string per level
	width       int    // Maximum line width
}

func JSON(w io.Writer, r io.Reader, styles *Styles) {
	d := json.NewDecoder(r)
	d.UseNumber()

	ctx := &jsonContext{
		indentLevel: 0,
		indentStr:   styles.Indent,
		width:       styles.Width,
	}
	if ctx.indentStr == "" {
		ctx.indentStr = "  "
	}

	for d.More() {
		if err := printJSONValue(w, d, false, "", ctx, styles); err != nil {
			if err != io.EOF {
				fmt.Fprintf(w, "ERROR: %s\n", err)
			}
			break
		}
	}
}

func printJSONValue(w io.Writer, d *json.Decoder, isKey bool, keyName string, ctx *jsonContext, styles *Styles) error {
	t, err := d.Token()
	if err != nil {
		return err
	}
	switch v := t.(type) {
	case json.Delim:
		switch v {
		case '[':
			printJSONArray(w, d, ctx, styles)
		case '{':
			printJSONObject(w, d, ctx, styles)
		}
	case json.Number:
		io.WriteString(w, styles.Number.Render(string(v)))
	case string:
		var s string
		if isKey {
			s = styles.Name.Render(strconv.Quote(v))
		} else {
			// Apply intelligent wrapping for string values
			quotedValue := strconv.Quote(v)
			if !isKey && ctx.width > 0 {
				quotedValue = wrapJSONString(quotedValue, keyName, ctx)
			}
			s = styles.String.Render(quotedValue)
			s = strings.TrimRight(s, " ")
		}
		io.WriteString(w, s)
	case bool:
		io.WriteString(w, styles.Boolean.Render(strconv.FormatBool(v)))
	default:
		io.WriteString(w, styles.Null.Render("null"))
	}
	return nil
}

func printJSONArray(w io.Writer, d *json.Decoder, ctx *jsonContext, styles *Styles) {
	io.WriteString(w, styles.Syntax.Render("["))

	ctx.indentLevel++
	for first := true; d.More(); first = false {
		if !first {
			io.WriteString(w, styles.Syntax.Render(", "))
		}
		printJSONValue(w, d, false, "", ctx, styles)
	}
	ctx.indentLevel--

	// Consume the closing ']' delimiter
	d.Token()
	io.WriteString(w, styles.Syntax.Render("]"))
}

func printJSONObject(w io.Writer, d *json.Decoder, ctx *jsonContext, styles *Styles) {
	io.WriteString(w, styles.Syntax.Render("{"))
	writer := NewPrefixWriter(w, ctx.indentStr)
	length := 0

	ctx.indentLevel++
	for d.More() {
		if length != 0 {
			io.WriteString(writer, styles.Syntax.Render(","))
		}
		io.WriteString(writer, "\n")

		// Get the key first to pass it to the value renderer
		keyToken, err := d.Token()
		if err != nil {
			ctx.indentLevel--
			return
		}

		var keyName string
		if keyStr, ok := keyToken.(string); ok {
			keyName = keyStr
			// Render the key
			keyFormatted := styles.Name.Render(strconv.Quote(keyStr))
			io.WriteString(writer, keyFormatted)
		}

		io.WriteString(writer, styles.Syntax.Render(": "))
		printJSONValue(writer, d, false, keyName, ctx, styles)
		length++
	}
	ctx.indentLevel--

	// Consume the closing '}' delimiter
	d.Token()
	if length > 0 {
		io.WriteString(writer, "\n")
	}
	io.WriteString(w, styles.Syntax.Render("}"))
}

// wrapJSONString applies intelligent wrapping to JSON string values based on
// current indentation and key length
func wrapJSONString(quotedValue, keyName string, ctx *jsonContext) string {
	if ctx.width <= 0 || len(quotedValue) <= 2 {
		return quotedValue
	}

	// Calculate current position on the line
	// indentation + key + ": " = current position
	currentIndent := ctx.indentLevel * runewidth.StringWidth(ctx.indentStr)
	keyWidth := 0
	if keyName != "" {
		// Add quotes and key width
		keyWidth = runewidth.StringWidth(strconv.Quote(keyName)) + 2 // for ": "
	}

	currentPos := currentIndent + keyWidth

	// Calculate available width for the value
	availableWidth := ctx.width - currentPos

	// Only wrap if we have reasonable space and the value is longer than available width
	if availableWidth < 20 || runewidth.StringWidth(quotedValue) <= availableWidth {
		return quotedValue
	}

	// For JSON strings, we need to handle the quotes specially
	content := quotedValue

	// Apply word wrapping to the content
	wrappedContent := wordwrap.String(content, availableWidth-2) // Account for quotes

	// Handle multi-line by inserting newlines and proper indentation
	if strings.Contains(wrappedContent, "\n") {
		indent := strings.Repeat(" ", keyWidth)
		return strings.ReplaceAll(wrappedContent, "\n", "\n"+indent)
	}

	return wrappedContent
}
