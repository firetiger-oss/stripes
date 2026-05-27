package log

import (
	"iter"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

// FromLogsData yields each *ResourceLogs inside ld in order. Returns
// an empty iterator when ld is nil.
func FromLogsData(ld *logsv1.LogsData) iter.Seq[*logsv1.ResourceLogs] {
	return func(yield func(*logsv1.ResourceLogs) bool) {
		if ld == nil {
			return
		}
		for _, rl := range ld.GetResourceLogs() {
			if !yield(rl) {
				return
			}
		}
	}
}

// FromResourceLogs yields each non-nil *ResourceLogs from rl in
// order. Useful when the caller already has the resource-level slice
// (e.g. from an ExportLogsServiceRequest).
func FromResourceLogs(rl ...*logsv1.ResourceLogs) iter.Seq[*logsv1.ResourceLogs] {
	return func(yield func(*logsv1.ResourceLogs) bool) {
		for _, r := range rl {
			if r == nil {
				continue
			}
			if !yield(r) {
				return
			}
		}
	}
}

// FromScopeLogs wraps the given ScopeLogs into a single synthetic
// ResourceLogs whose resource attributes carry only a service.name
// derived from serviceName. The schema_url is left empty. Use when
// you have scope-level data but no resource context.
func FromScopeLogs(serviceName string, scopes ...*logsv1.ScopeLogs) iter.Seq[*logsv1.ResourceLogs] {
	rl := &logsv1.ResourceLogs{
		Resource:  serviceResource(serviceName),
		ScopeLogs: scopes,
	}
	return FromResourceLogs(rl)
}

// FromLogRecords wraps a flat slice of records into a single synthetic
// ResourceLogs / ScopeLogs pair. serviceName is recorded as the
// resource's service.name attribute so the renderer can label the
// records.
func FromLogRecords(serviceName string, records ...*logsv1.LogRecord) iter.Seq[*logsv1.ResourceLogs] {
	rl := &logsv1.ResourceLogs{
		Resource: serviceResource(serviceName),
		ScopeLogs: []*logsv1.ScopeLogs{{
			LogRecords: records,
		}},
	}
	return FromResourceLogs(rl)
}

// serviceResource constructs a minimal Resource carrying only
// service.name. Returns nil when name is empty so the renderer's
// "unknown_service" fallback kicks in.
func serviceResource(name string) *resourcev1.Resource {
	if name == "" {
		return nil
	}
	return &resourcev1.Resource{
		Attributes: []*commonv1.KeyValue{{
			Key:   "service.name",
			Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: name}},
		}},
	}
}
