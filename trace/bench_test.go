package trace_test

import (
	"io"
	"testing"
	"time"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/trace"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// buildSyntheticTrace returns a TracesData with one trace_id containing
// a configurable number of spans across a fixed pool of services. The
// shape is deliberately wide-and-shallow (one root, N children) so the
// benchmark exercises the typical waterfall hot path: bar rendering,
// column-width layout, and per-row styling.
func buildSyntheticTrace(spans int) *tracev1.TracesData {
	services := []string{"api-gateway", "auth", "postgres", "redis", "kafka", "cache"}
	rs := make([]*tracev1.ResourceSpans, 0, len(services))
	for _, svc := range services {
		rs = append(rs, &tracev1.ResourceSpans{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
				{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: svc}}},
			}},
			ScopeSpans: []*tracev1.ScopeSpans{{}},
		})
	}

	traceID := []byte{0x4f, 0x3a, 0x91, 0xd2, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}
	rootID := []byte{0x10, 0, 0, 0, 0, 0, 0, 1}
	t0 := time.Unix(0, 0)
	rs[0].ScopeSpans[0].Spans = append(rs[0].ScopeSpans[0].Spans, &tracev1.Span{
		TraceId:           traceID,
		SpanId:            rootID,
		Name:              "GET /api/v1/users",
		Kind:              tracev1.Span_SPAN_KIND_SERVER,
		StartTimeUnixNano: uint64(t0.UnixNano()),
		EndTimeUnixNano:   uint64(t0.Add(120 * time.Millisecond).UnixNano()),
	})

	for i := 1; i < spans; i++ {
		spanID := []byte{0x10, byte(i >> 24), byte(i >> 16), byte(i >> 8), byte(i), 0, 0, 0}
		bucket := i % len(services)
		rs[bucket].ScopeSpans[0].Spans = append(rs[bucket].ScopeSpans[0].Spans, &tracev1.Span{
			TraceId:           traceID,
			SpanId:            spanID,
			ParentSpanId:      rootID,
			Name:              "child.span.op",
			Kind:              tracev1.Span_SPAN_KIND_INTERNAL,
			StartTimeUnixNano: uint64(t0.Add(time.Duration(i) * time.Millisecond).UnixNano()),
			EndTimeUnixNano:   uint64(t0.Add(time.Duration(i+10) * time.Millisecond).UnixNano()),
		})
	}
	return &tracev1.TracesData{ResourceSpans: rs}
}

func BenchmarkFormatterRenderSmall(b *testing.B) {
	td := buildSyntheticTrace(10)
	f := trace.New(trace.WithStyles(stripes.DefaultStyles), trace.WithWidth(120))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, trace.FromTracesData(td))
	}
}

func BenchmarkFormatterRenderLarge(b *testing.B) {
	td := buildSyntheticTrace(500)
	f := trace.New(trace.WithStyles(stripes.DefaultStyles), trace.WithWidth(120))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, trace.FromTracesData(td))
	}
}

func BenchmarkFormatterRenderUnstyled(b *testing.B) {
	td := buildSyntheticTrace(100)
	styles := &stripes.Styles{Width: 120}
	f := trace.New(trace.WithStyles(styles))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, trace.FromTracesData(td))
	}
}

func BenchmarkFormatterRenderVerbose(b *testing.B) {
	td := buildSyntheticTrace(100)
	// Decorate each span with a handful of attributes so verbose mode
	// has meaningful work.
	for _, rs := range td.GetResourceSpans() {
		for _, ss := range rs.GetScopeSpans() {
			for _, s := range ss.GetSpans() {
				s.Attributes = []*commonv1.KeyValue{
					{Key: "http.method", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
					{Key: "http.status_code", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 200}}},
					{Key: "service.version", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "v1.2.3"}}},
				}
			}
		}
	}
	f := trace.New(trace.WithStyles(stripes.DefaultStyles), trace.WithWidth(120), trace.WithVerbose(true))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, trace.FromTracesData(td))
	}
}
