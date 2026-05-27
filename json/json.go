// Package json registers the JSON renderer with the stripes registry.
// Import for side effects to enable application/json support:
//
//	import _ "github.com/firetiger-oss/stripes/json"
package json

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "json",
		ContentType: "application/json",
		Extensions:  []string{".json", ".tfstate"},
		MatchPath:   matchTFStateBackup,
		Detect:      detectJSON,
		RendererFor: stripes.Simple(Render),
	})
}

// matchTFStateBackup matches Terraform state backup files. The plain
// .tfstate extension is registered above; .tfstate.backup needs a path
// rule because filepath.Ext returns ".backup".
func matchTFStateBackup(path string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".tfstate.backup")
}

// jsonContext tracks the current rendering context for indentation.
type jsonContext struct {
	indentLevel int    // Current nesting level
	indentStr   string // Indentation string per level
}

// Render writes a styled rendering of the JSON read from r to w. Each
// top-level value is pretty-printed with the configured indent;
// non-string scalars never wrap.
func Render(w io.Writer, r io.Reader, styles *stripes.Styles) {
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

// detectJSON returns true when peek opens with what looks like a
// complete JSON value. The check rejects payloads that just happen
// to start with '{' or '[' but don't actually parse as JSON —
// log4j's bracketed timestamps like "[2026-01-15 10:23:45,123] …"
// are a common false positive a pure first-byte check would claim.
//
// The implementation skims the first value with [json.Decoder]: a
// well-formed top-level object/array completes without error, and
// truncated-but-otherwise-valid payloads (peek is only ~512 bytes)
// surface as io.ErrUnexpectedEOF — also accepted, since "we ran out
// of peek before the document ended" is consistent with valid JSON.
// Other parse errors mean the bytes aren't JSON.
func detectJSON(peek []byte) bool {
	trimmed := bytes.TrimLeft(peek, " \t\r\n")
	if len(trimmed) == 0 {
		return false
	}
	if trimmed[0] != '{' && trimmed[0] != '[' {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(trimmed))
	var raw json.RawMessage
	err := d.Decode(&raw)
	if err == nil {
		return true
	}
	// A clean truncation inside an otherwise valid document is OK —
	// we only see the first ~512 bytes.
	if errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
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
