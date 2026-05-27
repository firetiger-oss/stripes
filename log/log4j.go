package log

import (
	"regexp"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// log4jFormat renders log4j / Logback default pattern lines, which
// is also the shape used by Kafka's server.log:
//
//	[YYYY-MM-DD HH:MM:SS,SSS] LEVEL [thread] logger - message
//	YYYY-MM-DD HH:MM:SS,SSS LEVEL  thread logger - message
//
// Either bracketed or unbracketed; the thread and logger sections
// are optional in some PatternLayout configurations but the
// timestamp + level prefix is universal.
var log4jFormat = LineFormat{
	Name:        "log4j",
	ContentType: "application/vnd.log4j",
	HasLevel:    true,
	Detect:      detectLog4j,
	Format:      formatLog4j,
}

var (
	log4jBracketRe = regexp.MustCompile(`^\[(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}[.,]\d{3})\] (TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\b\s*(?:\[([^\]]+)\])?\s*(.*)$`)
	log4jPlainRe   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}[.,]\d{3}) (TRACE|DEBUG|INFO|WARN|ERROR|FATAL)\b\s+(?:\[([^\]]+)\]\s+)?(.*)$`)
)

func detectLog4j(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		// Bracketed timestamp is unambiguous — log4j-only convention.
		if log4jBracketRe.MatchString(line) {
			return true
		}
		// Plain-timestamp variant overlaps with python logging
		// (same prefix shape). Require a distinguishing marker: a
		// [thread] block immediately after the level, OR the
		// "logger - message" dash that Logback's PatternLayout
		// inserts by default. Without either, treat as ambiguous
		// so the python/logfmt detectors get a chance.
		m := log4jPlainRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		thread, rest := m[3], m[4]
		if thread != "" || strings.Contains(rest, " - ") {
			return true
		}
	}
	return false
}

func formatLog4j(line string, styles *stripes.Styles) (Row, bool) {
	m := log4jBracketRe.FindStringSubmatch(line)
	if m == nil {
		m = log4jPlainRe.FindStringSubmatch(line)
	}
	if m == nil {
		return Row{}, false
	}
	ts, level, thread, rest := m[1], m[2], m[3], m[4]

	logger, body := splitLog4jLogger(rest)
	metadata := logger
	if metadata == "" {
		metadata = thread
	} else if thread != "" {
		metadata += "[" + thread + "]"
	}

	return Row{
		Timestamp: ts,
		Level:     level,
		Metadata:  StyleMetadata(styles).Render(metadata),
		Message:   StyleText(styles).Render(body),
	}, true
}

// splitLog4jLogger separates the logger name from the message body
// in the trailing portion of a log4j line. The convention is
// "logger - message"; when the dash is missing the whole tail is
// returned as the body.
func splitLog4jLogger(rest string) (logger, body string) {
	if i := strings.Index(rest, " - "); i > 0 {
		return rest[:i], rest[i+3:]
	}
	return "", rest
}
