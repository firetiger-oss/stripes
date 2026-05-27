package log

import (
	"regexp"

	"github.com/firetiger-oss/stripes"
)

// albAccessFormat renders AWS Application Load Balancer access logs.
// Each entry is a single space-separated line of up to 30 fields
// with quoted substrings for request/user-agent/etc. The documented
// field positions (indexed from 0) the parser extracts:
//
//	 0 type                        14 ssl_cipher
//	 1 time                        15 ssl_protocol
//	 2 elb                         16 target_group_arn
//	 3 client:port                 17 "trace_id" (X-Amzn-Trace-Id)
//	 4 target:port                 18 "domain_name"
//	 5 request_processing_time     19 "chosen_cert_arn"
//	 6 target_processing_time      20 matched_rule_priority
//	 7 response_processing_time    21 request_creation_time
//	 8 elb_status_code             22 "actions_executed"
//	 9 target_status_code          23 "redirect_url"
//	10 received_bytes              24 "error_reason"
//	11 sent_bytes                  25 "target:port_list"
//	12 "request"                   26 "target_status_code_list"
//	13 "user_agent"                27 "classification"
//	                               28 "classification_reason"
//	                               29 conn_trace_id
//
// The header surfaces the at-a-glance triple (status, message,
// size+duration); every other useful field rides along as an
// attribute, skipped per-record when the value is "-" so simple
// records stay short.
//
// Source: AWS docs, "Access logs for your Application Load Balancer".
var albAccessFormat = LineFormat{
	Name:        "alb-access",
	ContentType: "application/vnd.amazon.alb-access-log",
	// No level column — the status code (coloured by class in the
	// metadata cell) already carries severity for HTTP traffic, so
	// a synthesised LEVEL would be redundant noise.
	HasLevel: false,
	Detect:   detectALBAccess,
	Format:   formatALBAccess,
}

var albLeadingRe = regexp.MustCompile(`^(h2|http|https|ws|wss|grpcs|grpc) \d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}`)

func detectALBAccess(peek []byte) bool {
	for _, line := range FirstNonEmptyLines(peek, 2) {
		if albLeadingRe.MatchString(line) {
			return true
		}
	}
	return false
}

func formatALBAccess(line string, styles *stripes.Styles) (Row, bool) {
	fields := parseSpaceSeparatedWithQuotes(line)
	if len(fields) < 13 {
		return Row{}, false
	}
	get := func(i int) string {
		if i < len(fields) {
			return fields[i]
		}
		return ""
	}
	getQ := func(i int) string { return stripQuotes(get(i)) }

	scheme := get(0)
	ts := get(1)
	elbStatus := get(8)
	targetStatus := get(9)
	receivedBytes := get(10)
	sentBytes := get(11)
	req := getQ(12)

	// Attribute keys match the field names AWS documents for the
	// access-log format
	// (docs.aws.amazon.com/elasticloadbalancing/latest/application/load-balancer-access-logs.html)
	// so a reader can paste any attr name straight into the docs
	// or an Athena DDL without translation. Every documented field
	// is surfaced — the only values omitted are AWS's "-"
	// placeholder for fields that don't apply to a given record
	// (no TLS, no error, etc.) so simple records stay terse.
	row := Row{
		Timestamp: ts,
		Level:     statusToLevel(elbStatus),
		Metadata:  styleProtoStatus(scheme, elbStatus, styles),
		Message:   styleRequestLine(req, styles),
		Attrs:     []KV{},
	}
	addOpt := func(key, val string) {
		if val == "" || val == "-" {
			return
		}
		row.Attrs = append(row.Attrs, KV{Key: key, Value: val})
	}
	addOpt("type", scheme)
	addOpt("elb", get(2))
	addOpt("client:port", get(3))
	addOpt("target:port", get(4))
	// Processing times: "-1" is AWS's marker for "this leg didn't
	// complete" (no target response, fixed-response action, etc.);
	// treat that like "-" and skip.
	addOptTime := func(key, val string) {
		if val == "" || val == "-" || val == "-1" {
			return
		}
		row.Attrs = append(row.Attrs, KV{Key: key, Value: val})
	}
	addOptTime("request_processing_time", get(5))
	addOptTime("target_processing_time", get(6))
	addOptTime("response_processing_time", get(7))
	addOpt("elb_status_code", elbStatus)
	addOpt("target_status_code", targetStatus)
	addOpt("received_bytes", receivedBytes)
	addOpt("sent_bytes", sentBytes)
	addOpt("user_agent", getQ(13))
	addOpt("ssl_cipher", get(14))
	addOpt("ssl_protocol", get(15))
	addOpt("target_group_arn", get(16))
	addOpt("trace_id", getQ(17))
	addOpt("domain_name", getQ(18))
	addOpt("chosen_cert_arn", getQ(19))
	addOpt("matched_rule_priority", get(20))
	addOpt("request_creation_time", get(21))
	addOpt("actions_executed", getQ(22))
	addOpt("redirect_url", getQ(23))
	addOpt("error_reason", getQ(24))
	addOpt("target:port_list", getQ(25))
	addOpt("target_status_code_list", getQ(26))
	addOpt("classification", getQ(27))
	addOpt("classification_reason", getQ(28))
	addOpt("conn_trace_id", get(29))
	return row, true
}

// statusToLevel maps an HTTP status code to a coarse severity
// token the shared classifier understands: 5xx=error, 4xx=warn,
// anything else=info.
func statusToLevel(code string) string {
	if code == "" {
		return "info"
	}
	switch code[0] {
	case '5':
		return "error"
	case '4':
		return "warn"
	}
	return "info"
}

// parseSpaceSeparatedWithQuotes splits s on spaces, keeping
// double-quoted substrings as single fields (quotes retained on the
// result so the caller can choose to strip them per-field). Empty
// fields are preserved. Used by ALB and NGINX log parsers.
func parseSpaceSeparatedWithQuotes(s string) []string {
	var out []string
	i := 0
	for i < len(s) {
		// skip leading spaces
		for i < len(s) && s[i] == ' ' {
			i++
		}
		if i >= len(s) {
			break
		}
		if s[i] == '"' {
			j := i + 1
			for j < len(s) {
				if s[j] == '\\' && j+1 < len(s) {
					j += 2
					continue
				}
				if s[j] == '"' {
					j++
					break
				}
				j++
			}
			out = append(out, s[i:j])
			i = j
			continue
		}
		if s[i] == '[' {
			j := i + 1
			for j < len(s) && s[j] != ']' {
				j++
			}
			if j < len(s) {
				j++
			}
			out = append(out, s[i:j])
			i = j
			continue
		}
		j := i
		for j < len(s) && s[j] != ' ' {
			j++
		}
		out = append(out, s[i:j])
		i = j
	}
	return out
}

// stripQuotes drops a single pair of surrounding double quotes from
// s, if present. Returns s unchanged otherwise.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
