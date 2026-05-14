package stripes

import "testing"

func TestIsNumeric(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"123", true},
		{"123.45", true},
		{"-123", true},
		{"+123", true},
		{"123.45e10", true},
		{"1.23e-5", true},
		{"-0.0", true},
		{"0", true},
		{"", true},
		{"abc", false},
		{"12abc", false},
		{"$123", false},
		{"123%", true},
		{"1.4 KiB", true},
		{"N/A", false},
		{"null", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := IsNumeric(tt.input)
			if result != tt.expected {
				t.Errorf("IsNumeric(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}
