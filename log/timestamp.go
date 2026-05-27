package log

import (
	"strings"
	"time"
)

// canonicalTimestampLayout is the unified shape every text log
// format renders its timestamp in. Picked to fit the common
// engineering "yyyy/mm/dd hh:mm:ss.mmm" shorthand, in the local
// timezone of the process running stripes, so timestamps line up
// across heterogeneous log sources on one terminal.
const canonicalTimestampLayout = "2006/01/02 15:04:05.000"

// canonicalTimestampWidth is the printed width of
// canonicalTimestampLayout. Exported so the row renderer can pad to
// it deterministically.
const canonicalTimestampWidth = 23

// NormalizeTimestamp parses raw against the timestamp shapes the
// built-in text-log formats produce and returns the canonical
// "yyyy/mm/dd hh:mm:ss.mmm" representation in local time. Returns
// raw unchanged when it doesn't match any known layout — so
// fixtures with unusual shapes still pass through legibly rather
// than being dropped.
//
// Timezone handling:
//   - Inputs with explicit tz info (RFC 3339 / Apache log /
//     RFC 5424 syslog) are converted to local time.
//   - Inputs without tz info (Go log, log4j, python logging, plain
//     "YYYY-MM-DD HH:MM:SS") are parsed as local time directly.
//   - BSD syslog ("Jan 15 10:23:45") has no year; the current year
//     is substituted, matching journald's convention.
func NormalizeTimestamp(raw string) string {
	if raw == "" {
		return ""
	}

	// RFC 3339 / ISO 8601 with optional sub-second precision and tz.
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return t.Local().Format(canonicalTimestampLayout)
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.Local().Format(canonicalTimestampLayout)
	}

	// Python logging / log4j use a comma between seconds and
	// milliseconds. Swap to "." and retry the standard layout.
	if i := strings.IndexByte(raw, ','); i >= 0 {
		cleaned := raw[:i] + "." + raw[i+1:]
		if t, err := time.ParseInLocation("2006-01-02 15:04:05.000", cleaned, time.Local); err == nil {
			return t.Format(canonicalTimestampLayout)
		}
	}

	// Apache / NGINX combined log format.
	if t, err := time.Parse("02/Jan/2006:15:04:05 -0700", raw); err == nil {
		return t.Local().Format(canonicalTimestampLayout)
	}

	// Go stdlib log (Ldate|Ltime, optional Lmicroseconds).
	for _, layout := range []string{"2006/01/02 15:04:05.999999", "2006/01/02 15:04:05"} {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			return t.Format(canonicalTimestampLayout)
		}
	}

	// Plain "YYYY-MM-DD HH:MM:SS" (no fractional, no tz).
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", raw, time.Local); err == nil {
		return t.Format(canonicalTimestampLayout)
	}

	// BSD syslog / journald short form: month, day, time only.
	for _, layout := range []string{"Jan _2 15:04:05", "Jan 2 15:04:05", "Jan _2 15:04:05.000000"} {
		if t, err := time.ParseInLocation(layout, raw, time.Local); err == nil {
			// Year is zero in the parsed value — substitute the
			// current year so the canonical output isn't anchored on
			// year 0000.
			t = time.Date(time.Now().Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.Local)
			return t.Format(canonicalTimestampLayout)
		}
	}

	return raw
}
