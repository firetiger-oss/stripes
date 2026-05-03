package stripes

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestNewPrefixWriter(t *testing.T) {
	var buf bytes.Buffer
	prefix := "  "
	writer := NewPrefixWriter(&buf, prefix)

	// Verify it returns a writer
	if writer == nil {
		t.Fatal("NewPrefixWriter returned nil")
	}

	// Verify it implements io.Writer interface
	var _ io.Writer = writer

	// Verify it implements io.StringWriter interface
	if _, ok := writer.(io.StringWriter); !ok {
		t.Error("PrefixWriter should implement io.StringWriter")
	}
}

func TestPrefixWriterBasicWrite(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, ">> ")

	// Write simple string without newline
	n, err := writer.Write([]byte("hello"))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 5 {
		t.Errorf("Expected to write 5 bytes, got %d", n)
	}

	expected := ">> hello"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterWithNewlines(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "  ")

	// Write string with newlines
	text := "line1\nline2\nline3"
	n, err := writer.Write([]byte(text))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(text) {
		t.Errorf("Expected to write %d bytes, got %d", len(text), n)
	}

	expected := "  line1\n  line2\n  line3"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterMultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "| ")

	// First write
	writer.Write([]byte("first"))

	// Second write continuing same line
	writer.Write([]byte(" second"))

	// Third write with newline
	writer.Write([]byte("\nthird"))

	expected := "| first second\n| third"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterWriteString(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "## ")

	// Test WriteString method
	if sw, ok := writer.(io.StringWriter); ok {
		n, err := sw.WriteString("hello world")
		if err != nil {
			t.Fatalf("WriteString failed: %v", err)
		}
		if n != 11 {
			t.Errorf("Expected to write 11 bytes, got %d", n)
		}

		expected := "## hello world"
		if buf.String() != expected {
			t.Errorf("Expected %q, got %q", expected, buf.String())
		}
	} else {
		t.Fatal("Writer doesn't implement io.StringWriter")
	}
}

func TestPrefixWriterWriteStringWithNewlines(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, ">>> ")

	if sw, ok := writer.(io.StringWriter); ok {
		text := "first line\nsecond line\nthird line"
		n, err := sw.WriteString(text)
		if err != nil {
			t.Fatalf("WriteString failed: %v", err)
		}
		if n != len(text) {
			t.Errorf("Expected to write %d bytes, got %d", len(text), n)
		}

		expected := ">>> first line\n>>> second line\n>>> third line"
		if buf.String() != expected {
			t.Errorf("Expected %q, got %q", expected, buf.String())
		}
	} else {
		t.Fatal("Writer doesn't implement io.StringWriter")
	}
}

func TestPrefixWriterEmptyPrefix(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "")

	// Write with empty prefix should work without adding anything
	writer.Write([]byte("hello\nworld"))

	expected := "hello\nworld"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterOnlyNewlines(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "* ")

	// Write only newlines
	writer.Write([]byte("\n\n\n"))

	expected := "* \n* \n* \n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterTrailingNewline(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "- ")

	// Write text ending with newline
	writer.Write([]byte("line1\nline2\n"))

	// Write more text (should get prefix since previous ended with newline)
	writer.Write([]byte("line3"))

	expected := "- line1\n- line2\n- line3"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterSingleNewline(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "> ")

	// Write just a newline
	writer.Write([]byte("\n"))

	expected := "> \n"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterMultiBytePrefix(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "🔥 ")

	// Write with multi-byte prefix
	writer.Write([]byte("hello\nworld"))

	expected := "🔥 hello\n🔥 world"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterInterleavedWriteAndWriteString(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "  ")

	// Mix Write and WriteString calls
	writer.Write([]byte("first"))

	if sw, ok := writer.(io.StringWriter); ok {
		sw.WriteString(" second")
	}

	writer.Write([]byte("\nthird"))

	if sw, ok := writer.(io.StringWriter); ok {
		sw.WriteString(" fourth")
	}

	expected := "  first second\n  third fourth"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}

func TestPrefixWriterLargeText(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "| ")

	// Create large text with many lines
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = strings.Repeat("x", 50)
	}
	largeText := strings.Join(lines, "\n")

	n, err := writer.Write([]byte(largeText))
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != len(largeText) {
		t.Errorf("Expected to write %d bytes, got %d", len(largeText), n)
	}

	// Verify all lines got prefixed
	result := buf.String()
	resultLines := strings.Split(result, "\n")
	if len(resultLines) != 100 {
		t.Errorf("Expected 100 lines, got %d", len(resultLines))
	}

	for i, line := range resultLines {
		if !strings.HasPrefix(line, "| ") {
			t.Errorf("Line %d doesn't have prefix: %q", i, line)
		}
	}
}

func TestPrefixWriterZeroLengthWrite(t *testing.T) {
	var buf bytes.Buffer
	writer := NewPrefixWriter(&buf, "  ")

	// Write zero-length data
	n, err := writer.Write([]byte{})
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if n != 0 {
		t.Errorf("Expected to write 0 bytes, got %d", n)
	}

	// Buffer should still be empty
	if buf.Len() != 0 {
		t.Errorf("Expected empty buffer, got %q", buf.String())
	}

	// Follow up with actual content
	writer.Write([]byte("hello"))
	expected := "  hello"
	if buf.String() != expected {
		t.Errorf("Expected %q, got %q", expected, buf.String())
	}
}
