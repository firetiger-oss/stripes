package stripes

import (
	"bytes"
	"strings"
	"testing"
)

func TestPlain(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple text",
			input: "Hello, World!",
		},
		{
			name:  "multiline text",
			input: "Line 1\nLine 2\nLine 3",
		},
		{
			name:  "empty input",
			input: "",
		},
		{
			name:  "text with special characters",
			input: "Special chars: @#$%^&*(){}[]|\\:;\"'<>?,./`~",
		},
		{
			name:  "unicode text",
			input: "Hello 世界 🌍",
		},
		{
			name:  "json-like text",
			input: `{"key": "value", "number": 123}`,
		},
		{
			name:  "xml-like text",
			input: `<root><item>value</item></root>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output bytes.Buffer
			reader := strings.NewReader(tt.input)

			Plain(&output, reader, DefaultStyles)

			result := output.String()
			if result != tt.input {
				t.Errorf("Plain() = %q, expected %q", result, tt.input)
			}
		})
	}
}

func TestPlainWithNilStyles(t *testing.T) {
	// Test that Plain works even when styles is nil (since it uses _ parameter)
	var output bytes.Buffer
	input := "Test with nil styles"
	reader := strings.NewReader(input)

	Plain(&output, reader, nil)

	result := output.String()
	if result != input {
		t.Errorf("Plain() with nil styles = %q, expected %q", result, input)
	}
}
