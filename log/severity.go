package log

import (
	"charm.land/lipgloss/v2"
	"github.com/firetiger-oss/stripes"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
)

// SeverityClass collapses OTLP's 24 severity numbers into the five
// classes used for display and colouring. UNKNOWN is the fallback
// when SeverityNumber is unset and SeverityText doesn't match any
// known token (rare; emitted by SDKs that don't populate severity).
type SeverityClass int

const (
	SevUnknown SeverityClass = iota
	SevTrace
	SevDebug
	SevInfo
	SevWarn
	SevError
	SevFatal
)

// Label returns the 4-char severity label drawn in the record row.
// All labels are exactly 4 columns so the column following the
// severity aligns regardless of class. The label intentionally drops
// the "2/3/4" suffix that OTLP attaches inside a class (DEBUG2 etc.)
// — that granularity belongs in verbose output, not in the
// at-a-glance table.
func (c SeverityClass) Label() string {
	switch c {
	case SevTrace:
		return "TRAC"
	case SevDebug:
		return "DEBU"
	case SevInfo:
		return "INFO"
	case SevWarn:
		return "WARN"
	case SevError:
		return "ERRO"
	case SevFatal:
		return "FATA"
	}
	return "----"
}

// Style returns the lipgloss style used to render the severity label
// in the table. The colour palette is fixed and intentionally
// independent of [stripes.DefaultStyles] so the severity column
// reads the same across every log format and against any background
// theme: cyan TRAC/DEBU, green INFO, yellow WARN, red ERRO, purple
// FATA. All bold.
//
// When styles is nil [stripes.DefaultStyles] is used (currently
// only for the unknown-class fallback).
func (c SeverityClass) Style(styles *stripes.Styles) lipgloss.Style {
	switch c {
	case SevTrace, SevDebug:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true) // bright cyan
	case SevInfo:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // bright green
	case SevWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // bright yellow
	case SevError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true) // bright red
	case SevFatal:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Bold(true) // bright magenta / purple
	}
	if styles == nil {
		styles = stripes.DefaultStyles
	}
	return styles.Comment
}

// ClassifyOTLP maps an OTLP SeverityNumber to a SeverityClass. The 24
// SeverityNumber values bucket into TRACE/DEBUG/INFO/WARN/ERROR/FATAL
// in blocks of 4 (TRACE = 1..4, DEBUG = 5..8, etc.). UNSPECIFIED (0)
// falls back to text (see [ClassifySeverity]).
func ClassifyOTLP(n logsv1.SeverityNumber, text string) SeverityClass {
	switch {
	case n >= logsv1.SeverityNumber_SEVERITY_NUMBER_FATAL:
		return SevFatal
	case n >= logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR:
		return SevError
	case n >= logsv1.SeverityNumber_SEVERITY_NUMBER_WARN:
		return SevWarn
	case n >= logsv1.SeverityNumber_SEVERITY_NUMBER_INFO:
		return SevInfo
	case n >= logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG:
		return SevDebug
	case n >= logsv1.SeverityNumber_SEVERITY_NUMBER_TRACE:
		return SevTrace
	}
	return ClassifySeverity(text)
}

// ClassifySeverity maps a free-form severity string (e.g. the
// SeverityText field on an OTLP LogRecord, or the level token from a
// text log line) to a SeverityClass. Matching is case-insensitive
// and covers the common spellings emitted by stdlib loggers,
// structured loggers (zap, slog, pino), and operating-system log
// daemons (syslog/journald). Returns [SevUnknown] for empty input or
// unrecognised tokens.
func ClassifySeverity(s string) SeverityClass {
	if s == "" {
		return SevUnknown
	}
	// Lowercase in place without allocating for the common case.
	var buf [16]byte
	n := len(s)
	if n > len(buf) {
		n = len(buf)
	}
	for i := 0; i < n; i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	switch string(buf[:n]) {
	case "trace", "trc", "verbose", "trace2", "trace3", "trace4":
		return SevTrace
	case "debug", "dbg", "debug2", "debug3", "debug4":
		return SevDebug
	case "info", "information", "informational", "notice", "info2", "info3", "info4":
		return SevInfo
	case "warn", "warning", "warn2", "warn3", "warn4":
		return SevWarn
	case "error", "err", "severe", "error2", "error3", "error4":
		return SevError
	case "fatal", "crit", "critical", "emerg", "emergency", "alert", "panic",
		"fatal2", "fatal3", "fatal4":
		return SevFatal
	}
	return SevUnknown
}
