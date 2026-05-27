package log

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
)

// resourceGroup is the renderer's internal view of one ResourceLogs:
// the resource attributes promoted to typed fields (service, host…),
// plus a flat list of records across all of its ScopeLogs.
type resourceGroup struct {
	resourceAttrs []*commonv1.KeyValue
	service       string
	namespace     string
	host          string
	records       []logRecord
}

// logRecord folds a *logsv1.LogRecord plus its enclosing
// InstrumentationScope into one struct so the row writer doesn't
// need to keep separate handles.
type logRecord struct {
	pb    *logsv1.LogRecord
	scope *commonv1.InstrumentationScope
	ts    time.Time
}

// renderResourceLogs walks seq and writes one block per resource,
// separated by a blank line. Empty input produces no output.
func renderResourceLogs(w io.Writer, seq iter.Seq[*logsv1.ResourceLogs], opts *Options) error {
	groups := collectResources(seq)
	if len(groups) == 0 {
		return nil
	}
	for i, g := range groups {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if err := renderOneResource(w, g, opts); err != nil {
			return err
		}
	}
	return nil
}

// collectResources flattens seq into one resourceGroup per
// ResourceLogs, preserving input order. Records are emitted in the
// order they appear inside their ScopeLogs.
func collectResources(seq iter.Seq[*logsv1.ResourceLogs]) []*resourceGroup {
	var out []*resourceGroup
	for rl := range seq {
		if rl == nil {
			continue
		}
		g := &resourceGroup{resourceAttrs: rl.GetResource().GetAttributes()}
		g.service, g.namespace, g.host = extractResourceInfo(rl)
		for _, sl := range rl.GetScopeLogs() {
			scope := sl.GetScope()
			for _, lr := range sl.GetLogRecords() {
				if lr == nil {
					continue
				}
				ts := time.Unix(0, int64(lr.GetTimeUnixNano()))
				if lr.GetTimeUnixNano() == 0 {
					ts = time.Unix(0, int64(lr.GetObservedTimeUnixNano()))
				}
				g.records = append(g.records, logRecord{pb: lr, scope: scope, ts: ts})
			}
		}
		if len(g.records) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// extractResourceInfo pulls service.name, service.namespace, and
// host.name from a ResourceLogs' resource attributes. service.name
// falls back to "unknown_service" to match OTel SDK conventions.
func extractResourceInfo(rl *logsv1.ResourceLogs) (service, namespace, host string) {
	for _, kv := range rl.GetResource().GetAttributes() {
		switch kv.GetKey() {
		case "service.name":
			service = kv.GetValue().GetStringValue()
		case "service.namespace":
			namespace = kv.GetValue().GetStringValue()
		case "host.name":
			host = kv.GetValue().GetStringValue()
		}
	}
	if service == "" {
		service = "unknown_service"
	}
	return
}

// resourceHeading returns the right-of-rule heading text for a
// resource block: "service=<svc>[ namespace=<ns>][ host=<h>]".
func (g *resourceGroup) heading() string {
	var b strings.Builder
	b.WriteString("service=")
	b.WriteString(g.service)
	if g.namespace != "" {
		b.WriteString(" namespace=")
		b.WriteString(g.namespace)
	}
	if g.host != "" {
		b.WriteString(" host=")
		b.WriteString(g.host)
	}
	return b.String()
}

// renderOneResource emits the resource's heading rule and one row
// per record. In verbose mode, each row is followed by an indented
// detail block carrying the record's attributes and trace
// correlation.
func renderOneResource(w io.Writer, g *resourceGroup, opts *Options) error {
	width := opts.Width
	if width <= 0 {
		width = 80
	}
	commentStyle := opts.Styles.Comment

	heading := " " + g.heading() + " "
	leadingDashes := 3
	rule := width - ansi.StringWidth(heading) - leadingDashes
	if rule < 2 {
		rule = 2
	}
	headerLine := strings.Repeat("─", leadingDashes) + heading + strings.Repeat("─", rule)
	if _, err := io.WriteString(w, commentStyle.Render(headerLine)+"\n"); err != nil {
		return err
	}

	tsWidth := timestampWidth(g.records)
	for _, r := range g.records {
		// writeRecord folds verbose-mode detail (scope, trace_id,
		// span_id) into the same indented-attrs block under each
		// record, so a separate detail pass isn't needed.
		if err := writeRecord(w, g, r, opts, width, tsWidth); err != nil {
			return err
		}
	}
	return nil
}

// timestampWidth returns the printed width of a single timestamp
// (always 23: "yyyy/mm/dd hh:mm:ss.mmm" in local time, matching the
// canonical format used across every renderer in this package).
// Returns 0 when no record has a timestamp so the column collapses
// for trace-only batches.
func timestampWidth(rs []logRecord) int {
	for _, r := range rs {
		if !r.ts.IsZero() {
			return len("2026/01/15 10:23:45.000")
		}
	}
	return 0
}

// writeRecord emits one record's block:
//
//	yyyy/mm/dd hh:mm:ss.mmm  LEVEL  body
//	  attr.key1      = value
//	  attr.key2      = value
//	  trace_id       = …   (verbose only)
//	  span_id        = …   (verbose only)
//
// Single line when the record has no attributes (and verbose is
// off); the per-record vertical cost is paid only when there's
// structured data to surface. Mirrors the text-log layout in
// text.go so OTLP and text-log records align column-for-column.
func writeRecord(w io.Writer, g *resourceGroup, r logRecord, opts *Options, width, tsWidth int) error {
	// Records are indented one space inside the per-resource block
	// rule (matching the column separator); attributes get a
	// further two-space indent so they read as a nested list.
	gutter := " "

	var tsCol string
	if tsWidth > 0 {
		if r.ts.IsZero() {
			tsCol = opts.Styles.Comment.Render(strings.Repeat(" ", tsWidth))
		} else {
			tsCol = opts.Styles.Comment.Render(formatTime(r.ts))
		}
		tsCol += " "
	}

	class := ClassifyOTLP(r.pb.GetSeverityNumber(), r.pb.GetSeverityText())
	sevCol := class.Style(opts.Styles).Render(class.Label()) + " "

	body := anyValueString(r.pb.GetBody())
	_ = g

	// Body header line + multi-line continuation. Newlines in the
	// body are preserved verbatim — only the per-resource gutter
	// is reapplied to continuation lines so the block stays
	// visually nested under the "─── service=… ───" rule. Wrapper
	// indent is intentionally omitted so the body's own structure
	// (tab-indented stack frames, SQL line breaks, etc.) renders
	// faithfully.
	headerPrefix := gutter + tsCol + sevCol
	if err := writeMultilineStyled(w, headerPrefix, body, opts.Styles.Text, gutter); err != nil {
		return err
	}

	// Indented-attrs block.
	attrs := collectAttrs(r, opts.Verbose)
	if len(attrs) == 0 {
		return nil
	}
	// Pad keys to the per-record max, capped so a single
	// pathologically-long key doesn't push values off-screen.
	const maxKeyWidth = 32
	keyW := 0
	for _, kv := range attrs {
		if w := len(kv.k); w > keyW {
			keyW = w
		}
	}
	if keyW > maxKeyWidth {
		keyW = maxKeyWidth
	}
	keyStyle := opts.Styles.Anchor
	valStyle := opts.Styles.String
	syntax := opts.Styles.Syntax
	// Continuation lines for multi-line values repeat the "│ "
	// at the same column as the first line, so the vertical bar
	// runs uninterrupted from the first value line to the last —
	// the eye reads the whole multi-line value as one bracketed
	// block.
	valueContIndent := strings.Repeat(" ", 3+keyW+1) + syntax.Bold(false).Render("│") + " "
	for _, kv := range attrs {
		linePrefix := "   " + keyStyle.Render(padRight(kv.k, keyW)) + " " + syntax.Bold(false).Render("│") + " "
		if err := writeMultilineStyled(w, linePrefix, kv.v, valStyle, valueContIndent); err != nil {
			return err
		}
	}
	return nil
}

// writeMultilineStyled writes prefix + styled(value) + "\n" with
// continuation lines (when value contains '\n') indented to
// contIndent. Each line is styled independently so SGR escapes
// don't leak across newlines — works on terminals that reset
// styling at line breaks as well as those that don't.
func writeMultilineStyled(w io.Writer, prefix, value string, style lipgloss.Style, contIndent string) error {
	if !strings.ContainsRune(value, '\n') {
		_, err := io.WriteString(w, prefix+style.Render(value)+"\n")
		return err
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		var p string
		if i == 0 {
			p = prefix
		} else {
			p = contIndent
		}
		if _, err := io.WriteString(w, p+style.Render(line)+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// attrPair is the small key/value tuple [collectAttrs] returns —
// avoids allocating an OTLP KeyValue and avoids exposing the
// text-log [KV] type from the OTLP code path.
type attrPair struct{ k, v string }

// collectAttrs returns the sorted attribute set rendered under a
// record. The record's own attributes come first; verbose mode
// adds scope and trace/span correlation. trace/span IDs are always
// promoted out of the inline row (they're high-cardinality
// identifiers, not user attributes) so they only ever appear here.
func collectAttrs(r logRecord, verbose bool) []attrPair {
	out := make([]attrPair, 0, len(r.pb.GetAttributes())+3)
	for _, kv := range r.pb.GetAttributes() {
		// anyValueString returns string values raw (no quoting);
		// nested array/object values still use formatAttrValue
		// internally so their inner strings stay quoted for
		// structural clarity.
		out = append(out, attrPair{k: kv.GetKey(), v: anyValueString(kv.GetValue())})
	}
	if verbose {
		if r.scope != nil && r.scope.GetName() != "" {
			v := r.scope.GetName()
			if ver := r.scope.GetVersion(); ver != "" {
				v += "@" + ver
			}
			out = append(out, attrPair{k: "scope", v: v})
		}
		if id := r.pb.GetTraceId(); len(id) > 0 {
			out = append(out, attrPair{k: "trace_id", v: hexBytes(id)})
		}
		if id := r.pb.GetSpanId(); len(id) > 0 {
			out = append(out, attrPair{k: "span_id", v: hexBytes(id)})
		}
	}
	slices.SortFunc(out, func(a, b attrPair) int { return strings.Compare(a.k, b.k) })
	return out
}

// formatTime returns the yyyy/mm/dd hh:mm:ss.mmm representation
// used in the row timestamp column. Local time, to match the
// canonical layout used across every text log renderer.
func formatTime(t time.Time) string {
	return t.Local().Format("2006/01/02 15:04:05.000")
}

// hexBytes returns the lowercase hex encoding of b. Used for
// trace_id and span_id fields where the proto type is []byte but the
// value is conventionally rendered as hex.
func hexBytes(b []byte) string {
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(b)*2)
	for _, c := range b {
		out = append(out, hexdigits[c>>4], hexdigits[c&0x0f])
	}
	return string(out)
}

// anyValueString renders an OTLP AnyValue as a string. Strings are
// unquoted and preserve embedded newlines (the caller is
// responsible for multi-line layout); non-strings go through
// [formatAttrValue].
func anyValueString(v *commonv1.AnyValue) string {
	if v == nil {
		return ""
	}
	if sv, ok := v.GetValue().(*commonv1.AnyValue_StringValue); ok {
		return sv.StringValue
	}
	return formatAttrValue(v)
}

// formatAttrValue renders an OTLP AnyValue as a compact one-line
// string for verbose output and attribute display.
func formatAttrValue(v *commonv1.AnyValue) string {
	if v == nil {
		return "null"
	}
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return strconv.Quote(x.StringValue)
	case *commonv1.AnyValue_BoolValue:
		return strconv.FormatBool(x.BoolValue)
	case *commonv1.AnyValue_IntValue:
		return strconv.FormatInt(x.IntValue, 10)
	case *commonv1.AnyValue_DoubleValue:
		return strconv.FormatFloat(x.DoubleValue, 'f', -1, 64)
	case *commonv1.AnyValue_BytesValue:
		return fmt.Sprintf("0x%x", x.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		parts := make([]string, 0, len(x.ArrayValue.GetValues()))
		for _, vv := range x.ArrayValue.GetValues() {
			parts = append(parts, formatAttrValue(vv))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *commonv1.AnyValue_KvlistValue:
		parts := make([]string, 0, len(x.KvlistValue.GetValues()))
		for _, kv := range x.KvlistValue.GetValues() {
			parts = append(parts, kv.GetKey()+"="+formatAttrValue(kv.GetValue()))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ""
}

// padRight is defined in row.go (shared with the text-log
// renderer; both live in this package).

// Ensure lipgloss is referenced even when ANSI is off — the import
// is needed by Severity.Style for warn/fatal classes.
var _ = lipgloss.NewStyle
