// Package json registers the JSON renderer with the stripes registry.
// Import for side effects to enable application/json support:
//
//	import _ "github.com/firetiger-oss/stripes/json"
package json

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "json",
		ContentType: "application/json",
		Extensions:  []string{".json"},
		Detect:      detectJSON,
		RendererFor: stripes.Simple(JSON),
	})
}

// jsonContext tracks the current rendering context for indentation.
type jsonContext struct {
	indentLevel int    // Current nesting level
	indentStr   string // Indentation string per level
}

// JSON renders a JSON stream with ANSI styling. Each top-level value is
// pretty-printed with the configured indent; non-string scalars never
// wrap.
func JSON(w io.Writer, r io.Reader, styles *stripes.Styles) {
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

// detectJSON returns true when peek starts with '{' or '[' (after
// leading whitespace).
func detectJSON(peek []byte) bool {
	trimmed := bytes.TrimLeft(peek, " \t\r\n")
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '{' || trimmed[0] == '['
}

func printJSONValue(w io.Writer, d *json.Decoder, isKey bool, ctx *jsonContext, styles *stripes.Styles) error {
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

func printJSONArray(w io.Writer, d *json.Decoder, ctx *jsonContext, styles *stripes.Styles) {
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

func printJSONObject(w io.Writer, d *json.Decoder, ctx *jsonContext, styles *stripes.Styles) {
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
