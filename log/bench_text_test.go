package log_test

import (
	"io"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/log"
)

// repeatLines returns a payload made of n copies of line followed by
// a newline. Used to give each benchmark a stable working set
// independent of the per-format line length.
func repeatLines(line string, n int) string {
	if !strings.HasSuffix(line, "\n") {
		line += "\n"
	}
	return strings.Repeat(line, n)
}

func benchFormat(b *testing.B, contentType, input string) {
	b.Helper()
	r := stripes.Func(contentType, "")
	if r == nil {
		b.Fatalf("no renderer for %q", contentType)
	}
	styles := stripes.DefaultStyles
	reader := strings.NewReader(input)
	b.SetBytes(int64(len(input)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		reader.Reset(input)
		r(io.Discard, reader, styles)
	}
}

func BenchmarkFormatLogfmt(b *testing.B) {
	benchFormat(b, "application/vnd.logfmt",
		repeatLines(`time=2026-01-15T10:23:45.123Z level=INFO msg="hello world" user=alice request_id=abc123 duration_ms=42`, 100))
}

func BenchmarkFormatJSONLog(b *testing.B) {
	benchFormat(b, "application/vnd.json-log",
		repeatLines(`{"time":"2026-01-15T10:23:45Z","level":"info","msg":"hello","user":"alice","request_id":"abc123","duration_ms":42}`, 100))
}

func BenchmarkFormatALB(b *testing.B) {
	benchFormat(b, "application/vnd.amazon.alb-access-log",
		repeatLines(`http 2026-01-15T12:00:00.000000Z app/my-alb/abc 192.0.2.1:54321 10.0.0.5:80 0.001 0.020 0.000 200 200 100 256 "GET http://example.com/ HTTP/1.1" "curl/7.79.0" - - arn:tg "tid" "example.com" "-" 1 2026-01-15T11:59:59.999Z "forward" "-" "-" "10.0.0.5:80" "200" "-" "-" t1`, 100))
}

func BenchmarkFormatNginx(b *testing.B) {
	benchFormat(b, "application/vnd.nginx-access-log",
		repeatLines(`127.0.0.1 - - [05/Feb/2026:17:11:55 +0000] "GET /api/v1/users HTTP/1.1" 200 2048 "https://example.com/" "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36"`, 100))
}

func BenchmarkFormatLog4j(b *testing.B) {
	benchFormat(b, "application/vnd.log4j",
		repeatLines(`[2026-01-15 10:23:45,123] INFO [main] kafka.server.KafkaServer - starting and listening on port 9092`, 100))
}

func BenchmarkFormatPython(b *testing.B) {
	benchFormat(b, "application/vnd.python-log",
		repeatLines(`2026-01-15 10:23:45,123 INFO django.request:200 GET /api/v1/users 200 OK in 49ms`, 100))
}

func BenchmarkFormatGoLog(b *testing.B) {
	benchFormat(b, "application/vnd.go-log",
		repeatLines(`2026/01/15 10:23:45 main.go:42: handler /api/v1/users registered with middleware`, 100))
}

func BenchmarkFormatSyslogBSD(b *testing.B) {
	benchFormat(b, "application/vnd.syslog",
		repeatLines(`Jan 15 10:23:45 myhost systemd[1]: Started example.service.`, 100))
}

func BenchmarkFormatSyslogRFC5424(b *testing.B) {
	benchFormat(b, "application/vnd.syslog-rfc5424",
		repeatLines(`<134>1 2026-01-15T10:23:45.123Z host1.example.com myapp 4242 ID01 - User alice logged in from 10.0.0.1`, 100))
}
