package log_test

import (
	"io"
	"testing"
	"time"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/log"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
)

// buildSyntheticLogs returns a LogsData with `records` log entries
// spread across a small pool of services. The shape exercises the
// per-resource grouping and the row-rendering hot path.
func buildSyntheticLogs(records int) *logsv1.LogsData {
	services := []string{"api-gateway", "auth", "postgres", "redis"}
	t0 := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	rls := make([]*logsv1.ResourceLogs, 0, len(services))
	for _, svc := range services {
		rls = append(rls, &logsv1.ResourceLogs{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
				{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: svc}}},
			}},
			ScopeLogs: []*logsv1.ScopeLogs{{}},
		})
	}
	severities := []logsv1.SeverityNumber{
		logsv1.SeverityNumber_SEVERITY_NUMBER_DEBUG,
		logsv1.SeverityNumber_SEVERITY_NUMBER_INFO,
		logsv1.SeverityNumber_SEVERITY_NUMBER_WARN,
		logsv1.SeverityNumber_SEVERITY_NUMBER_ERROR,
	}
	for i := 0; i < records; i++ {
		bucket := i % len(services)
		rls[bucket].ScopeLogs[0].LogRecords = append(rls[bucket].ScopeLogs[0].LogRecords, &logsv1.LogRecord{
			TimeUnixNano:   uint64(t0.Add(time.Duration(i) * time.Millisecond).UnixNano()),
			SeverityNumber: severities[i%len(severities)],
			Body:           &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "synthetic event"}},
			Attributes: []*commonv1.KeyValue{
				{Key: "request.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "abc123"}}},
				{Key: "user.id", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: int64(i)}}},
			},
		})
	}
	return &logsv1.LogsData{ResourceLogs: rls}
}

func BenchmarkFormatterRenderSmall(b *testing.B) {
	ld := buildSyntheticLogs(10)
	f := log.New(log.WithStyles(stripes.DefaultStyles), log.WithWidth(120))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, log.FromLogsData(ld))
	}
}

func BenchmarkFormatterRenderLarge(b *testing.B) {
	ld := buildSyntheticLogs(500)
	f := log.New(log.WithStyles(stripes.DefaultStyles), log.WithWidth(120))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, log.FromLogsData(ld))
	}
}

func BenchmarkFormatterRenderUnstyled(b *testing.B) {
	ld := buildSyntheticLogs(100)
	styles := &stripes.Styles{Width: 120}
	f := log.New(log.WithStyles(styles))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, log.FromLogsData(ld))
	}
}

func BenchmarkFormatterRenderVerbose(b *testing.B) {
	ld := buildSyntheticLogs(100)
	f := log.New(log.WithStyles(stripes.DefaultStyles), log.WithWidth(120), log.WithVerbose(true))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = f.Render(io.Discard, log.FromLogsData(ld))
	}
}
