package trace_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/trace"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

// unstyled returns a Styles value with no ANSI escapes, suitable for
// asserting layout/text in tests without escape-code noise.
func unstyled(width int) *stripes.Styles {
	return &stripes.Styles{Indent: "  ", Width: width}
}

// kv constructs a string-valued OTLP KeyValue, the most common
// attribute shape in tests.
func kv(k, v string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   k,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}},
	}
}

// span builds a *tracev1.Span anchored at the given start with the
// given duration. parent may be nil for roots.
func mkSpan(traceID, spanID, parentID []byte, name string, start time.Time, dur time.Duration, kind tracev1.Span_SpanKind, status tracev1.Status_StatusCode) *tracev1.Span {
	s := &tracev1.Span{
		TraceId:           traceID,
		SpanId:            spanID,
		ParentSpanId:      parentID,
		Name:              name,
		Kind:              kind,
		StartTimeUnixNano: uint64(start.UnixNano()),
		EndTimeUnixNano:   uint64(start.Add(dur).UnixNano()),
	}
	if status != tracev1.Status_STATUS_CODE_UNSET {
		s.Status = &tracev1.Status{Code: status}
	}
	return s
}

// resourceSpans wraps a list of spans under a single service.
func resourceSpans(serviceName string, spans ...*tracev1.Span) *tracev1.ResourceSpans {
	return &tracev1.ResourceSpans{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{kv("service.name", serviceName)},
		},
		ScopeSpans: []*tracev1.ScopeSpans{{Spans: spans}},
	}
}

// trivialTrace returns a small but tree-shaped trace: gateway →
// {auth, db, cache} with realistic durations. Used as the fixture
// for most rendering tests.
func trivialTrace() *tracev1.TracesData {
	t0 := time.Unix(0, 0)
	traceID := []byte{0x4f, 0x3a, 0x91, 0xd2, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	root := []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01}
	authID := []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02}
	dbID := []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03}
	cacheID := []byte{0x10, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x04}

	return &tracev1.TracesData{
		ResourceSpans: []*tracev1.ResourceSpans{
			resourceSpans("api-gateway",
				mkSpan(traceID, root, nil, "GET /api/v1/users", t0, 120*time.Millisecond, tracev1.Span_SPAN_KIND_SERVER, tracev1.Status_STATUS_CODE_OK),
			),
			resourceSpans("auth",
				mkSpan(traceID, authID, root, "auth.verify", t0.Add(2*time.Millisecond), 20*time.Millisecond, tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_UNSET),
			),
			resourceSpans("postgres",
				mkSpan(traceID, dbID, root, "db.users.select", t0.Add(25*time.Millisecond), 75*time.Millisecond, tracev1.Span_SPAN_KIND_CLIENT, tracev1.Status_STATUS_CODE_UNSET),
			),
			resourceSpans("redis",
				mkSpan(traceID, cacheID, root, "cache.set", t0.Add(105*time.Millisecond), 5*time.Millisecond, tracev1.Span_SPAN_KIND_CLIENT, tracev1.Status_STATUS_CODE_UNSET),
			),
		},
	}
}

// TestRenderBasic verifies the top-line layout: header, ruler, and
// per-span rows for a simple 4-span trace. Only checks for expected
// substrings — the full byte-exact rendering is sensitive to a lot of
// implementation details we shouldn't pin here.
func TestRenderBasic(t *testing.T) {
	td := trivialTrace()
	out := trace.Format(trace.FromTracesData(td), trace.WithStyles(unstyled(80)))

	for _, want := range []string{
		"Trace:",                           // title-case header key
		"4f3a91d2000000000000000000000001", // full trace ID (32 hex chars)
		"Root:",
		"GET /api/v1/users", // root op in header
		"Duration:",
		"120.0ms", // root duration in header AND in row (1-decimal forced unit)
		"Spans:",
		"api-gateway", // shown in the Services legend
		"auth.verify",
		"db.users.select",
		"cache.set",
		"├─", // tree connector
		"└─", // last child connector
		"←",  // server-kind glyph (root)
		"→",  // client-kind glyph
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing expected substring %q in output:\n%s", want, out)
		}
	}
}

