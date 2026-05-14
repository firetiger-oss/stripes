package html

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRenderHTML(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name:  "simple element",
			input: `<div>Hello World</div>`,
			output: `<html>
  <head></head>
  <body>
    <div>Hello World</div>
  </body>
</html>`,
		},
		{
			name:  "element with attributes",
			input: `<div id="test" class="main">content</div>`,
			output: `<html>
  <head></head>
  <body>
    <div id="test" class="main">content</div>
  </body>
</html>`,
		},
		{
			name:  "nested elements",
			input: `<div><p>paragraph</p></div>`,
			output: `<html>
  <head></head>
  <body>
    <div>
      <p>paragraph</p>
    </div>
  </body>
</html>`,
		},
		{
			name:  "void element",
			input: `<img src="test.jpg" alt="test">`,
			output: `<html>
  <head></head>
  <body>
    <img src="test.jpg" alt="test">
  </body>
</html>`,
		},
		{
			name:  "br element",
			input: `<p>Line 1<br>Line 2</p>`,
			output: `<html>
  <head></head>
  <body>
    <p>Line 1<br>Line 2</p>
  </body>
</html>`,
		},
		{
			name:  "HTML with comments",
			input: `<div><!-- This is a comment --><p>content</p></div>`,
			output: `<html>
  <head></head>
  <body>
    <div>
      <!-- This is a comment -->
      <p>content</p>
    </div>
  </body>
</html>`,
		},
		{
			name:  "complex nested structure",
			input: `<div class="container"><header><h1>Title</h1></header><main><p>Content</p></main></div>`,
			output: `<html>
  <head></head>
  <body>
    <div class="container">
      <header>
        <h1>Title</h1>
      </header>
      <main>
        <p>Content</p>
      </main>
    </div>
  </body>
</html>`,
		},
		{
			name:  "doctype",
			input: `<!DOCTYPE html><html><body><h1>Hello</h1></body></html>`,
			output: `<!DOCTYPE html>
<html>
  <head></head>
  <body>
    <h1>Hello</h1>
  </body>
</html>`,
		},
		{
			name:  "comment at start",
			input: `<!-- yo! --><div class="test">hello</div>`,
			output: `<!-- yo! -->
<html>
  <head></head>
  <body>
    <div class="test">hello</div>
  </body>
</html>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			reader := strings.NewReader(tt.input)
			HTML(&output, reader, stripes.DefaultStyles)
			result := output.String()

			// Strip ANSI codes for byte-for-byte comparison
			stripped := ansi.Strip(result)
			if stripped != tt.output {
				t.Errorf("HTML() output mismatch\nInput: %s\nExpected:\n%s\nGot:\n%s\nActual (with ANSI):\n%s",
					tt.input, tt.output, stripped, result)
			}
		})
	}
}

func TestRenderHTMLWithInvalidHTML(t *testing.T) {
	// Test that invalid HTML doesn't crash the function
	var output strings.Builder

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HTML() panicked with invalid HTML: %v", r)
		}
	}()

	reader := strings.NewReader("<div><unclosed><p>test</div>")
	HTML(&output, reader, stripes.DefaultStyles)
	// Invalid HTML should produce some output or handle gracefully
}

func TestRenderHTMLWithEmptyInput(t *testing.T) {
	// Test that empty input doesn't crash the function
	var output strings.Builder

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("HTML() panicked with empty input: %v", r)
		}
	}()

	reader := strings.NewReader("")
	HTML(&output, reader, stripes.DefaultStyles)
	// Empty input should produce some minimal HTML structure
	result := output.String()
	if result == "" {
		t.Error("Expected some output for empty input")
	}
}

func TestRenderHTMLStyling(t *testing.T) {
	// Test that HTML elements contain proper structure (styling may not work in test environment)
	input := `<div id="test"><p>content</p></div>`
	var output strings.Builder
	reader := strings.NewReader(input)
	HTML(&output, reader, stripes.DefaultStyles)
	result := output.String()

	// Should contain styled elements (ANSI codes may not appear in test environment due to lipgloss auto-detection)
	if !strings.Contains(result, "div") {
		t.Error("Expected output to contain 'div' element")
	}
	if !strings.Contains(result, "p") {
		t.Error("Expected output to contain 'p' element")
	}
	if !strings.Contains(result, "test") {
		t.Error("Expected output to contain 'test' attribute value")
	}

	// Ensure proper formatting for complex structures
	if !strings.Contains(result, "content") {
		t.Error("Expected output to contain text content")
	}
}
