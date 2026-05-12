package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"io"
	"iter"
	"sort"
	"time"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/table"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/format"
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

// parquetTable renders a parquet file as a typed-table view. The parquet
// schema drives column formatting: TIMESTAMP columns surface as time.Time
// (rendered with the table sub-package's slash-date layout), DATE as
// midnight time.Time, and primitives pass through as their natural Go
// types. The table sub-package then dispatches per-cell via
// anyCellFormatter.
//
// Parquet requires random access, so the entire input is buffered.
func parquetTable(w io.Writer, r io.Reader, styles *stripes.Styles) {
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
	converters := make([]parquetCellConverter, len(fields))
	for i, f := range fields {
		headers[i] = f.Name()
		converters[i] = converterFor(f)
	}

	var rows [][]any
	for {
		raw := make(map[string]any, len(headers))
		if err := reader.Read(&raw); err == io.EOF {
			break
		} else if err != nil {
			io.WriteString(w, "Error reading Parquet: "+err.Error())
			return
		}
		row := make([]any, len(headers))
		for i, name := range headers {
			row[i] = converters[i](raw[name])
		}
		rows = append(rows, row)
	}

	if len(rows) == 0 {
		io.WriteString(w, "Empty Parquet")
		return
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
		io.WriteString(w, "Error rendering Parquet table: "+err.Error())
	}
}

// parquetCellConverter turns a raw value decoded by parquet.Reader into the
// natural Go type for the column. For example, parquet TIMESTAMP_MILLIS
// arrives as int64 and is converted to time.Time so the table package's
// time formatter takes over.
type parquetCellConverter func(any) any

func converterFor(f parquet.Field) parquetCellConverter {
	// Group / repeated nodes: surface as-is (the table package's
	// isJSONFallbackType path renders maps and slices as colorized JSON).
	if !f.Leaf() {
		return identityConverter
	}
	lt := f.Type().LogicalType()
	if lt != nil {
		switch {
		case lt.Timestamp != nil:
			return timestampConverter(lt.Timestamp)
		case lt.Date != nil:
			return dateConverter
		case lt.UTF8 != nil, lt.Enum != nil, lt.Json != nil:
			return bytesToStringConverter
		case lt.UUID != nil, lt.Bson != nil:
			return identityConverter
		}
	}
	switch f.Type().Kind() {
	case parquet.ByteArray, parquet.FixedLenByteArray:
		// Raw binary without a logical type — leave as []byte so the
		// table package's JSON fallback renders it readably.
		return identityConverter
	}
	return identityConverter
}

func identityConverter(v any) any { return v }

func bytesToStringConverter(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

func timestampConverter(ts *format.TimestampType) parquetCellConverter {
	// parquet-go re-exports format.TimestampType; the value is whole units
	// of millis / micros / nanos since the Unix epoch as int64.
	switch {
	case ts.Unit.Millis != nil:
		return func(v any) any {
			if i, ok := v.(int64); ok {
				return time.UnixMilli(i).UTC()
			}
			return v
		}
	case ts.Unit.Micros != nil:
		return func(v any) any {
			if i, ok := v.(int64); ok {
				return time.UnixMicro(i).UTC()
			}
			return v
		}
	case ts.Unit.Nanos != nil:
		return func(v any) any {
			if i, ok := v.(int64); ok {
				return time.Unix(0, i).UTC()
			}
			return v
		}
	}
	return identityConverter
}

// dateConverter turns parquet's DATE (days since 1970-01-01 as int32) into
// a UTC midnight time.Time.
func dateConverter(v any) any {
	switch x := v.(type) {
	case int32:
		return time.Unix(int64(x)*86400, 0).UTC()
	case int64:
		return time.Unix(x*86400, 0).UTC()
	}
	return v
}