// TestRenderError exercises the status marker for an ERROR span.
func TestRenderError(t *testing.T) {
	t0 := time.Unix(0, 0)
	traceID := bytes.Repeat([]byte{0xab}, 16)
	rootID := bytes.Repeat([]byte{0xcd}, 8)

	td := &tracev1.TracesData{
		ResourceSpans: []*tracev1.ResourceSpans{
			resourceSpans("svc",
				mkSpan(traceID, rootID, nil, "doomed.call", t0, 10*time.Millisecond, tracev1.Span_SPAN_KIND_CLIENT, tracev1.Status_STATUS_CODE_ERROR),
			),
		},
	}
	out := trace.Format(trace.FromTracesData(td), trace.WithStyles(unstyled(80)))
	if !strings.Contains(out, "✗") {
		t.Errorf("missing error marker ✗ in output:\n%s", out)
	}
	if !strings.Contains(out, "Errors:") || !strings.Contains(out, "1") {
		t.Errorf("missing 'Errors: 1' in header:\n%s", out)
	}
}

// TestRenderVerbose checks that attributes and events appear in
// verbose mode but not in default mode.
func TestRenderVerbose(t *testing.T) {
	t0 := time.Unix(0, 0)
	traceID := bytes.Repeat([]byte{0x11}, 16)
	rootID := bytes.Repeat([]byte{0x22}, 8)
	span := mkSpan(traceID, rootID, nil, "GET /thing", t0, 50*time.Millisecond, tracev1.Span_SPAN_KIND_SERVER, tracev1.Status_STATUS_CODE_ERROR)
	span.Attributes = []*commonv1.KeyValue{
		kv("http.method", "GET"),
		{Key: "http.status_code", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 500}}},
	}
	span.Status = &tracev1.Status{Code: tracev1.Status_STATUS_CODE_ERROR, Message: "kaboom"}
	span.Events = []*tracev1.Span_Event{{
		Name:         "exception",
		TimeUnixNano: uint64(t0.Add(40 * time.Millisecond).UnixNano()),
		Attributes:   []*commonv1.KeyValue{kv("exception.type", "oops")},
	}}

	td := &tracev1.TracesData{ResourceSpans: []*tracev1.ResourceSpans{resourceSpans("svc", span)}}

	compact := trace.Format(trace.FromTracesData(td), trace.WithStyles(unstyled(120)))
	if strings.Contains(compact, "http.method") {
		t.Errorf("compact mode unexpectedly shows attributes:\n%s", compact)
	}

	verbose := trace.Format(trace.FromTracesData(td), trace.WithStyles(unstyled(120)), trace.WithVerbose(true))
	for _, want := range []string{
		"http.method",
		"= GET", // verbose values render unquoted
		"http.status_code",
		"500",
		"status.message",
		"= kaboom",
		"◆",
		"exception",
		"exception.type",
		"= oops",
	} {
		if !strings.Contains(verbose, want) {
			t.Errorf("verbose mode missing %q in output:\n%s", want, verbose)
		}
	}
}

// TestRenderMultipleTraces verifies that two trace_ids in one
// payload render as two separate waterfall blocks, separated by a
// blank line.
func TestRenderMultipleTraces(t *testing.T) {
	t0 := time.Unix(0, 0)
	t1 := []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	t2 := []byte{0xaa, 0xbb, 0xcc, 0xdd, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10}
	td := &tracev1.TracesData{
		ResourceSpans: []*tracev1.ResourceSpans{
			resourceSpans("svc",
				mkSpan(t1, []byte("aaaaaaaa"), nil, "op-A", t0, 10*time.Millisecond, tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_UNSET),
				mkSpan(t2, []byte("bbbbbbbb"), nil, "op-B", t0.Add(time.Second), 5*time.Millisecond, tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_UNSET),
			),
		},
	}
	out := trace.Format(trace.FromTracesData(td), trace.WithStyles(unstyled(80)))
	if !strings.Contains(out, "01020304") || !strings.Contains(out, "aabbccdd") {
		t.Errorf("missing one or both trace IDs in:\n%s", out)
	}
	if !strings.Contains(out, "op-A") || !strings.Contains(out, "op-B") {
		t.Errorf("missing one or both span names in:\n%s", out)
	}
	if strings.Count(out, "\n\n") < 1 {
		t.Errorf("expected at least one blank-line separator between traces in:\n%s", out)
	}
}

