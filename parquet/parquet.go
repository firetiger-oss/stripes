// Package parquet registers the Parquet renderer with the stripes
// registry. Import for side effects to enable
// application/vnd.apache.parquet support:
//
//	import _ "github.com/firetiger-oss/stripes/parquet"
package parquet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	"github.com/firetiger-oss/stripes"
	"github.com/parquet-go/parquet-go"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "parquet",
		ContentType: "application/vnd.apache.parquet",
		Extensions:  []string{".parquet"},
		MagicBytes:  [][]byte{[]byte("PAR1")},
		RendererFor: stripes.Simple(Render),
	})
}

// Render writes the parquet file read from r to w as a styled lipgloss
// table, mirroring the look of the csv renderer. Top-level schema fields
// become headers; each row is flattened to strings. Numeric columns are
// right-aligned.
//
// Parquet requires random access, so the entire input is buffered into
// memory before decoding.
func Render(w io.Writer, r io.Reader, styles *stripes.Styles) {
	buf, err := io.ReadAll(r)
	if err != nil {
		io.WriteString(w, "Error reading Parquet: "+err.Error())
		return
	}
	if len(buf) == 0 {
		io.WriteString(w, "Empty Parquet")
		return
	}

	pf, err := parquet.OpenFile(bytes.NewReader(buf), int64(len(buf)))
	if err != nil {
		io.WriteString(w, "Error reading Parquet: "+err.Error())
		return
	}
	reader := parquet.NewReader(pf)
	defer reader.Close()

	fields := reader.Schema().Fields()
	headers := make([]string, len(fields))
	for i, f := range fields {
		headers[i] = f.Name()
	}

	var rows [][]string
	for {
		row := make(map[string]any, len(headers))
		if err := reader.Read(&row); err == io.EOF {
			break
		} else if err != nil {
			io.WriteString(w, "Error reading Parquet: "+err.Error())
			return
		}
		cells := make([]string, len(headers))
		for i, name := range headers {
			cells[i] = stringifyParquetCell(row[name])
		}
		rows = append(rows, cells)
	}

	if len(rows) == 0 {
		io.WriteString(w, "Empty Parquet")
		return
	}

	isNumericCol := make([]bool, len(headers))
	for col := range isNumericCol {
		all := true
		for _, r := range rows {
			if col < len(r) && !stripes.IsNumeric(r[col]) {
				all = false
				break
			}
		}
		isNumericCol[col] = all
	}

	t := table.New().
		Border(styles.Border).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers(headers...)
	for _, r := range rows {
		t = t.Row(r...)
	}
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		if row == -1 {
			base = styles.Columns
		} else {
			base = styles.Rows
		}
		s := base.Padding(0, 1)
		if row >= 0 && col < len(isNumericCol) && isNumericCol[col] {
			s = s.Align(lipgloss.Right)
		} else {
			s = s.Align(lipgloss.Left)
		}
		return s
	})

	io.WriteString(w, t.Render())
}

// stringifyParquetCell formats a value decoded from a parquet row map for
// display in the default Parquet renderer. Bytes that look like UTF-8 text
// render as a string; other byte slices render as a hex literal.
func stringifyParquetCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		if utf8.Valid(x) {
			return string(x)
		}
		return "0x" + hexEncode(x)
	case bool:
		return strconv.FormatBool(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case int32:
		return strconv.FormatInt(int64(x), 10)
	case uint64:
		return strconv.FormatUint(x, 10)
	case uint32:
		return strconv.FormatUint(uint64(x), 10)
	case float64:
		return strconv.FormatFloat(x, 'g', -1, 64)
	case float32:
		return strconv.FormatFloat(float64(x), 'g', -1, 32)
	default:
		// Slices, maps, and any other parquet-decoded shape render as
		// compact JSON. Mirrors the typed-table package's jsonFormat
		// fallback, so nested-list cells appear as [1,2,3] rather than
		// Go's default "[1 2 3]" formatting.
		if b, err := json.Marshal(x); err == nil {
			return string(b)
		}
		return fmt.Sprintf("%v", x)
	}
}

func hexEncode(b []byte) string {
	const hex = "0123456789abcdef"
	out := make([]byte, 2*len(b))
	for i, c := range b {
		out[2*i] = hex[c>>4]
		out[2*i+1] = hex[c&0x0f]
	}
	return string(out)
}
