package log

import (
	"regexp"

	"github.com/firetiger-oss/stripes"
)

// goLogFormat renders Go stdlib `log` package output. The package
// default uses Ldate|Ltime which produces:
//
//	YYYY/MM/DD HH:MM:SS message
//	YYYY/MM/DD HH:MM:SS.μμμμμμ message              (with Lmicroseconds)
//	YYYY/MM/DD HH:MM:SS file.go:42: message         (with Lshortfile/Llongfile)
//
// The marker is the unique slash-date format; Go's log package is
// the most common producer of YYYY/MM/DD lines in the wild.
//
// Reference: pkg.go.dev/log#pkg-constants.
var goLogFormat = LineFormat{
	Name:        "go-log",
	ContentType: "application/vnd.go-log",
	// Most lines lack a level (and an inline LEVEL: marker is a
	// convention some wrappers add). HasLevel=true preserves
	// alignment so the rare INFO/ERROR lines stand out against
	// blank slots without the ---- placeholder noise.
	HasLevel: true,
	Detect:   detectGoLog,
	Format:   formatGoLog,
}

var goLogRe = regexp.MustCompile(`^(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}(?:\.\d+)?) (?:([^\s:]+\.go:\d+): )?(.*)$`)

func detectGoLog(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		if goLogRe.MatchString(line) {
			return true
		}
	}
	return false
}

func formatGoLog(line string, styles *stripes.Styles) (Row, bool) {
	m := goLogRe.FindStringSubmatch(line)
	if m == nil {
		return Row{}, false
	}
	ts, source, msg := m[1], m[2], m[3]

	// Pull an inline "[LEVEL]" or "LEVEL:" marker if the program
	// uses one (very common in tools wrapping stdlib log).
	level, body := pickInlineLevel(msg)
	row := Row{
		Timestamp: ts,
		Level:     level,
		Message:   StyleText(styles).Render(body),
	}
	if source != "" {
		row.Metadata = StyleMetadata(styles).Render(source)
	}
	return row, true
}
