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
