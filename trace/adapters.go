package trace

import (
	"iter"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// FromTracesData yields each *ResourceSpans inside td in order.
// Returns an empty iterator when td is nil.
func FromTracesData(td *tracev1.TracesData) iter.Seq[*tracev1.ResourceSpans] {
	return func(yield func(*tracev1.ResourceSpans) bool) {
		if td == nil {
			return
		}
		for _, rs := range td.GetResourceSpans() {
			if !yield(rs) {
				return
			}
		}
	}
}

// FromResourceSpans yields each non-nil *ResourceSpans from rs in
// order. Useful when the caller already has the resource-level slice
// (e.g. from an ExportTraceServiceRequest).
func FromResourceSpans(rs ...*tracev1.ResourceSpans) iter.Seq[*tracev1.ResourceSpans] {
	return func(yield func(*tracev1.ResourceSpans) bool) {
		for _, r := range rs {
			if r == nil {
				continue
			}
			if !yield(r) {
				return
			}
		}
	}
}

// FromScopeSpans wraps the given ScopeSpans into a single synthetic
// ResourceSpans whose resource attributes carry only a service.name
// derived from serviceName. The schema_url is left empty. Use when
// you have scope-level data but no resource context.
func FromScopeSpans(serviceName string, scopes ...*tracev1.ScopeSpans) iter.Seq[*tracev1.ResourceSpans] {
	rs := &tracev1.ResourceSpans{
		Resource:   serviceResource(serviceName),
		ScopeSpans: scopes,
	}
	return FromResourceSpans(rs)
}

// FromSpans wraps a flat slice of spans into a single synthetic
// ResourceSpans / ScopeSpans pair. serviceName is recorded as the
// resource's service.name attribute so the renderer can colour and
// label the spans.
func FromSpans(serviceName string, spans ...*tracev1.Span) iter.Seq[*tracev1.ResourceSpans] {
	rs := &tracev1.ResourceSpans{
		Resource: serviceResource(serviceName),
		ScopeSpans: []*tracev1.ScopeSpans{{
			Spans: spans,
		}},
	}
	return FromResourceSpans(rs)
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
