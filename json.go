package stripes

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// jsonContext tracks the current rendering context for indentation.
type jsonContext struct {
	indentLevel int    // Current nesting level
	indentStr   string // Indentation string per level
}

func JSON(w io.Writer, r io.Reader, styles *Styles) {
	d := json.NewDecoder(r)
	d.UseNumber()

	ctx := &jsonContext{
		indentStr: styles.Indent,
	}
	if ctx.indentStr == "" {
		ctx.indentStr = "  "
	}

	for d.More() {
		if err := printJSONValue(w, d, false, ctx, styles); err != nil {
			if err != io.EOF {
				fmt.Fprintf(w, "ERROR: %s\n", err)
			}
			break
		}
	}
}

func printJSONValue(w io.Writer, d *json.Decoder, isKey bool, ctx *jsonContext, styles *Styles) error {
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
		quoted := strconv.Quote(v)
		if isKey {
			io.WriteString(w, styles.Name.Render(quoted))
		} else {
			io.WriteString(w, styles.String.Render(quoted))
		}
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
	inner := strings.Repeat(ctx.indentStr, ctx.indentLevel)
	first := true
	for d.More() {
		if !first {
			io.WriteString(w, styles.Syntax.Render(","))
		}
		first = false
		io.WriteString(w, "\n")
		io.WriteString(w, inner)
		printJSONValue(w, d, false, ctx, styles)
	}
	ctx.indentLevel--

	d.Token() // consume ']'
	if !first {
		io.WriteString(w, "\n")
		io.WriteString(w, strings.Repeat(ctx.indentStr, ctx.indentLevel))
	}
	io.WriteString(w, styles.Syntax.Render("]"))
}

func printJSONObject(w io.Writer, d *json.Decoder, ctx *jsonContext, styles *Styles) {
	io.WriteString(w, styles.Syntax.Render("{"))

	ctx.indentLevel++
	inner := strings.Repeat(ctx.indentStr, ctx.indentLevel)
	first := true
	for d.More() {
		if !first {
			io.WriteString(w, styles.Syntax.Render(","))
		}
		first = false
		io.WriteString(w, "\n")
		io.WriteString(w, inner)

		keyToken, err := d.Token()
		if err != nil {
			ctx.indentLevel--
			return
		}
		if keyStr, ok := keyToken.(string); ok {
			io.WriteString(w, styles.Name.Render(strconv.Quote(keyStr)))
		}

		io.WriteString(w, styles.Syntax.Render(": "))
		printJSONValue(w, d, false, ctx, styles)
	}
	ctx.indentLevel--

	d.Token() // consume '}'
	if !first {
		io.WriteString(w, "\n")
		io.WriteString(w, strings.Repeat(ctx.indentStr, ctx.indentLevel))
	}
	io.WriteString(w, styles.Syntax.Render("}"))
}
