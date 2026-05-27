package log

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// jsonLogFormat renders JSON-per-line structured logs (often
// "JSONL" or "ndjson"), the output shape of zap, slog.JSONHandler,
// pino, bunyan, and many web frameworks. Each line is one JSON
// object; this format recognises the common timestamp / severity /
// body keys, promotes them to the styled prefix, and renders the
// remaining fields inline as key=value pairs.
//
// Recognised key aliases:
//   - timestamp: "time", "ts", "timestamp", "@timestamp"
//   - severity:  "level", "lvl", "severity"
//   - body:      "msg", "message", "body", "event"
var jsonLogFormat = LineFormat{
	Name:        "jsonlog",
	ContentType: "application/vnd.json-log",
	HasLevel:    true,
	Detect:      detectJSONLog,
	Format:      formatJSONLog,
}

func detectJSONLog(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 3) {
		if !strings.HasPrefix(line, "{") {
			return false
		}
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			return false
		}
		if hasJSONLogKey(obj) {
			return true
		}
	}
	return false
}

// hasJSONLogKey reports whether m has at least one key in any of
// the recognised JSONL-log slots. Used by Detect to distinguish a
// log line from an arbitrary JSON object (which the JSON renderer
// should handle).
func hasJSONLogKey(m map[string]json.RawMessage) bool {
	for k := range m {
		switch k {
		case "time", "ts", "timestamp", "@timestamp",
			"level", "lvl", "severity",
			"msg", "message", "body", "event":
			return true
		}
	}
	return false
}

func formatJSONLog(line string, styles *stripes.Styles) (Row, bool) {
	dec := json.NewDecoder(strings.NewReader(line))
	dec.UseNumber()
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return Row{}, false
	}

	row := Row{
		Timestamp: firstStringField(obj, "time", "ts", "timestamp", "@timestamp"),
		Level:     firstStringField(obj, "level", "lvl", "severity"),
		Message:   StyleText(styles).Render(firstStringField(obj, "msg", "message", "body", "event")),
	}

	// Render the remaining keys inline (after the message), in
	// stable sorted order so the same log line always looks the
	// same.
	rest := remainingKeys(obj)
	row.Attrs = make([]KV, 0, len(rest))
	for _, k := range rest {
		row.Attrs = append(row.Attrs, KV{Key: k, Value: decodeJSONScalar(obj[k])})
	}
	return row, true
}

// firstStringField returns the string value of the first matching
// key from keys, or "" if none is present or matches a non-string
// scalar. Numbers and booleans are also stringified so int-encoded
// severities and unix-timestamp numbers pass through unchanged.
func firstStringField(obj map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		raw, ok := obj[k]
		if !ok {
			continue
		}
		delete(obj, k)
		return decodeJSONScalar(raw)
	}
	return ""
}

// decodeJSONScalar renders a json.RawMessage as a flat string for
// inline display. Strings are unquoted; numbers/bools/null pass
// through as-is; arrays/objects render as their compact JSON form
// trimmed of inserted whitespace.
func decodeJSONScalar(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if err := json.Unmarshal(raw, &s); err == nil {
			return s
		}
	}
	switch raw[0] {
	case 't', 'f', 'n':
		return string(raw)
	}
	if raw[0] == '-' || (raw[0] >= '0' && raw[0] <= '9') {
		// Pass numbers through verbatim — json.Number is preserved
		// by UseNumber() above so we don't lose precision.
		return string(raw)
	}
	// Object/array: render compact (no inserted spaces).
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return string(raw)
	}
	return buf.String()
}

// remainingKeys returns the keys still present in obj in sorted
// order. The reserved promoted-keys have already been deleted by
// firstStringField calls.
func remainingKeys(obj map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	// In-place insertion sort — log lines have small attribute
	// counts so this beats a sort.Slice closure allocation.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j-1] > keys[j]; j-- {
			keys[j-1], keys[j] = keys[j], keys[j-1]
		}
	}
	return keys
}
