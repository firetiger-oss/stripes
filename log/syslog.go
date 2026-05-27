package log

import (
	"regexp"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// syslogRFC5424Format renders RFC 5424 syslog. Format:
//
//	<PRI>1 TIMESTAMP HOSTNAME APP-NAME PROCID MSGID [STRUCTURED-DATA] MSG
//
// PRI encodes facility×8 + severity. TIMESTAMP is ISO 8601. The
// trailing structured-data is rendered dim; the free-form message
// (when present) gets the normal text style.
//
// Reference: RFC 5424 §6.
var syslogRFC5424Format = LineFormat{
	Name:        "syslog-rfc5424",
	ContentType: "application/vnd.syslog-rfc5424",
	HasLevel:    true, // PRI byte carries severity for every record.
	Detect:      detectSyslogRFC5424,
	Format:      formatSyslogRFC5424,
}

var rfc5424Re = regexp.MustCompile(`^<(\d{1,3})>1 (\S+) (\S+) (\S+) (\S+) (\S+) (.*)$`)

func detectSyslogRFC5424(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		if !strings.HasPrefix(line, "<") {
			return false
		}
		if rfc5424Re.MatchString(line) {
			return true
		}
	}
	return false
}

func formatSyslogRFC5424(line string, styles *stripes.Styles) (Row, bool) {
	m := rfc5424Re.FindStringSubmatch(line)
	if m == nil {
		return Row{}, false
	}
	pri, ts, host, app, procID, msgID, rest := m[1], m[2], m[3], m[4], m[5], m[6], m[7]
	sd, msg := splitStructuredData(rest)

	metadata := app
	if procID != "-" {
		metadata += "[" + procID + "]"
	}

	row := Row{
		Timestamp: ts,
		Level:     levelFromPRI(pri),
		Metadata:  StyleMetadata(styles).Render(metadata),
		Message:   StyleText(styles).Render(msg),
		Attrs:     []KV{{Key: "host", Value: host}},
	}
	if msgID != "-" {
		row.Attrs = append(row.Attrs, KV{Key: "msg-id", Value: msgID})
	}
	if sd != "" {
		row.Attrs = append(row.Attrs, KV{Key: "sd", Value: sd})
	}
	return row, true
}

// splitStructuredData separates an RFC 5424 STRUCTURED-DATA prefix
// from the MSG that follows it. "-" indicates absent
// structured-data; a "[...]" block (possibly stacked) precedes the
// message otherwise.
func splitStructuredData(rest string) (sd, msg string) {
	if rest == "" {
		return "", ""
	}
	if rest == "-" {
		return "", ""
	}
	if strings.HasPrefix(rest, "- ") {
		return "", rest[2:]
	}
	if rest[0] != '[' {
		return "", rest
	}
	// Walk balanced [...] groups. Quoted brackets inside SD-PARAM
	// values are rare in practice; the lightweight scan here covers
	// the common case (rsyslog, syslog-ng) and falls through to
	// "treat the whole thing as message" on malformed input.
	depth := 0
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				if i+1 < len(rest) && rest[i+1] == ' ' {
					return rest[:i+1], rest[i+2:]
				}
				return rest[:i+1], ""
			}
		}
	}
	return "", rest
}

// syslogBSDFormat renders BSD syslog (RFC 3164) and the equivalent
// journald short-form / macOS system.log shape:
//
//	Mon DD HH:MM:SS hostname tag[pid]: message
//
// PID and the surrounding brackets are optional. PRI is rarely
// printed in this format (it's encoded for the transport rather than
// the file), so severity is left as Unknown unless the message
// itself opens with a bracketed level marker like "[INFO]".
var syslogBSDFormat = LineFormat{
	Name:        "syslog",
	ContentType: "application/vnd.syslog",
	// BSD-format / journald-short / macOS system.log lines carry
	// no PRI byte and rarely include inline level markers — the
	// level column would be a long run of "----" placeholders, so
	// drop it entirely. ClassifyLevel is still consulted on the
	// rare inline-marker case via [pickInlineLevel], but the
	// extracted level surfaces as a "level=" attribute instead of
	// hijacking the (absent) column.
	HasLevel: false,
	Detect:   detectSyslogBSD,
	Format:   formatSyslogBSD,
}