// TestZeroDurationSpan checks that zero-duration spans render the
// vertical-tick glyph rather than a bar.
func TestZeroDurationSpan(t *testing.T) {
	t0 := time.Unix(0, 0)
	traceID := bytes.Repeat([]byte{0x33}, 16)
	rootID := bytes.Repeat([]byte{0x44}, 8)
	instantID := bytes.Repeat([]byte{0x55}, 8)
	td := &tracev1.TracesData{
		ResourceSpans: []*tracev1.ResourceSpans{
			resourceSpans("svc",
				mkSpan(traceID, rootID, nil, "parent", t0, 10*time.Millisecond, tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_UNSET),
				mkSpan(traceID, instantID, rootID, "instant", t0.Add(5*time.Millisecond), 0, tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_UNSET),
			),
		},
	}
	out := trace.Format(trace.FromTracesData(td), trace.WithStyles(unstyled(100)))
	if !strings.Contains(out, "│") {
		t.Errorf("missing zero-duration tick │ in output:\n%s", out)
	}
}

// TestNewRendererBinary decodes a TracesData via the byte-stream API
// (the entrypoint used by stripes.Func) and checks the resulting
// output matches the structured-API output.
func TestNewRendererBinary(t *testing.T) {
	td := trivialTrace()
	bin, err := proto.Marshal(td)
	if err != nil {
		t.Fatal(err)
	}
	r := trace.NewRenderer(td.ProtoReflect().Descriptor(), nil)
	var buf bytes.Buffer
	r(&buf, bytes.NewReader(bin), unstyled(80))
	if !strings.Contains(buf.String(), "GET /api/v1/users") {
		t.Errorf("byte-stream renderer didn't produce expected content:\n%s", buf.String())
	}
}

// TestFromSpans wraps a flat slice of spans with an injected service
// name. Confirms the helper produces a valid ResourceSpans iterator.
func TestFromSpans(t *testing.T) {
	t0 := time.Unix(0, 0)
	traceID := bytes.Repeat([]byte{0xfe}, 16)
	rootID := bytes.Repeat([]byte{0x99}, 8)
	span := mkSpan(traceID, rootID, nil, "standalone", t0, time.Millisecond, tracev1.Span_SPAN_KIND_INTERNAL, tracev1.Status_STATUS_CODE_UNSET)
	out := trace.Format(trace.FromSpans("custom-svc", span), trace.WithStyles(unstyled(80)))
	if !strings.Contains(out, "custom-svc") {
		t.Errorf("expected custom-svc in output:\n%s", out)
	}
	if !strings.Contains(out, "standalone") {
		t.Errorf("expected span name in output:\n%s", out)
	}
}

// TestEmptyInput verifies that an empty input produces no output and
// no error.
func TestEmptyInput(t *testing.T) {
	out := trace.Format(trace.FromTracesData(&tracev1.TracesData{}), trace.WithStyles(unstyled(80)))
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

// TestIsTraceMessage covers the OTLP-trace schema-name predicate
// used by the CLI for auto-routing.
func TestIsTraceMessage(t *testing.T) {
	for _, name := range []string{
		"opentelemetry.proto.trace.v1.TracesData",
		"opentelemetry.proto.trace.v1.ResourceSpans",
		"opentelemetry.proto.trace.v1.ScopeSpans",
		"opentelemetry.proto.trace.v1.Span",
	} {
		if !trace.IsTraceMessage(name) {
			t.Errorf("IsTraceMessage(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"opentelemetry.proto.metrics.v1.MetricsData",
		"google.protobuf.Empty",
		"",
	} {
		if trace.IsTraceMessage(name) {
			t.Errorf("IsTraceMessage(%q) = true, want false", name)
		}
	}
}
