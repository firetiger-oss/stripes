package stripes

import (
	"strconv"
	"strings"
)

// isNumber checks if a string represents a numeric value
// It tries to parse with strconv.ParseInt, then strconv.ParseUint, then strconv.ParseFloat as fallback
func isNumber(s string) bool {
	if _, err := strconv.ParseInt(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseUint(s, 10, 64); err == nil {
		return true
	}
	if _, err := strconv.ParseFloat(s, 64); err == nil {
		return true
	}
	return false
}

// IsNumeric reports whether s looks like a number — optionally signed,
// with `.` or `,` as decimals, scientific notation, and a recognised unit
// suffix (`%`, time units, byte sizes, SI prefixes, `kg`). Empty strings
// are treated as numeric (a placeholder for missing values), so callers
// that care about presence must check separately.
func IsNumeric(s string) bool {
	if s == "" {
		return true
	}

	n := len(s)
checkLoop:
	for i := range n {
		c := s[i]
		switch {
		case '0' <= c && c <= '9':
		case c == '.' || c == ',':
		case c == '-' || c == '+':
		case c == 'e' || c == 'E':
		default:
			n = i
			break checkLoop
		}
	}

	s = strings.TrimSpace(s[n:])
	switch s {
	case "":
	case "%":
	case "h", "m", "s", "ms", "us", "ns", "µs":
	case "B", "KB", "MB", "GB", "TB", "PB", "EB", "ZB", "YB":
	case "KiB", "MiB", "GiB", "TiB", "PiB", "EiB", "ZiB", "YiB":
	case "k", "g", "t", "p", "e", "z", "y":
	case "K", "M", "G", "T", "P", "E", "Z", "Y":
	case "kg":
	default:
		return false
	}
	return true
}
