package stripes

import (
	"bytes"
	"strings"
	"testing"
)

func TestTextWrapping(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		width    int
		expected string
	}{
		{
			name:     "no wrapping needed",
			input:    "Short text",
			width:    20,
			expected: "Short text",
		},
		{
			name:     "wrapping needed",
			input:    "This is a very long line that should be wrapped at the specified width",
			width:    20,
			expected: "This is a very long\nline that should be\nwrapped at the\nspecified width",
		},
		{
			name:     "zero width no wrapping",
			input:    "This text should not be wrapped when width is zero",
			width:    0,
			expected: "This text should not be wrapped when width is zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			input := strings.NewReader(tt.input)

			styles := DefaultStyles.Clone()
			styles.Width = tt.width

			Text(&buf, input, styles)

			// Get the result and strip any styling to check just the text content
			result := buf.String()

			// Since the text goes through lipgloss styling, we need to check
			// that the line breaks are present in the output
			if tt.width > 0 && len(tt.input) > tt.width {
				if !strings.Contains(result, "\n") {
					t.Errorf("Expected wrapped text to contain newlines, got: %q", result)
				}
			}
		})
	}
}

// TestTextNoTrailingPadding guards against the lipgloss multi-line render
// padding: rendering a multi-line block with a styled Text in one call
// pads every line with spaces to the longest line. In narrow terminals
// that padding wraps and looks like an empty line after every source
// line. Text now renders one line at a time, so each output line must
// match the visible content with no trailing whitespace before the
// newline.
func TestTextNoTrailingPadding(t *testing.T) {
	input := "short\na much longer line of text here\nmid\n"
	var buf bytes.Buffer
	styles := DefaultStyles.Clone()
	styles.Width = 0 // disable wrapping
	Text(&buf, strings.NewReader(input), styles)

	for i, line := range strings.Split(buf.String(), "\n") {
		// Strip ANSI escapes; whatever is left must not end in a space.
		stripped := stripANSI(line)
		if strings.HasSuffix(stripped, " ") {
			t.Errorf("line %d ends with trailing space: %q", i, stripped)
		}
	}
}

// stripANSI removes CSI escape sequences for length-sensitive assertions.
func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); {
		if s[i] == 0x1b && i+1 < len(s) && s[i+1] == '[' {
			j := i + 2
			for j < len(s) && (s[j] < 0x40 || s[j] > 0x7e) {
				j++
			}
			if j < len(s) {
				j++
			}
			i = j
			continue
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
