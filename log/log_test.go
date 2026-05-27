package log_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/log"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// unstyled returns a Styles value with no ANSI escapes, suitable for
// asserting layout/text in tests without escape-code noise.
func unstyled(width int) *stripes.Styles {
	return &stripes.Styles{Indent: "  ", Width: width}
}

// kv constructs a string-valued OTLP KeyValue.
func kv(k, v string) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   k,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}},
	}
}

// kvi constructs an int-valued OTLP KeyValue.
func kvi(k string, v int64) *commonv1.KeyValue {
	return &commonv1.KeyValue{
		Key:   k,
		Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: v}},
	}
}

// mkRecord builds a LogRecord with the given timestamp, severity,
// body and attributes.
func mkRecord(ts time.Time, sev logsv1.SeverityNumber, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return &logsv1.LogRecord{
		TimeUnixNano:   uint64(ts.UnixNano()),
		SeverityNumber: sev,
		Body:           &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: body}},
		Attributes:     attrs,
	}
}

// resourceLogs wraps a list of records under a single service.
func resourceLogs(serviceName string, records ...*logsv1.LogRecord) *logsv1.ResourceLogs {
	return &logsv1.ResourceLogs{
		Resource: &resourcev1.Resource{
			Attributes: []*commonv1.KeyValue{kv("service.name", serviceName)},
		},
		ScopeLogs: []*logsv1.ScopeLogs{{LogRecords: records}},
	}
}

// trivialLogs returns a small two-resource batch: an api-gateway
// with INFO and WARN records, plus a db service with an ERROR.
func trivialLogs() *logsv1.LogsData {
	t0 := time.Date(2026, 1, 15, 12, 4, 1, 234_000_000, time.UTC)
	return &logsv1.LogsData{
		ResourceLogs: []*logsv1.ResourceLogs{
			resourceLogs("api-gateway",
				mkRecord(t0, logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
					"user.login",
					kv("user", "alice"),
				),
				mkRecord(t0.Add(57*time.Millisecond), logsv1.SeverityNumber_SEVERITY_NUMBER_WARN,
					"close to limit",
					kvi("rate", 87),
				),
			),
			resourceLogs("postgres",
				mkRecord(t0.Add(767*time.Millisecond), logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR,
					"timeout after 5s",
				),
			),
		},
	}
}

// TestRenderBasic verifies the per-resource header and a few row
// substrings for a small fixture.
func TestRenderBasic(t *testing.T) {
	out := log.Format(log.FromLogsData(trivialLogs()), log.WithStyles(unstyled(100)))
	// trivialLogs builds records at 12:04:01.234 UTC. Asserting on
	// the exact local-time prefix would tie the test to the host's
	// timezone, so check the date / fractional seconds shape and
	// the timezone-independent ":04:01.234" sub-string instead.
	for _, want := range []string{
		"service=api-gateway",
		"service=postgres",
		"INFO",
		"WARN",
		"ERRO",
		"user.login",
		`user │ alice`,
		"close to limit",
		"timeout after 5s",
		"2026/01/15",
		":04:01.234",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing expected substring %q in output:\n%s", want, out)
		}
	}
}

// TestRenderVerbose asserts the detail block expands attributes and
// trace correlation under each row.
func TestRenderVerbose(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	rec := mkRecord(t0, logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR, "boom",
		kv("http.method", "POST"),
		kvi("http.status_code", 500),
	)
	rec.TraceId = bytes.Repeat([]byte{0xab}, 16)
	rec.SpanId = bytes.Repeat([]byte{0xcd}, 8)
	ld := &logsv1.LogsData{
		ResourceLogs: []*logsv1.ResourceLogs{resourceLogs("svc", rec)},
	}

	compact := log.Format(log.FromLogsData(ld), log.WithStyles(unstyled(120)))
	if strings.Contains(compact, "trace_id") {
		t.Errorf("compact mode unexpectedly shows trace_id:\n%s", compact)
	}

	verbose := log.Format(log.FromLogsData(ld), log.WithStyles(unstyled(120)), log.WithVerbose(true))
	for _, want := range []string{
		"http.method",
		"POST",
		"http.status_code",
		"500",
		"trace_id",
		strings.Repeat("ab", 16),
		"span_id",
		strings.Repeat("cd", 8),
	} {
		if !strings.Contains(verbose, want) {
			t.Errorf("verbose mode missing %q in output:\n%s", want, verbose)
		}
	}
}

