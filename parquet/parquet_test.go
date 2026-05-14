package parquet

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	"github.com/parquet-go/parquet-go"
)

type parquetTestRow struct {
	Name      string    `parquet:"name"`
	Age       int64     `parquet:"age"`
	Score     float64   `parquet:"score"`
	Active    bool      `parquet:"active"`
	CreatedAt time.Time `parquet:"created_at,timestamp"`
}

// sampleParquet returns a small parquet file in memory. The set covers
// string, int64, float64, bool, and TIMESTAMP_MILLIS — enough to exercise
// both renderers' type handling.
func sampleParquet(tb testing.TB) []byte {
	tb.Helper()
	rows := []parquetTestRow{
		{"alice", 30, 91.5, true, time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)},
		{"bob", 25, 76.2, false, time.Date(2026, 2, 3, 14, 30, 0, 0, time.UTC)},
		{"carol", 42, 88.0, true, time.Date(2026, 3, 9, 9, 15, 0, 0, time.UTC)},
	}
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[parquetTestRow](&buf)
	if _, err := w.Write(rows); err != nil {
		tb.Fatalf("write parquet: %v", err)
	}
	if err := w.Close(); err != nil {
		tb.Fatalf("close parquet writer: %v", err)
	}
	return buf.Bytes()
}

func runParquet(t *testing.T, buf []byte) string {
	t.Helper()
	var out strings.Builder
	Parquet(&out, bytes.NewReader(buf), stripes.DefaultStyles)
	return ansi.Strip(out.String())
}

func TestParquet(t *testing.T) {
	out := runParquet(t, sampleParquet(t))
	for _, want := range []string{
		"name", "age", "score", "active", "created_at",
		"alice", "bob", "carol",
		"30", "25", "42",
		"91.5", "76.2", "88",
		"true", "false",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Parquet output missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestParquetEmpty(t *testing.T) {
	var out strings.Builder
	Parquet(&out, strings.NewReader(""), stripes.DefaultStyles)
	if !strings.Contains(out.String(), "Empty Parquet") {
		t.Fatalf("expected Empty Parquet message, got: %q", out.String())
	}
}

// TestParquetNestedJSON verifies that list/map cells render as compact
// JSON, not Go's default `[1 2 3]` slice formatting.
func TestParquetNestedJSON(t *testing.T) {
	type ListRow struct {
		Name   string   `parquet:"name"`
		Tags   []int64  `parquet:"tags"`
		Labels []string `parquet:"labels"`
	}
	rows := []ListRow{
		{"alice", []int64{1, 2, 3}, []string{"admin", "ops"}},
	}
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[ListRow](&buf)
	if _, err := w.Write(rows); err != nil {
		t.Fatalf("write parquet: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close parquet writer: %v", err)
	}
	out := runParquet(t, buf.Bytes())
	for _, want := range []string{`[1,2,3]`, `["admin","ops"]`} {
		if !strings.Contains(out, want) {
			t.Errorf("Parquet output missing %q\nfull output:\n%s", want, out)
		}
	}
	// Make sure Go's default slice format didn't sneak through.
	if strings.Contains(out, "[1 2 3]") {
		t.Errorf("Parquet output contains Go-formatted slice [1 2 3], want JSON\nfull output:\n%s", out)
	}
}

func TestParquetInvalid(t *testing.T) {
	var out strings.Builder
	Parquet(&out, strings.NewReader("not a parquet file"), stripes.DefaultStyles)
	if !strings.Contains(out.String(), "Error reading Parquet") {
		t.Fatalf("expected Error reading Parquet, got: %q", out.String())
	}
}

func BenchmarkParquet(b *testing.B) {
	buf := sampleParquet(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var sink discardWriter
		Parquet(sink, bytes.NewReader(buf), stripes.DefaultStyles)
	}
}

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
