// Package log renders log data with the stripes registry. It covers
// two surfaces in one place:
//
//   - OpenTelemetry log batches (binary or protojson, MIME type
//     application/vnd.opentelemetry.logs). The [Write] / [Format] /
//     [New] helpers take an iter.Seq of *logsv1.ResourceLogs; the
//     Fromxxx adapters wrap common OTLP shapes into that input.
//   - Line-oriented text log formats (logfmt, JSON-per-line, NGINX
//     and ALB access logs, log4j/Kafka, Python logging, Go stdlib
//     log, BSD syslog / journald / macOS, RFC 5424 syslog). Each
//     registers as its own stripes format; [Register] adds more.
//
// Import for side effects to enable detection and rendering of every
// supported format:
//
//	import _ "github.com/firetiger-oss/stripes/log"
//
// Both surfaces share the [SeverityClass] classifier, the
// canonical "yyyy/mm/dd hh:mm:ss.mmm" timestamp shape (local time),
// and the per-record block layout (header + indented attrs sorted
// and aligned on the "=").
package log

import (
	"io"
	"iter"
	"path"
	"strings"
	"time"

	"github.com/firetiger-oss/stripes"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// OTLP log messages this renderer can decode from a byte stream.
// Anything that decodes to one of these is rewrapped to a LogsData
// internally before being fed to the formatter.
const (
	logsDataFullName     = "opentelemetry.proto.logs.v1.LogsData"
	resourceLogsFullName = "opentelemetry.proto.logs.v1.ResourceLogs"
	scopeLogsFullName    = "opentelemetry.proto.logs.v1.ScopeLogs"
	logRecordFullName    = "opentelemetry.proto.logs.v1.LogRecord"
)

// IsLogMessage reports whether fullName is one of the OTLP log
// message types this package can render. Useful for CLI auto-routing.
func IsLogMessage(fullName string) bool {
	switch fullName {
	case logsDataFullName, resourceLogsFullName, scopeLogsFullName, logRecordFullName:
		return true
	}
	return false
}

func init() {
	stripes.Register(stripes.Format{
		Name:        "logs",
		ContentType: "application/vnd.opentelemetry.logs",
		RendererFor: rendererFor,
	})
}

// rendererFor resolves a byte-stream renderer for
// application/vnd.opentelemetry.logs[+suffix]. The +protobuf suffix
// (or no suffix) picks binary OTLP; +json picks protojson. schemaURL
// is the full message name to decode against; when empty,
// opentelemetry.proto.logs.v1.LogsData is assumed.
func rendererFor(params map[string]string, schemaURL string) stripes.Renderer {
	types := protoregistry.GlobalTypes
	protojsonEncoded := params[stripes.SuffixParam] == "json"
	if schemaURL == "" {
		schemaURL = params["messagetype"]
	}
	if schemaURL == "" {
		schemaURL = logsDataFullName
	}
	fullName := protoreflect.FullName(path.Base(schemaURL))

	desc := resolveDescriptor(fullName, types)
	if desc == nil {
		desc = resolveDescriptor(logsDataFullName, types)
	}

	if protojsonEncoded {
		return NewJSONRenderer(desc, types)
	}
	return NewRenderer(desc, types)
}

func resolveDescriptor(fullName protoreflect.FullName, types *protoregistry.Types) protoreflect.MessageDescriptor {
	if mt, err := types.FindMessageByName(fullName); err == nil {
		return mt.Descriptor()
	}
	if d, err := protoregistry.GlobalFiles.FindDescriptorByName(fullName); err == nil {
		if md, ok := d.(protoreflect.MessageDescriptor); ok {
			return md
		}
	}
	return nil
}

// Options control how a [Formatter] renders its input. Use the
// Withxxx helpers to build option values for [New] / [Write] /
// [Format].
type Options struct {
	// Styles selects the lipgloss styles used for labels and rule
	// elements. Nil falls back to [stripes.DefaultStyles]. The logs
	// renderer reads Styles.Width to size the inline-attribute
	// budget and Styles.Verbose to decide whether to expand record
	// attributes and trace correlation.
	Styles *stripes.Styles

	// Verbose, when true, expands each record's attributes, resource
	// keys, and trace/span correlation under the record's row.
	// Overrides Styles.Verbose when set. Use [WithVerbose] to set it.
	Verbose    bool
	verboseSet bool

	// Width is the target output width in columns. 0 falls back to
	// Styles.Width, then to 80.
	Width int

	// Now resolves the wall-clock reference time. Currently used only
	// in tests to keep golden output stable; production callers can
	// leave it nil.
	Now func() time.Time
}

// Option is a functional option for the logs renderer.
type Option func(*Options)

// WithStyles selects the styles used by the renderer.
func WithStyles(s *stripes.Styles) Option {
	return func(o *Options) { o.Styles = s }
}

// WithVerbose toggles expanded per-record attribute / correlation
// output.
func WithVerbose(v bool) Option {
	return func(o *Options) { o.Verbose = v; o.verboseSet = true }
}

// WithWidth overrides the output width in columns. Falls back to
// [stripes.Styles.Width] if zero.
func WithWidth(w int) Option {
	return func(o *Options) { o.Width = w }
}

// WithNow injects a clock for time-dependent formatting. Used in
// tests; production callers can leave it unset.
func WithNow(now func() time.Time) Option {
	return func(o *Options) { o.Now = now }
}

func resolveOptions(opts []Option) *Options {
	o := &Options{}
	for _, fn := range opts {
		fn(o)
	}
	if o.Styles == nil {
		o.Styles = stripes.DefaultStyles
	}
	if !o.verboseSet {
		o.Verbose = o.Styles.Verbose
	}
	if o.Width <= 0 {
		o.Width = o.Styles.Width
	}
	if o.Width <= 0 {
		o.Width = 80
	}
	return o
}

// Formatter renders OpenTelemetry log data as a per-resource table.
// Construct one with [New] and reuse across calls; each call to
// [Formatter.Render] is independent.
type Formatter struct {
	opts *Options
}

// New returns a Formatter pre-bound to opts. The Formatter is
// independent of any particular input; one Formatter can render many
// log batches.
func New(opts ...Option) *Formatter {
	return &Formatter{opts: resolveOptions(opts)}
}

// Render writes the per-resource table for every record in seq to w.
// Records are grouped by resource identity in the order their
// containing ResourceLogs appears.
func (f *Formatter) Render(w io.Writer, seq iter.Seq[*logsv1.ResourceLogs]) error {
	return renderResourceLogs(w, seq, f.opts)
}

// Write is a one-shot helper equivalent to New(opts...).Render(w, seq).
func Write(w io.Writer, seq iter.Seq[*logsv1.ResourceLogs], opts ...Option) error {
	return New(opts...).Render(w, seq)
}

// Format is a one-shot helper that returns the rendered table as a
// string. Convenient for tests and assertions.
func Format(seq iter.Seq[*logsv1.ResourceLogs], opts ...Option) string {
	var sb strings.Builder
	_ = New(opts...).Render(&sb, seq)
	return sb.String()
}