var (
	// The trailing `(?:\s+\([^)]*\))?` clause is for macOS, whose
	// system.log entries look like "loginwindow[150] (com.apple.spotlight):
	// …" — a parenthesised subsystem suffix after the tag/pid pair.
	// Ignored after capture (rolls into the discarded prefix); the
	// real subsystem identity lives in unified-logging exports.
	bsdSyslogRe = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d{1,2} \d{2}:\d{2}:\d{2}) (\S+) ([^\s:\[]+)(?:\[(\d+)\])?(?:\s+\([^)]*\))?: (.*)$`)
	// macOS / new-syslogd variant uses an ISO 8601 timestamp prefix.
	bsdSyslogIsoRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+\-]\d{2}:?\d{2})?) (\S+) ([^\s:\[]+)(?:\[(\d+)\])?(?:\s+\([^)]*\))?: (.*)$`)
)

func detectSyslogBSD(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		if bsdSyslogRe.MatchString(line) || bsdSyslogIsoRe.MatchString(line) {
			return true
		}
	}
	return false
}

func formatSyslogBSD(line string, styles *stripes.Styles) (Row, bool) {
	m := bsdSyslogRe.FindStringSubmatch(line)
	if m == nil {
		m = bsdSyslogIsoRe.FindStringSubmatch(line)
	}
	if m == nil {
		return Row{}, false
	}
	ts, host, tag, pid, msg := m[1], m[2], m[3], m[4], m[5]

	level, body := pickInlineLevel(msg)
	metadata := tag
	if pid != "" {
		metadata += "[" + pid + "]"
	}
	row := Row{
		Timestamp: ts,
		Metadata:  StyleMetadata(styles).Render(metadata),
		Message:   StyleText(styles).Render(body),
		Attrs:     []KV{{Key: "host", Value: host}},
	}
	// HasLevel=false on this format hides the level column, so any
	// inline-extracted level becomes an attribute instead.
	if level != "" {
		row.Attrs = append(row.Attrs, KV{Key: "level", Value: level})
	}
	return row, true
}

// pickInlineLevel pulls a leading "[LEVEL]" or "LEVEL:" token off
// the message body and returns (level, remaining body). Returns
// ("", msg) when no level marker is present so callers can keep the
// full body intact.
func pickInlineLevel(msg string) (level, body string) {
	if strings.HasPrefix(msg, "[") {
		if i := strings.IndexByte(msg, ']'); i > 1 {
			candidate := msg[1:i]
			if ClassifyLevel(candidate) != 0 {
				rest := strings.TrimLeft(msg[i+1:], " ")
				return candidate, rest
			}
		}
	}
	if i := strings.IndexByte(msg, ':'); i > 0 && i < 12 {
		candidate := msg[:i]
		if ClassifyLevel(candidate) != 0 {
			rest := strings.TrimLeft(msg[i+1:], " ")
			return candidate, rest
		}
	}
	return "", msg
}

// levelFromPRI extracts the severity component (0..7) from a
// syslog PRI value (facility×8 + severity) and returns a level
// token the shared classifier understands. Returns "" on
// unparseable input so it falls through to SevUnknown.
func levelFromPRI(pri string) string {
	n := 0
	for i := 0; i < len(pri); i++ {
		c := pri[i]
		if c < '0' || c > '9' {
			return ""
		}
		n = n*10 + int(c-'0')
		if n > 191 { // 23 facilities × 8 + 7
			return ""
		}
	}
	switch n & 7 {
	case 0, 1, 2: // emerg, alert, crit
		return "crit"
	case 3:
		return "error"
	case 4:
		return "warn"
	case 5, 6: // notice, info
		return "info"
	case 7:
		return "debug"
	}
	return ""
}
