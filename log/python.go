package log

import (
	"regexp"

	"github.com/firetiger-oss/stripes"
)

// pythonFormat renders Python `logging` stdlib default format:
//
//	YYYY-MM-DD HH:MM:SS,sss LEVEL logger:lineno message
//	YYYY-MM-DD HH:MM:SS,sss [LEVEL] logger: message
//
// (Both variants appear in practice: the basicConfig default lacks
// the brackets, popular formatters add them. Both shapes are
// detected and parsed.)
//
// Reference: docs.python.org/3/library/logging.html "LogRecord
// attributes" and "Formatter".
var pythonFormat = LineFormat{
	Name:        "python-log",
	ContentType: "application/vnd.python-log",
	HasLevel:    true,
	Detect:      detectPython,
	Format:      formatPython,
}

var (
	// Bracketed: "2024-01-15 10:23:45,123 [INFO] root: message"
	pythonBracketRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3}) \[(TRACE|DEBUG|INFO|WARNING|WARN|ERROR|CRITICAL|FATAL|NOTICE)\] ([^\s:]+)(?::(\d+))?: (.*)$`)
	// Unbracketed: "2024-01-15 10:23:45,123 INFO root:42 message"
	pythonPlainRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2},\d{3}) (TRACE|DEBUG|INFO|WARNING|WARN|ERROR|CRITICAL|FATAL|NOTICE) ([^\s:]+)(?::(\d+))? (.*)$`)
)

func detectPython(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		if pythonBracketRe.MatchString(line) || pythonPlainRe.MatchString(line) {
			return true
		}
	}
	return false
}

func formatPython(line string, styles *stripes.Styles) (Row, bool) {
	m := pythonBracketRe.FindStringSubmatch(line)
	if m == nil {
		m = pythonPlainRe.FindStringSubmatch(line)
	}
	if m == nil {
		return Row{}, false
	}
	ts, level, logger, lineno, msg := m[1], m[2], m[3], m[4], m[5]

	metadata := logger
	if lineno != "" {
		metadata += ":" + lineno
	}
	return Row{
		Timestamp: ts,
		Level:     level,
		Metadata:  StyleMetadata(styles).Render(metadata),
		Message:   StyleText(styles).Render(msg),
	}, true
}
