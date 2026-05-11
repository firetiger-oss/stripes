package stripes

import "testing"

func TestIsNumber(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		// Integer tests (should use ParseInt)
		{"123", true},
		{"-123", true},
		{"+123", true},
		{"0", true},
		{"9223372036854775807", true}, // max int64

		// Unsigned integer tests (should use ParseUint if ParseInt fails)
		{"18446744073709551615", true}, // max uint64

		// Float tests (should use ParseFloat as fallback)
		{"123.45", true},
		{"-123.45", true},
		{"+123.45", true},
		{"0.0", true},
		{"123.45e10", true},
		{"1.23e-5", true},
		{"1.23E10", true},
		{"1.23E-5", true},

		// Invalid cases
		{"abc", false},
		{"12abc", false},
		{"", false},
		{"123.45.67", false},
		{"1e", false},
		{"++123", false},
		{"--123", false},
		{"12.34.56", false},
		{"not-a-number", false},
		{"123.456.789", false},
		{"1.2.3", false},
		{"inf", true},      // Go's ParseFloat accepts this
		{"NaN", true},      // Go's ParseFloat accepts this
		{"infinity", true}, // Go's ParseFloat accepts this
		{" 123", false},    // leading/trailing spaces not handled
		{"123 ", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := isNumber(tt.input)
			if result != tt.expected {
				t.Errorf("isNumber(%q) = %v, expected %v", tt.input, result, tt.expected)
			}
		})
	}
}

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