// TestRenderEmpty verifies that empty input produces no output and
// no error.
func TestRenderEmpty(t *testing.T) {
	out := log.Format(log.FromLogsData(&logsv1.LogsData{}), log.WithStyles(unstyled(80)))
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}

// TestNewRendererBinary feeds a marshalled LogsData through the
// byte-stream renderer and checks the output matches the structured
// API path.
func TestNewRendererBinary(t *testing.T) {
	ld := trivialLogs()
	bin, err := proto.Marshal(ld)
	if err != nil {
		t.Fatal(err)
	}
	r := log.NewRenderer(ld.ProtoReflect().Descriptor(), nil)
	var buf bytes.Buffer
	r(&buf, bytes.NewReader(bin), unstyled(100))
	if !strings.Contains(buf.String(), "user.login") {
		t.Errorf("byte-stream renderer didn't produce expected content:\n%s", buf.String())
	}
}

// TestNewJSONRenderer mirrors TestNewRendererBinary for protojson.
func TestNewJSONRenderer(t *testing.T) {
	ld := trivialLogs()
	jsonBytes, err := protojson.Marshal(ld)
	if err != nil {
		t.Fatal(err)
	}
	r := log.NewJSONRenderer(ld.ProtoReflect().Descriptor(), nil)
	var buf bytes.Buffer
	r(&buf, bytes.NewReader(jsonBytes), unstyled(100))
	if !strings.Contains(buf.String(), "user.login") {
		t.Errorf("json renderer didn't produce expected content:\n%s", buf.String())
	}
}

// TestFromScopeLogs wraps a scope-level value with an injected
// service name.
func TestFromScopeLogs(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	sl := &logsv1.ScopeLogs{
		Scope: &commonv1.InstrumentationScope{Name: "test-scope"},
		LogRecords: []*logsv1.LogRecord{
			mkRecord(t0, logsv1.SeverityNumber_SEVERITY_NUMBER_INFO, "hello"),
		},
	}
	out := log.Format(log.FromScopeLogs("custom-svc", sl), log.WithStyles(unstyled(80)))
	if !strings.Contains(out, "custom-svc") {
		t.Errorf("expected custom-svc in output:\n%s", out)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("expected body in output:\n%s", out)
	}
}

// TestFromLogRecords wraps a flat slice.
func TestFromLogRecords(t *testing.T) {
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	rec := mkRecord(t0, logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG, "diag")
	out := log.Format(log.FromLogRecords("only-svc", rec), log.WithStyles(unstyled(80)))
	if !strings.Contains(out, "only-svc") {
		t.Errorf("expected only-svc in output:\n%s", out)
	}
	if !strings.Contains(out, "DEBU") || !strings.Contains(out, "diag") {
		t.Errorf("expected DEBU row in output:\n%s", out)
	}
}

// TestIsLogMessage covers the OTLP-logs schema-name predicate used
// by the CLI for auto-routing.
func TestIsLogMessage(t *testing.T) {
	for _, name := range []string{
		"opentelemetry.proto.logs.v1.LogsData",
		"opentelemetry.proto.logs.v1.ResourceLogs",
		"opentelemetry.proto.logs.v1.ScopeLogs",
		"opentelemetry.proto.logs.v1.LogRecord",
	} {
		if !log.IsLogMessage(name) {
			t.Errorf("IsLogMessage(%q) = false, want true", name)
		}
	}
	for _, name := range []string{
		"opentelemetry.proto.trace.v1.TracesData",
		"opentelemetry.proto.metrics.v1.MetricsData",
		"google.protobuf.Empty",
		"",
	} {
		if log.IsLogMessage(name) {
			t.Errorf("IsLogMessage(%q) = true, want false", name)
		}
	}
}

// TestClassifySeverity covers a few representative tokens for the
// text-log classifier shared with the text-log renderer.
func TestClassifySeverity(t *testing.T) {
	cases := map[string]log.SeverityClass{
		"":          log.SevUnknown,
		"DEBUG":     log.SevDebug,
		"info":      log.SevInfo,
		"Info":      log.SevInfo,
		"warn":      log.SevWarn,
		"WARNING":   log.SevWarn,
		"ERROR":     log.SevError,
		"err":       log.SevError,
		"CRITICAL":  log.SevFatal,
		"emergency": log.SevFatal,
		"???":       log.SevUnknown,
	}
	for in, want := range cases {
		if got := log.ClassifySeverity(in); got != want {
			t.Errorf("ClassifySeverity(%q) = %v, want %v", in, got, want)
		}
	}
}
