package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	"github.com/parquet-go/parquet-go"
)

type pqRow struct {
	Name      string    `parquet:"name"`
	Age       int64     `parquet:"age"`
	Score     float64   `parquet:"score"`
	Active    bool      `parquet:"active"`
	CreatedAt time.Time `parquet:"created_at,timestamp"`
}

func samplePQ(tb testing.TB) []byte {
	tb.Helper()
	rows := []pqRow{
		{"alice", 30, 91.5, true, time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
		{"bob", 25, 76.2, false, time.Date(2026, 2, 3, 14, 30, 0, 0, time.UTC)},
		{"carol", 42, 88.0, true, time.Date(2026, 3, 9, 9, 15, 0, 0, time.UTC)},
	}
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[pqRow](&buf)
	if _, err := w.Write(rows); err != nil {
		tb.Fatalf("write parquet: %v", err)
	}
	if err := w.Close(); err != nil {
		tb.Fatalf("close parquet writer: %v", err)
	}
	return buf.Bytes()
}

func runParquetTable(t *testing.T, buf []byte) string {
	t.Helper()
	var out strings.Builder
	parquetTable(&out, bytes.NewReader(buf), stripes.DefaultStyles)
	return ansi.Strip(out.String())
}

func TestParquetTable(t *testing.T) {
	out := runParquetTable(t, samplePQ(t))
	mustContain(t, out, "name", "age", "score", "active", "created_at",
		"alice", "bob", "carol",
		"30", "25", "42",
		"true", "false",
	)
	// TIMESTAMP_MILLIS columns must be formatted with the table subpackage's
	// slash-date layout ("2026/01/15 10:00:00"), proving schema-driven
	// conversion ran.
	for _, want := range []string{
		"2026/01/15 10:00:00",
		"2026/02/03 14:30:00",
		"2026/03/09 09:15:00",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("parquetTable output missing timestamp %q\nfull output:\n%s", want, out)
		}
	}
}

func TestParquetTableEmpty(t *testing.T) {
	var out strings.Builder
	parquetTable(&out, strings.NewReader(""), stripes.DefaultStyles)
	if !strings.Contains(out.String(), "Empty Parquet") {
		t.Fatalf("expected Empty Parquet, got: %q", out.String())
	}
}

func TestParquetTableInvalid(t *testing.T) {
	var out strings.Builder
	parquetTable(&out, strings.NewReader("not a parquet file"), stripes.DefaultStyles)
	if !strings.Contains(out.String(), "Error reading Parquet") {
		t.Fatalf("expected Error reading Parquet, got: %q", out.String())
	}
}

func BenchmarkParquetTable(b *testing.B) {
	buf := samplePQ(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parquetTable(discardWriter{}, bytes.NewReader(buf), stripes.DefaultStyles)
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
