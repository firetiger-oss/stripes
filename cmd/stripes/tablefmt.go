package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"iter"
	"sort"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/table"
)

// csvTable renders CSV input as a styled table via the table sub-package.
// The first row is treated as the column headers.
func csvTable(w io.Writer, r io.Reader, styles *stripes.Styles) {
	renderDelimited(w, r, styles, ',', "CSV")
}

// tsvTable renders tab-separated input as a styled table.
func tsvTable(w io.Writer, r io.Reader, styles *stripes.Styles) {
	renderDelimited(w, r, styles, '\t', "TSV")
}

func renderDelimited(w io.Writer, r io.Reader, styles *stripes.Styles, comma rune, name string) {
	rd := csv.NewReader(r)
	rd.Comma = comma
	rd.LazyQuotes = true
	rd.FieldsPerRecord = -1
	rd.TrimLeadingSpace = true

	rows, err := rd.ReadAll()
	if err != nil {
		io.WriteString(w, "Error reading "+name+": "+err.Error())
		return
	}
	if len(rows) == 0 {
		io.WriteString(w, "Empty "+name)
		return
	}
	headers, data := rows[0], rows[1:]

	seq := iter.Seq2[[]string, error](func(yield func([]string, error) bool) {
		for _, row := range data {
			if !yield(row, nil) {
				return
			}
		}
	})
	if err := table.Write[[]string](w, seq,
		table.WithHeaders(headers...),
		table.WithStyles(styles),
	); err != nil {
		io.WriteString(w, "Error rendering "+name+" table: "+err.Error())
	}
}

// jsonlTable renders newline-delimited JSON objects as a styled table.
// The union of object keys (sorted) becomes the column headers; missing
// keys render as empty cells. Cell values use the table package's
// dynamic-type formatter, so nested arrays / objects get the compact-JSON
// colorize treatment automatically.
func jsonlTable(w io.Writer, r io.Reader, styles *stripes.Styles) {
	scanner := bufio.NewScanner(r)
	// Lines in JSONL can carry sizable payloads; allow up to 16 MiB per row.
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)

	var objects []map[string]any
	keys := make(map[string]struct{})
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			io.WriteString(w, "Error parsing JSONL: "+err.Error())
			return
		}
		for k := range obj {
			keys[k] = struct{}{}
		}
		objects = append(objects, obj)
	}
	if err := scanner.Err(); err != nil {
		io.WriteString(w, "Error reading JSONL: "+err.Error())
		return
	}
	if len(objects) == 0 {
		io.WriteString(w, "Empty JSONL")
		return
	}

	headers := make([]string, 0, len(keys))
	for k := range keys {
		headers = append(headers, k)
	}
	sort.Strings(headers)

	rows := make([][]any, len(objects))
	for i, obj := range objects {
		row := make([]any, len(headers))
		for j, k := range headers {
			row[j] = obj[k]
		}
		rows[i] = row
	}

	seq := iter.Seq2[[]any, error](func(yield func([]any, error) bool) {
		for _, row := range rows {
			if !yield(row, nil) {
				return
			}
		}
	})
	if err := table.Write[[]any](w, seq,
		table.WithHeaders(headers...),
		table.WithStyles(styles),
	); err != nil {
		io.WriteString(w, "Error rendering JSONL table: "+err.Error())
	}
}
