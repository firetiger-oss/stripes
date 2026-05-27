package log

import (
	"regexp"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// nginxAccessFormat renders NGINX (and identical Apache) "combined"
// access log lines:
//
//	$remote_addr - $remote_user [$time_local] "$request" $status
//	$body_bytes_sent "$http_referer" "$http_user_agent"
//
// Reference: nginx.org/en/docs/http/ngx_http_log_module.html
// (default "combined" format).
var nginxAccessFormat = LineFormat{
	Name:        "nginx-access",
	ContentType: "application/vnd.nginx-access-log",
	// No level column — the status code (coloured by class in the
	// metadata cell) already carries severity for HTTP traffic.
	HasLevel: false,
	Detect:   detectNginxAccess,
	Format:   formatNginxAccess,
}

// Loose detector: a remote address, two dashes (or names), a
// bracketed Apache-style date, then a quoted request. Matches both
// NGINX combined and Apache combined.
var nginxLeadingRe = regexp.MustCompile(`^\S+ \S+ \S+ \[\d{2}/[A-Za-z]{3}/\d{4}:\d{2}:\d{2}:\d{2}`)

func detectNginxAccess(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		if nginxLeadingRe.MatchString(line) {
			return true
		}
	}
	return false
}

func formatNginxAccess(line string, styles *stripes.Styles) (Row, bool) {
	fields := parseSpaceSeparatedWithQuotes(line)
	if len(fields) < 6 {
		return Row{}, false
	}
	remote := fields[0]
	user := fields[2]
	tsBracket := fields[3] // "[dd/Mon/YYYY:HH:MM:SS +ZONE]"
	req := stripQuotes(fields[4])
	status := fields[5]
	bytesSent := ""
	if len(fields) >= 7 {
		bytesSent = fields[6]
	}
	referer, ua := "", ""
	if len(fields) >= 8 {
		referer = stripQuotes(fields[7])
	}
	if len(fields) >= 9 {
		ua = stripQuotes(fields[8])
	}

	ts := strings.TrimSuffix(strings.TrimPrefix(tsBracket, "["), "]")

	// nginx combined logs don't carry the request scheme, so the
	// metadata cell always renders the unified HTTP marker. Attribute
	// keys mirror the nginx config variable names (remote_addr,
	// body_bytes_sent, http_referer, http_user_agent…) so they
	// match anything pasted from an nginx log_format directive.
	row := Row{
		Timestamp: ts,
		Level:     statusToLevel(status),
		Metadata:  styleProtoStatus("http", status, styles),
		Message:   styleRequestLine(req, styles),
		Attrs:     []KV{{Key: "remote_addr", Value: remote}},
	}
	if user != "-" {
		row.Attrs = append(row.Attrs, KV{Key: "remote_user", Value: user})
	}
	if bytesSent != "" && bytesSent != "-" {
		row.Attrs = append(row.Attrs, KV{Key: "body_bytes_sent", Value: bytesSent})
	}
	if referer != "" && referer != "-" {
		row.Attrs = append(row.Attrs, KV{Key: "http_referer", Value: referer})
	}
	if ua != "" && ua != "-" {
		row.Attrs = append(row.Attrs, KV{Key: "http_user_agent", Value: ua})
	}
	return row, true
}
