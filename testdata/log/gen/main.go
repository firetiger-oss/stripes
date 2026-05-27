// Command gen builds the testdata/log/otlp.binpb fixture: a small,
// realistic OTLP LogsData payload exercising the renderer's
// branches (multiple resources, mixed severities, trace
// correlation, structured attribute values). Re-run with
//
//	go run ./testdata/log/gen
//
// from the repo root to regenerate the file in place.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/proto"
)

func main() {
	ld := buildLogsData()
	bin, err := proto.Marshal(ld)
	if err != nil {
		fmt.Fprintln(os.Stderr, "marshal:", err)
		os.Exit(1)
	}
	out := filepath.Join("testdata", "log", "otlp.binpb")
	if err := os.WriteFile(out, bin, 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %d bytes to %s\n", len(bin), out)
}

func buildLogsData() *logsv1.LogsData {
	// Fixed wall-clock so the marshalled bytes are reproducible:
	// re-running the generator on the same git commit yields an
	// identical file.
	t0 := time.Date(2026, 5, 26, 22, 55, 0, 0, time.UTC)

	gatewayScope := &commonv1.InstrumentationScope{
		Name:    "github.com/firetiger-oss/api-gateway",
		Version: "1.4.2",
	}
	dbScope := &commonv1.InstrumentationScope{
		Name:    "github.com/lib/pq",
		Version: "1.10.9",
	}

	traceA := []byte{0xab, 0x12, 0xcd, 0x34, 0xef, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x12, 0x34, 0x56, 0x78, 0x9a}
	spanA := []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80}
	spanB := []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x81}

	return &logsv1.LogsData{
		ResourceLogs: []*logsv1.ResourceLogs{
			{
				Resource: resource(
					kvStr("service.name", "api-gateway"),
					kvStr("service.namespace", "prod"),
					kvStr("host.name", "api-1.us-west-2.internal"),
					kvStr("service.version", "1.4.2"),
				),
				ScopeLogs: []*logsv1.ScopeLogs{{
					Scope: gatewayScope,
					LogRecords: []*logsv1.LogRecord{
						info(t0.Add(0*time.Millisecond), "server listening",
							kvStr("addr", ":8080"),
							kvInt("workers", 16),
						),
						withTrace(info(t0.Add(127*time.Millisecond), "request started",
							kvStr("http.method", "POST"),
							kvStr("http.target", "/v1/logs"),
							kvStr("client.ip", "100.21.176.113"),
						), traceA, spanA),
						withTrace(debug(t0.Add(132*time.Millisecond), "authorized request",
							kvStr("user.id", "alice"),
							kvStr("auth.scheme", "bearer"),
						), traceA, spanA),
						withTrace(warn(t0.Add(891*time.Millisecond), "downstream timeout, retrying",
							kvStr("target", "postgres.prod.svc:5432"),
							kvInt("attempt", 1),
							kvDouble("elapsed_ms", 764.3),
						), traceA, spanA),
						withTrace(errorRec(t0.Add(2500*time.Millisecond), "request failed after retries",
							kvStr("http.method", "POST"),
							kvStr("http.target", "/v1/logs"),
							kvInt("http.status_code", 503),
							kvStr("error.kind", "deadline_exceeded"),
						), traceA, spanA),
					},
				}},
			},
			{
				Resource: resource(
					kvStr("service.name", "postgres"),
					kvStr("host.name", "postgres-primary.prod"),
				),
				ScopeLogs: []*logsv1.ScopeLogs{{
					Scope: dbScope,
					LogRecords: []*logsv1.LogRecord{
						info(t0.Add(45*time.Millisecond), "connection accepted",
							kvStr("client.ip", "10.0.0.5"),
							kvInt("backend_pid", 8421),
						),
						withTrace(warn(t0.Add(782*time.Millisecond), "slow query",
							kvStr("query.sql", "SELECT * FROM logs WHERE service_id = $1 AND ts > $2"),
							kvDouble("duration_ms", 764.3),
							kvInt("rows", 1284731),
						), traceA, spanB),
						fatal(t0.Add(2700*time.Millisecond),
							"out of memory (cannot allocate 256 MB)\n"+
								"CONTEXT:  while vacuuming relation \"public.logs\"\n"+
								"  automatic vacuum of table \"prod.public.logs\"",
							kvStr("operation", "vacuum"),
							kvInt("memory.used_bytes", 17179869184),
							kvStr("stack",
								"Traceback (most recent call last):\n"+
									"  File \"/app/handler.py\", line 142, in process\n"+
									"    raise MemoryError(\"oom\")\n"+
									"MemoryError: oom"),
						),
					},
				}},
			},
		},
	}
}

// record builds a LogRecord with the given timestamp, severity,
// and body. The shorthand wrappers (info/debug/warn/errorRec/fatal)
// fix the severity so the table-style declarations in buildLogsData
// read fluently.
func record(ts time.Time, sev logsv1.SeverityNumber, sevText, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return &logsv1.LogRecord{
		TimeUnixNano:   uint64(ts.UnixNano()),
		SeverityNumber: sev,
		SeverityText:   sevText,
		Body:           &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: body}},
		Attributes:     attrs,
	}
}

// withTrace mutates r in place to add trace correlation and returns
// it, so calls compose inline within slice literals.
func withTrace(r *logsv1.LogRecord, traceID, spanID []byte) *logsv1.LogRecord {
	r.TraceId = traceID
	r.SpanId = spanID
	return r
}

func info(ts time.Time, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return record(ts, logsv1.SeverityNumber_SEVERITY_NUMBER_INFO, "INFO", body, attrs...)
}
func debug(ts time.Time, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return record(ts, logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG, "DEBUG", body, attrs...)
}
func warn(ts time.Time, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return record(ts, logsv1.SeverityNumber_SEVERITY_NUMBER_WARN, "WARN", body, attrs...)
}
func errorRec(ts time.Time, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return record(ts, logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR, "ERROR", body, attrs...)
}
func fatal(ts time.Time, body string, attrs ...*commonv1.KeyValue) *logsv1.LogRecord {
	return record(ts, logsv1.SeverityNumber_SEVERITY_NUMBER_FATAL, "FATAL", body, attrs...)
}

func resource(attrs ...*commonv1.KeyValue) *resourcev1.Resource {
	return &resourcev1.Resource{Attributes: attrs}
}

func kvStr(k, v string) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: k, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: v}}}
}
func kvInt(k string, v int64) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: k, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: v}}}
}
func kvDouble(k string, v float64) *commonv1.KeyValue {
	return &commonv1.KeyValue{Key: k, Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_DoubleValue{DoubleValue: v}}}
}
