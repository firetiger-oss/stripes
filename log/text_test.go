package log_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/log"
)

// renderWith invokes the renderer registered for contentType against
// input and returns the resulting string. Used by every per-format
// test below so the test reads as "feed bytes through the registry
// just like the CLI does." Reuses [unstyled] from log_test.go (same
// _test package).
func renderWith(t *testing.T, contentType, input string) string {
	t.Helper()
	r := stripes.Func(contentType, "")
	if r == nil {
		t.Fatalf("no renderer registered for %q", contentType)
	}
	var buf bytes.Buffer
	r(&buf, strings.NewReader(input), unstyled(120))
	return buf.String()
}

// formatCase pairs an input log line with the substrings that
// renderer's output must contain. Bundles per-format tests into a
// table-driven layout.
type formatCase struct {
	name        string
	contentType string
	input       string
	wantContain []string
}

func TestFormatRenders(t *testing.T) {
	cases := []formatCase{
		// Timestamp assertions check the canonical "2026/01/15"
		// prefix and the ":45.000" fractional-seconds suffix rather
		// than the full string, since the absolute hour depends on
		// the test host's local timezone after RFC 3339 → local
		// conversion.
		{
			name:        "logfmt",
			contentType: "application/vnd.logfmt",
			input:       `time=2026-01-15T10:23:45Z level=INFO msg="user logged in" user=alice` + "\n",
			wantContain: []string{"INFO", "user logged in", "user │ alice", "2026/01/15", ":45.000"},
		},
		{
			name:        "jsonlog",
			contentType: "application/vnd.json-log",
			input:       `{"time":"2026-01-15T10:23:45Z","level":"info","msg":"hello","user":"alice"}` + "\n",
			wantContain: []string{"INFO", "hello", "user │ alice", "2026/01/15", ":45.000"},
		},
		// HTTP access logs intentionally omit the LEVEL column —
		// the status code colour carries severity already, so a
		// duplicate INFO/WARN/ERRO would be noise.
		{
			name:        "alb-access",
			contentType: "application/vnd.amazon.alb-access-log",
			input:       `http 2026-01-15T12:00:00.000000Z app/my-alb/abc 192.0.2.1:54321 10.0.0.5:80 0.001 0.020 0.000 200 200 100 256 "GET http://example.com/ HTTP/1.1" "curl/7.79.0" - - arn:tg "tid" "example.com" "-" 1 2026-01-15T11:59:59.999Z "forward" "-" "-" "10.0.0.5:80" "200" "-" "-" t1` + "\n",
			wantContain: []string{"HTTP 200", "GET http://example.com/ HTTP/1.1", "client:port              │ 192.0.2.1", "type                     │ http", "sent_bytes               │ 256"},
		},
		{
			name:        "nginx-access",
			contentType: "application/vnd.nginx-access-log",
			input:       `127.0.0.1 - - [05/Feb/2026:17:11:55 +0000] "GET / HTTP/1.1" 200 612 "-" "curl/7"` + "\n",
			wantContain: []string{"HTTP 200", "GET / HTTP/1.1", "remote_addr     │ 127.0.0.1", "body_bytes_sent │ 612"},
		},
		{
			name:        "log4j",
			contentType: "application/vnd.log4j",
			input:       `[2026-01-15 10:23:45,123] INFO [main] kafka.server.KafkaServer - starting` + "\n",
			wantContain: []string{"INFO", "kafka.server.KafkaServer", "starting", "main"},
		},
		{
			name:        "python-log",
			contentType: "application/vnd.python-log",
			input:       `2026-01-15 10:23:45,123 INFO root:42 application started` + "\n",
			wantContain: []string{"INFO", "root:42", "application started"},
		},
		{
			name:        "go-log",
			contentType: "application/vnd.go-log",
			input:       `2026/01/15 10:23:45 main.go:42: handler registered` + "\n",
			wantContain: []string{"2026/01/15", "main.go:42", "handler registered"},
		},
		{
			name:        "syslog-bsd",
			contentType: "application/vnd.syslog",
			input:       `Jan 15 10:23:45 myhost systemd[1]: Started example.service.` + "\n",
			wantContain: []string{"systemd[1]", "host │ myhost", "Started example.service."},
		},
		{
			name:        "syslog-rfc5424",
			contentType: "application/vnd.syslog-rfc5424",
			input:       `<134>1 2003-10-11T22:14:15.003Z mymachine.example.com evntslog - ID47 - An app event` + "\n",
			wantContain: []string{"INFO", "evntslog", "host   │ mymachine.example.com", "An app event"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := renderWith(t, c.contentType, c.input)
			for _, want := range c.wantContain {
				if !strings.Contains(out, want) {
					t.Errorf("missing %q in output:\n%s", want, out)
				}
			}
		})
	}
}

