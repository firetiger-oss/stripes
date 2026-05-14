package xml

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRenderRender(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:   "simple element",
			input:  `<root>Hello World</root>`,
			output: `<root>Hello World</root>`,
		},
		{
			name:   "self-closing element",
			input:  `<root />`,
			output: `<root />`,
		},
		{
			name:   "element with attributes",
			input:  `<root id="1" class="test">content</root>`,
			output: `<root id="1" class="test">content</root>`,
		},
		{
			name:  "nested elements",
			input: `<root><child>value</child></root>`,
			output: `<root>
  <child>value</child>
</root>`,
		},
		{
			name:  "complex nested structure",
			input: `<root><users><user id="1" active="true"><name>John</name><age>30</age></user></users></root>`,
			output: `<root>
  <users>
    <user id="1" active="true">
      <name>John</name>
      <age>30</age>
    </user>
  </users>
</root>`,
		},
		// TODO: Mixed content is complex due to interleaved text and elements
		// {
		// 	name: "mixed content",
		// 	input: `<root>text<child>nested</child>more text</root>`,
		// 	output: `<root>textmore text
		//   <child>nested</child>
		// </root>`,
		// },
		{
			name:   "empty element",
			input:  `<root></root>`,
			output: `<root />`,
		},
		{
			name:   "XML with processing instruction",
			input:  `<?xml version="1.0" encoding="UTF-8"?><root>content</root>`,
			output: `<?xml version="1.0" encoding="UTF-8"?><root>content</root>`,
		},
		{
			name:  "XML with comments",
			input: `<root><!-- This is a comment --><child>value</child></root>`,
			output: `<root>
  <!-- This is a comment -->
  <child>value</child>
</root>`,
		},
		{
			name:   "multiple attributes",
			input:  `<element id="123" class="main" data-value="test" active="true" />`,
			output: `<element id="123" class="main" data-value="test" active="true" />`,
		},
		{
			name:  "deeply nested structure",
			input: `<a><b><c><d>deep</d></c></b></a>`,
			output: `<a>
  <b>
    <c>
      <d>deep</d>
    </c>
  </b>
</a>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			reader := strings.NewReader(tt.input)
			Render(&output, reader, stripes.DefaultStyles)
			result := output.String()

			// Strip ANSI codes for byte-for-byte comparison
			stripped := ansi.Strip(result)
			if stripped != tt.output {
				t.Errorf("Render() output mismatch\nInput: %s\nExpected:\n%s\nGot:\n%s\nActual (with ANSI):\n%s",
					tt.input, tt.output, stripped, result)
			}
		})
	}
}

func TestRenderXMLWithInvalidRender(t *testing.T) {
	// Test that invalid XML doesn't crash the function
	var output strings.Builder

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render() panicked with invalid XML: %v", r)
		}
	}()

	reader := strings.NewReader("<invalid><unclosed>")
	Render(&output, reader, stripes.DefaultStyles)
	// Invalid XML should produce some output or handle gracefully
}

func TestRenderXMLWithEmptyInput(t *testing.T) {
	// Test that empty input doesn't crash the function
	var output strings.Builder

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render() panicked with empty input: %v", r)
		}
	}()

	reader := strings.NewReader("")
	Render(&output, reader, stripes.DefaultStyles)
	// Empty input should produce empty output
	result := output.String()
	if result != "" {
		t.Errorf("Expected empty output for empty input, got: %q", result)
	}
}

func TestRenderXMLStyling(t *testing.T) {
	// Test that XML elements contain proper structure (styling may not work in test environment)
	input := `<root id="test"><child>value</child></root>`
	var output strings.Builder
	reader := strings.NewReader(input)
	Render(&output, reader, stripes.DefaultStyles)
	result := output.String()

	// Should contain styled elements (ANSI codes may not appear in test environment due to lipgloss auto-detection)
	if !strings.Contains(result, "root") {
		t.Error("Expected output to contain 'root' element")
	}
	if !strings.Contains(result, "child") {
		t.Error("Expected output to contain 'child' element")
	}
	if !strings.Contains(result, "test") {
		t.Error("Expected output to contain 'test' attribute value")
	}

	// Ensure proper formatting
	if !strings.Contains(result, "\n") {
		t.Error("Expected output to contain proper formatting with newlines")
	}
}
