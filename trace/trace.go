// Package trace registers a waterfall renderer for OpenTelemetry trace
// data with the stripes registry, and exposes a structured Go API for
// callers that already have decoded *tracev1.ResourceSpans.
//
// Import for side effects to enable
// application/vnd.opentelemetry.trace (+protobuf / +json) routing:
//
//	import _ "github.com/firetiger-oss/stripes/trace"
//
// For programmatic use, [Write] / [Format] / [New] take an iter.Seq of
// *tracev1.ResourceSpans. The Fromxxx helpers wrap common OTLP shapes
// (TracesData, raw ResourceSpans, ScopeSpans, Spans) into that
// canonical input.
//
// Output is a per-trace waterfall: one block per trace_id, headed by a
// dense one-line rule with a tick-mark time axis, followed by one line
// per span (service │ tree+kind+name │ duration │ bar). Bars use
// eighth-block sub-cell precision. With [stripes.Styles.Verbose] set,
// each span's attributes and events are revealed in an indented
// detail block.
package trace

import (
	"hash/fnv"
	"io"
	"iter"
	"path"
	"strings"
	"time"

	"github.com/firetiger-oss/stripes"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

// Trace messages this renderer can decode from a byte stream. Anything
// that decodes to one of these is rewrapped to a TracesData internally
// before being fed to the formatter.
const (
	tracesDataFullName    = "opentelemetry.proto.trace.v1.TracesData"
	resourceSpansFullName = "opentelemetry.proto.trace.v1.ResourceSpans"
	scopeSpansFullName    = "opentelemetry.proto.trace.v1.ScopeSpans"
	spanFullName          = "opentelemetry.proto.trace.v1.Span"
)

// IsTraceMessage reports whether fullName is one of the OTLP trace
// message types this package can render. Useful for CLI auto-routing.
func IsTraceMessage(fullName string) bool {
	switch fullName {
	case tracesDataFullName, resourceSpansFullName, scopeSpansFullName, spanFullName:
		return true
	}
	return false
}

func init() {
	stripes.Register(stripes.Format{
		Name:        "trace",
		ContentType: "application/vnd.opentelemetry.trace",
		RendererFor: rendererFor,
	})
}

// rendererFor resolves a byte-stream renderer for
// application/vnd.opentelemetry.trace[+suffix]. The +protobuf suffix
// (or no suffix) picks binary OTLP; +json picks protojson. schemaURL
// is the full message name to decode against; when empty,
// opentelemetry.proto.trace.v1.TracesData is assumed.
func rendererFor(params map[string]string, schemaURL string) stripes.Renderer {
	types := protoregistry.GlobalTypes
	protojsonEncoded := params[stripes.SuffixParam] == "json"
	if schemaURL == "" {
		schemaURL = params["messagetype"]
	}
	if schemaURL == "" {
		schemaURL = tracesDataFullName
	}
	fullName := protoreflect.FullName(path.Base(schemaURL))

	desc := resolveDescriptor(fullName, types)
	if desc == nil {
		// Unknown schema — fall back to TracesData; if even that's not
		// registered we have bigger problems.
		desc = resolveDescriptor(tracesDataFullName, types)
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
	// elements. Nil falls back to [stripes.DefaultStyles]. The trace
	// renderer reads Styles.Width to size the bar column and
	// Styles.Verbose to decide whether to expand span attributes and
	// events.
	Styles *stripes.Styles

	// Verbose, when true, expands each span's attributes, events, and
	// status message under the span's row. Overrides Styles.Verbose
	// when set. Use [WithVerbose] to set it.
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

// Option is a functional option for the trace renderer.
type Option func(*Options)

// WithStyles selects the styles used by the renderer.
func WithStyles(s *stripes.Styles) Option {
	return func(o *Options) { o.Styles = s }
}

// WithVerbose toggles expanded per-span attribute / event output.
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

// Formatter renders OpenTelemetry trace data as a terminal waterfall.
// Construct one with [New] and reuse across calls; each call to
// [Formatter.Render] is independent.
type Formatter struct {
	opts *Options
}

// New returns a Formatter pre-bound to opts. The Formatter is
// independent of any particular input; one Formatter can render many
// traces.
func New(opts ...Option) *Formatter {
	return &Formatter{opts: resolveOptions(opts)}
}

// Render writes the waterfall for every trace in seq to w. Spans are
// grouped by trace_id and emitted in the order their roots first
// appear.
func (f *Formatter) Render(w io.Writer, seq iter.Seq[*tracev1.ResourceSpans]) error {
	return renderResourceSpans(w, seq, f.opts)
}

// Write is a one-shot helper equivalent to New(opts...).Render(w, seq).
func Write(w io.Writer, seq iter.Seq[*tracev1.ResourceSpans], opts ...Option) error {
	return New(opts...).Render(w, seq)
}

// Format is a one-shot helper that returns the rendered waterfall as
// a string. Convenient for tests and assertions.
func Format(seq iter.Seq[*tracev1.ResourceSpans], opts ...Option) string {
	var sb strings.Builder
	_ = New(opts...).Render(&sb, seq)
	return sb.String()
}

// hashString returns a stable 32-bit hash of s. Used for service-color
// assignment so the same service.name renders the same colour across
// runs.
func hashString(s string) uint32 {
	h := fnv.New32a()
	_, _ = io.WriteString(h, s)
	return h.Sum32()
}