// TestDetectCrossFormat verifies that each format's Detect callback
// claims its own fixture and rejects every other format's fixture.
// This is the regression net against accidental Detect broadening
// when a future format is added.
func TestDetectCrossFormat(t *testing.T) {
	type sample struct {
		ct    string
		bytes string
	}
	samples := []sample{
		{"application/vnd.logfmt",
			`time=2026-01-15T10:23:45Z level=INFO msg="hello" user=alice` + "\n"},
		{"application/vnd.json-log",
			`{"time":"2026-01-15T10:23:45Z","level":"info","msg":"hi"}` + "\n"},
		{"application/vnd.amazon.alb-access-log",
			`http 2026-01-15T12:00:00.000000Z app/my-alb/abc 1.2.3.4:1 5.6.7.8:80 0.001 0.020 0.000 200 200 100 256 "GET / HTTP/1.1" "curl" - - arn "tid" "h" "-" 1 2026-01-15T12:00:00Z "f" "-" "-" "5.6.7.8:80" "200" "-" "-" t1` + "\n"},
		{"application/vnd.nginx-access-log",
			`127.0.0.1 - - [05/Feb/2026:17:11:55 +0000] "GET / HTTP/1.1" 200 612 "-" "curl"` + "\n"},
		{"application/vnd.log4j",
			`[2026-01-15 10:23:45,123] INFO [main] kafka.server.KafkaServer - starting` + "\n"},
		{"application/vnd.python-log",
			`2026-01-15 10:23:45,123 INFO root:42 application started` + "\n"},
		{"application/vnd.go-log",
			`2026/01/15 10:23:45 server starting` + "\n"},
		{"application/vnd.syslog",
			`Jan 15 10:23:45 myhost systemd[1]: started` + "\n"},
		{"application/vnd.syslog-rfc5424",
			`<134>1 2026-01-15T10:23:45.123Z host1 myapp 42 ID01 - hello` + "\n"},
	}
	for _, s := range samples {
		t.Run(strings.TrimPrefix(s.ct, "application/vnd."), func(t *testing.T) {
			got := stripes.Detect("", []byte(s.bytes))
			if got != s.ct {
				t.Errorf("Detect for %s fixture = %q, want %q", s.ct, got, s.ct)
			}
		})
	}
}

// TestContinuationLine asserts that lines starting with whitespace
// are rendered through the dim/comment pass-through path rather
// than reparsed as a new record. Stack traces dumped under a log
// line are the motivating case.
func TestContinuationLine(t *testing.T) {
	input := `time=2026-01-15T10:23:45Z level=ERROR msg="crash" service=api` + "\n" +
		"\tat com.example.Foo.bar(Foo.java:42)\n" +
		"\tat com.example.Baz.qux(Baz.java:88)\n"
	out := renderWith(t, "application/vnd.logfmt", input)
	for _, want := range []string{"ERRO", "crash", "Foo.java:42", "Baz.java:88"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

// TestUnparseableFallback asserts that lines the format's Format
// declines to handle pass through dimmed rather than being dropped.
func TestUnparseableFallback(t *testing.T) {
	// A garbage line under the logfmt renderer: no key=value pairs.
	input := "this is not a log line\n"
	out := renderWith(t, "application/vnd.logfmt", input)
	if !strings.Contains(out, "this is not a log line") {
		t.Errorf("unparseable line dropped:\n%s", out)
	}
}

// TestEmptyInputProducesEmptyOutput keeps the streaming reader
// honest: empty stdin shouldn't allocate spurious lines.
func TestEmptyInputProducesEmptyOutput(t *testing.T) {
	out := renderWith(t, "application/vnd.logfmt", "")
	if out != "" {
		t.Errorf("expected empty output, got %q", out)
	}
}
