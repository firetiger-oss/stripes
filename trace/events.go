package trace

import (
	"strings"

	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// spanErrorMessage returns a single-line error description for s,
// suitable for inline overlay on the bar. The lookup order matches
// the OTel conventions:
//
//  1. status.message when status.code is ERROR and message is set.
//  2. The first "exception" event's `exception.message` attribute.
//  3. Empty string when neither applies (status is not ERROR, or
//     ERROR with no message anywhere).
//
// Only the first line of the message is returned so the overlay
// stays one row tall regardless of how chatty an SDK is.
func spanErrorMessage(s *span) string {
	if s.pb.GetStatus().GetCode() != tracev1.Status_STATUS_CODE_ERROR {
		return ""
	}
	if msg := s.pb.GetStatus().GetMessage(); msg != "" {
		return firstLine(msg)
	}
	for _, ev := range s.pb.GetEvents() {
		if ev.GetName() != "exception" {
			continue
		}
		for _, kv := range ev.GetAttributes() {
			if kv.GetKey() == "exception.message" {
				if v := kv.GetValue().GetStringValue(); v != "" {
					return firstLine(v)
				}
			}
		}
	}
	return ""
}

// firstLine returns s up to the first '\n', exclusive. Strings
// without a newline are returned unchanged.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
