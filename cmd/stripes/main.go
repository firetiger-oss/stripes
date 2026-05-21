// Command stripes pretty-prints structured data with ANSI styling and
// optional paging.
//
// Usage:
//
//	stripes [flags] [file...]
//
// If no file is given, stripes reads from stdin. When multiple files are
// given, they are rendered back to back, separated by a labeled rule. The
// pager is selected by --paging: "auto" (default) spawns a pager only if
// the rendered output is wider or taller than the terminal, or if more than
// one file is being rendered; "always" forces the pager when stdout is a
// terminal; "never" always writes directly to stdout.
package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/all"
	stripescobra "github.com/firetiger-oss/stripes/cobra"
	_ "github.com/firetiger-oss/stripes/protobuf/otlp"
	"github.com/firetiger-oss/stripes/protobuf/schema"
	"github.com/firetiger-oss/stripes/trace"
	basicauth "github.com/firetiger-oss/tigerblock/secret/authn/basic"
	bearerauth "github.com/firetiger-oss/tigerblock/secret/authn/bearer"
	"github.com/firetiger-oss/tigerblock/storage"
	_ "github.com/firetiger-oss/tigerblock/storage/file"
	_ "github.com/firetiger-oss/tigerblock/storage/gs"
	_ "github.com/firetiger-oss/tigerblock/storage/http"
	_ "github.com/firetiger-oss/tigerblock/storage/memory"
	_ "github.com/firetiger-oss/tigerblock/storage/r2"
	_ "github.com/firetiger-oss/tigerblock/storage/s3"
	"github.com/spf13/cobra"
	"golang.org/x/term"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

var version = "dev"

const longDescription = `Pretty-print structured data with ANSI colors and optional paging.

FORMATS

  Structured data: csv, json, parquet, xml, yaml
  Markup:          html, markdown
  Source code:     code, diff, text, txtar
  Build files:     dockerfile, gomod, gosum, gowork, modulestxt
  Binary:          protobuf, wasm
  Observability:   trace (OpenTelemetry waterfall)
  Special:         auto, table

PAGING

  Pager command: --pager flag > $PAGER env > "less -R".
  Mode "auto" spawns the pager only when output exceeds the terminal
  size or when multiple files are rendered; "never" bypasses paging.

AUTHENTICATION

  For http(s):// sources that require auth, set one of:
    --basic-auth user:password
    --bearer-token TOKEN
  If both are set, --basic-auth takes precedence.

PROTOBUF SCHEMAS

  --registry loads a descriptor source; --schema names the message.
  --registry may repeat. Accepted shapes:
    *.binpbset / *.protoset / *.pb     FileDescriptorSet bytes
    *.proto                            compiled in-process (--include adds import roots)
    buf.build/<owner>/<module>[:ref]   fetched via the buf CLI

  HTTP(S) sources serving content-type "application/(x-)protobuf;
  messageType=foo.Bar" auto-resolve the schema from the MIME parameter,
  no --schema needed when the type is already in scope.

`

type config struct {
	format      string
	contentType string
	schema      string
	registries  []string
	includes    []string
	color       string
	paging      string
	profile     string
	width       int
	pager       string
	lineNumbers bool
	verbose     bool
	basicAuth   string
	bearerToken string
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	cfg := &config{}
	root := &cobra.Command{
		Use:     "stripes [flags] [file|uri...]",
		Short:   "Pretty-print structured data with ANSI colors",
		Long:    longDescription,
		Version: version,
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateConfig(cfg); err != nil {
				return err
			}
			return run(cmd.Context(), cfg, args)
		},
	}

	f := root.Flags()
	f.StringVarP(&cfg.format, "format", "f", "auto", "input `format`")
	f.StringVar(&cfg.contentType, "content-type", "", "override MIME `type` (e.g. application/vnd.foo+json)")
	f.StringVar(&cfg.schema, "schema", "", "schema `url` (protobuf full name)")
	f.StringArrayVar(&cfg.registries, "registry", nil, "protobuf schema source (path|uri) — .binpbset/.protoset/.pb or .proto; repeatable")
	f.StringArrayVar(&cfg.includes, "include", nil, "protobuf .proto import-path root (path|uri); repeatable")
	f.StringVar(&cfg.color, "color", "auto", "color `mode` (always|never|auto)")
	f.StringVar(&cfg.paging, "paging", "auto", "paging `mode` (always|never|auto)")
	f.StringVar(&cfg.profile, "profile", "", "color profile `name` or YAML file")
	f.IntVarP(&cfg.width, "width", "w", 0, "output width in `cols` (0 = auto-detect)")
	f.StringVarP(&cfg.pager, "pager", "p", "", "pager `command`")
	f.BoolVarP(&cfg.lineNumbers, "line-numbers", "n", false, "show line numbers")
	f.BoolVarP(&cfg.verbose, "verbose", "v", false, "expand per-row detail (currently used by trace format)")
	f.StringVar(&cfg.basicAuth, "basic-auth", "", "HTTP Basic auth (see Authentication)")
	f.StringVar(&cfg.bearerToken, "bearer-token", "", "HTTP Bearer auth (see Authentication)")

	err := stripescobra.Execute(ctx, root)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func validateConfig(cfg *config) error {
	if cfg.basicAuth != "" && !strings.Contains(cfg.basicAuth, ":") {
		return fmt.Errorf("invalid --basic-auth %q (want user:password)", cfg.basicAuth)
	}
	if len(cfg.registries) > 0 && cfg.schema == "" {
		return fmt.Errorf("--registry requires --schema to select a message")
	}
	switch cfg.color {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("invalid --color %q (want auto|never|always)", cfg.color)
	}
	switch cfg.paging {
	case "auto", "always", "never":
	default:
		return fmt.Errorf("invalid --paging %q (want auto|never|always)", cfg.paging)
	}
	switch cfg.format {
	case "auto":
	case "code":
	case "csv":
	case "diff":
	case "dockerfile":
	case "gomod":
	case "gosum":
	case "gowork":
	case "html":
	case "json":
	case "markdown":
	case "modulestxt":
	case "parquet":
	case "protobuf":
	case "table":
	case "text":
	case "trace":
	case "txtar":
	case "wasm":
	case "xml":
	case "yaml":
	default:
		return fmt.Errorf("invalid --format %q", cfg.format)
	}
	return nil
}

func run(ctx context.Context, cfg *config, files []string) error {
	styles, profile := resolveStyles(cfg)
	rawSink, finish := openSink(cfg, len(files))
	sink := &colorprofile.Writer{Forward: rawSink, Profile: profile}

	if cfg.basicAuth != "" {
		user, pass, _ := strings.Cut(cfg.basicAuth, ":")
		http.DefaultTransport = basicauth.NewTransport(http.DefaultTransport, user, pass)
	} else if cfg.bearerToken != "" {
		http.DefaultTransport = bearerauth.NewTransport(http.DefaultTransport, cfg.bearerToken)
	}

	if len(cfg.registries) > 0 {
		loaded, err := schema.LoadRegistry(ctx, cfg.registries, cfg.includes)
		if err != nil {
			_ = finish()
			return err
		}
		for fd := range loaded.RangeFiles {
			// Skip files already in the process registry — e.g.
			// well-known types pulled in as transitive .proto imports
			// are present from the protobuf runtime and would conflict.
			if _, e := protoregistry.GlobalFiles.FindFileByPath(fd.Path()); e == nil {
				continue
			}
			if e := protoregistry.GlobalFiles.RegisterFile(fd); e != nil {
				_ = finish()
				return fmt.Errorf("register %s: %w", fd.Path(), e)
			}
		}
	}

	if cfg.schema != "" {
		if err := resolveSchema(cfg.schema); err != nil {
			_ = finish()
			return err
		}
	}

	if len(files) == 0 {
		renderOne(sink, "", "", os.Stdin, cfg, styles)
		return finish()
	}

	for i, file := range files {
		if err := func() error {
			rc, info, err := storage.GetObject(ctx, file)
			if err != nil {
				return err
			}
			defer rc.Close()

			r, err := decompress(rc, info.ContentEncoding)
			if err != nil {
				return err
			}
			defer r.Close()

			if len(files) > 1 {
				if i > 0 {
					io.WriteString(sink, "\n")
				}
				writeSeparator(sink, file, styles)
			}
			renderOne(sink, displayName(file), info.ContentType, r, cfg, styles)
			return nil
		}(); err != nil {
			_ = finish()
			return err
		}
	}
	return finish()
}

// resolveSchema verifies that schema (a protobuf full message name)
// resolves against protoregistry.GlobalTypes / GlobalFiles. It returns
// an error if neither registry knows the name, so a typo in --schema
// surfaces before the file payload is decoded as a different shape.
func resolveSchema(name string) error {
	fullName := protoreflect.FullName(name)
	if _, err := protoregistry.GlobalTypes.FindMessageByName(fullName); err == nil {
		return nil
	}
	if desc, err := protoregistry.GlobalFiles.FindDescriptorByName(fullName); err == nil {
		if _, ok := desc.(protoreflect.MessageDescriptor); ok {
			return nil
		}
		return fmt.Errorf("--schema %q resolves to a non-message descriptor", name)
	}
	return fmt.Errorf("--schema %q: message not found (try --registry to load schema sources)", name)
}

// displayName is the name handed to stripes.Detect for content-type sniffing.
// For URIs we strip the scheme/location so the basename and extension drive
// detection (s3://bucket/data.csv → data.csv via filepath.Base). Query strings
// on http(s) URLs are trimmed so .json?token=… still matches .json.
func displayName(arg string) string {
	if i := strings.Index(arg, "://"); i >= 0 {
		_, after, _ := strings.Cut(arg[i+3:], "/")
		if q := strings.IndexByte(after, '?'); q >= 0 {
			after = after[:q]
		}
		return after
	}
	return arg
}

// renderOne renders a single input into sink. name is the source filename
// (used for content-type detection); empty for stdin. contentTypeHint
// carries a server-supplied Content-Type (e.g. from tigerblock storage
// metadata or an HTTP response header) and is consulted between the
// user-supplied --format and the filename/sniff cascade.
func renderOne(sink io.Writer, name, contentTypeHint string, input io.Reader, cfg *config, styles *stripes.Styles) {
	br := bufio.NewReader(input)
	peek, _ := br.Peek(512)

	// --format table short-circuits the content-type dispatch entirely:
	// it routes row-oriented inputs (CSV/TSV/JSONL) through a dedicated
	// renderer that uses the typed table sub-package.
	if cfg.format == "table" {
		renderer := detectRowFlavor(name, peek)
		if cfg.lineNumbers {
			renderer = stripes.WithLineNumbers(renderer)
		}
		tw := &trailingNewlineWriter{w: sink}
		renderer(tw, br, styles)
		tw.flush()
		return
	}

	contentType := cfg.contentType
	if contentType == "" {
		contentType = formatToContentType(cfg.format)
		// Smart -f protobuf: a .json input under explicit -f protobuf
		// is interpreted as protojson rather than binary protobuf.
		if cfg.format == "protobuf" && strings.HasSuffix(strings.ToLower(name), ".json") {
			contentType = "application/protobuf+json"
		}
		if cfg.format == "trace" && strings.HasSuffix(strings.ToLower(name), ".json") {
			contentType = "application/vnd.opentelemetry.trace+json"
		}
	}
	if contentType == "" {
		contentType = contentTypeHint
	}
	if contentType == "" {
		contentType = stripes.Detect(name, peek)
	}
	// Schema-driven auto-routing: when --format=auto and either
	// --schema or the content-type's messageType MIME parameter names
	// an OpenTelemetry trace message, swap a generic protobuf
	// content-type for the dedicated waterfall renderer. Keeps explicit
	// --format=protobuf untouched so users can still get the text view.
	if cfg.format == "auto" {
		contentType = maybeRouteOTLPTrace(contentType, cfg.schema)
	}

	renderer := stripes.Func(contentType, cfg.schema)
	if renderer == nil {
		renderer = stripes.Plain
	}
	if cfg.lineNumbers {
		renderer = stripes.WithLineNumbers(renderer)
	}

	tw := &trailingNewlineWriter{w: sink}
	renderer(tw, br, styles)
	tw.flush()
}

// writeSeparator writes a labeled rule before a rendered file. The label is
// centered between two runs of ─. Rendered through styles.Comment so it's
// faint and doesn't compete with content; with color disabled Comment is the
// zero-value style and emits no escapes.
func writeSeparator(w io.Writer, name string, styles *stripes.Styles) {
	width := styles.Width
	if width <= 0 {
		width = 80
	}
	label := " " + name + " "
	rule := width - ansi.StringWidth(label)
	if rule < 2 {
		rule = 2
	}
	left := rule / 2
	right := rule - left
	line := strings.Repeat("─", left) + label + strings.Repeat("─", right)
	io.WriteString(w, styles.Comment.Render(line))
	io.WriteString(w, "\n")
}

// trailingNewlineWriter ensures the final byte written is a newline.
type trailingNewlineWriter struct {
	w    io.Writer
	last byte
	any  bool
}

func (t *trailingNewlineWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		t.last = p[len(p)-1]
		t.any = true
	}
	return t.w.Write(p)
}

func (t *trailingNewlineWriter) flush() {
	if t.any && t.last != '\n' {
		_, _ = t.w.Write([]byte{'\n'})
	}
}

// maybeRouteOTLPTrace inspects contentType (and optionally a CLI
// --schema) and, when either points at an OpenTelemetry trace message
// carried over a generic protobuf media type, rewrites the
// content-type to application/vnd.opentelemetry.trace[+json] so
// stripes.Func picks the waterfall renderer. The original MIME
// parameters (notably messageType) are preserved on the rewrite so
// the trace renderer can resolve the descriptor.
//
// schema is the explicit --schema flag value, consulted first; the
// content-type's messageType parameter is the fallback signal. When
// neither names a trace type the input contentType is returned
// unchanged.
func maybeRouteOTLPTrace(contentType, schema string) string {
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		mediaType = contentType
		params = nil
	}
	candidate := schema
	if candidate == "" {
		candidate = params["messagetype"]
	}
	if !trace.IsTraceMessage(candidate) {
		return contentType
	}

	var target string
	switch mediaType {
	case "application/protobuf", "application/x-protobuf", "":
		target = "application/vnd.opentelemetry.trace+protobuf"
	case "application/protobuf+json", "application/x-protobuf+json":
		target = "application/vnd.opentelemetry.trace+json"
	default:
		return contentType
	}
	if len(params) == 0 {
		return target
	}
	return mime.FormatMediaType(target, params)
}

func formatToContentType(format string) string {
	switch format {
	case "json":
		return "application/json"
	case "yaml":
		return "application/yaml"
	case "xml":
		return "application/xml"
	case "html":
		return "text/html"
	case "csv":
		return "text/csv"
	case "dockerfile":
		return "text/x-dockerfile"
	case "markdown":
		return "text/markdown"
	case "gomod":
		return "text/x-go-mod"
	case "gosum":
		return "text/x-go-sum"
	case "gowork":
		return "text/x-go-work"
	case "modulestxt":
		return "text/x-go-vendor-modules"
	case "text":
		return "text/plain"
	case "code":
		return "text/x-source-code"
	case "protobuf":
		return "application/protobuf"
	case "trace":
		return "application/vnd.opentelemetry.trace+protobuf"
	case "wasm":
		return "application/wasm"
	case "parquet":
		return "application/vnd.apache.parquet"
	case "txtar":
		return "text/x-txtar"
	case "diff":
		return "text/x-diff"
	}
	return ""
}

// resolveStyles resolves the style set and the color profile to downsample
// rendered output to. lipgloss v2 has no global color profile; callers wrap
// their output writer in a colorprofile.Writer using the returned profile.
func resolveStyles(cfg *config) (*stripes.Styles, colorprofile.Profile) {
	enable := false
	switch cfg.color {
	case "always":
		enable = true
	case "never":
		enable = false
	case "auto":
		enable = os.Getenv("NO_COLOR") == "" && term.IsTerminal(int(os.Stdout.Fd()))
	}

	width := cfg.width
	if width <= 0 {
		if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 0 {
			width = w
		}
	}

	if !enable {
		return &stripes.Styles{Indent: "  ", Width: width, Verbose: cfg.verbose}, colorprofile.NoTTY
	}

	s := loadStyles(cfg)
	s.Width = width
	s.Verbose = cfg.verbose
	return s, colorprofile.TrueColor
}

// loadStyles resolves the color profile selection (flag > env > built-in
// default). Profile-loading errors are reported to stderr and we fall back
// to DefaultStyles so a bad profile never bricks the CLI.
func loadStyles(cfg *config) *stripes.Styles {
	name := cfg.profile
	if name == "" {
		name = os.Getenv("STRIPES_PROFILE")
	}
	if name == "" {
		return stripes.DefaultStyles.Clone()
	}
	prof, err := stripes.LoadProfile(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "stripes: %v; using built-in default\n", err)
		return stripes.DefaultStyles.Clone()
	}
	return prof.ToStyles()
}

// openSink picks the output sink based on the paging mode and how many
// files are being rendered. The returned finish func must be called after
// rendering completes; depending on the mode it flushes a buffer, closes a
// pager pipe and waits, or is a no-op.
func openSink(cfg *config, fileCount int) (io.Writer, func() error) {
	switch determinePagingMode(cfg, fileCount) {
	case "always":
		return openEagerPagerSink(cfg)
	case "auto":
		return openAutoPagerSink(cfg)
	default: // "none"
		return os.Stdout, func() error { return nil }
	}
}

// determinePagingMode returns one of "none", "always", "auto".
//
//   - "never"  → none.
//   - "always" → always (even when stdout is not a TTY; users opt in
//     explicitly).
//   - "auto":
//   - multiple files → always (rule 3).
//   - stdout not a TTY → none (can't measure the terminal).
//   - otherwise → auto, decided lazily by autoPagerSink.
func determinePagingMode(cfg *config, fileCount int) string {
	switch cfg.paging {
	case "never":
		return "none"
	case "always":
		return "always"
	}
	if fileCount > 1 {
		return "always"
	}
	if !term.IsTerminal(int(os.Stdout.Fd())) {
		return "none"
	}
	return "auto"
}

// openEagerPagerSink spawns the pager immediately and returns its stdin
// pipe. On any failure it falls back to stdout with a stderr warning.
func openEagerPagerSink(cfg *config) (io.Writer, func() error) {
	spec := resolvePager(cfg)
	args := strings.Fields(spec)
	if len(args) == 0 {
		return os.Stdout, func() error { return nil }
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	pw, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stripes: pager pipe: %v; writing to stdout\n", err)
		return os.Stdout, func() error { return nil }
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "stripes: pager %q failed to start: %v; writing to stdout\n", spec, err)
		return os.Stdout, func() error { return nil }
	}
	finish := func() error {
		_ = pw.Close()
		return cmd.Wait()
	}
	return pw, finish
}

// openAutoPagerSink returns a lazy sink that buffers output until either
// the rendered width or line count exceeds the terminal, at which point it
// transparently switches to the pager. If the threshold is never reached
// (small output), the buffer is dumped to stdout at finish time.
func openAutoPagerSink(cfg *config) (io.Writer, func() error) {
	w, h, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		w = 80
	}
	if err != nil || h <= 0 {
		h = 24
	}
	s := &autoPagerSink{cfg: cfg, width: w, height: h}
	return s, s.finish
}

// autoPagerSink decides whether to spawn the pager based on the rendered
// content. Writes are accumulated in buf until any rendered line is wider
// than width or the number of completed lines exceeds height. Once that
// happens (or activatePager is invoked for any other reason) the buffer is
// flushed to the pager pipe and subsequent writes pass through directly.
type autoPagerSink struct {
	cfg    *config
	width  int
	height int

	buf   bytes.Buffer // pending bytes while the decision is still open
	line  bytes.Buffer // current incomplete line being measured
	lines int          // completed lines so far

	decided bool      // true once we've committed to a destination
	out     io.Writer // destination after decided == true
	pw      io.WriteCloser
	cmd     *exec.Cmd
}

func (s *autoPagerSink) Write(p []byte) (int, error) {
	if s.decided {
		return s.out.Write(p)
	}
	s.buf.Write(p)
	for _, b := range p {
		s.line.WriteByte(b)
		if b != '\n' {
			continue
		}
		// Trim trailing newline so its zero width doesn't confuse the
		// reader; ansi.StringWidth treats it as 0 anyway, but explicit
		// is friendlier.
		bs := s.line.Bytes()
		if n := len(bs); n > 0 && bs[n-1] == '\n' {
			bs = bs[:n-1]
		}
		if ansi.StringWidth(string(bs)) > s.width {
			s.activatePager()
			return len(p), nil
		}
		s.line.Reset()
		s.lines++
		if s.lines > s.height {
			s.activatePager()
			return len(p), nil
		}
	}
	return len(p), nil
}

// activatePager commits the sink to paging: spawns the pager, replays the
// buffer to it, and points out at the pipe so subsequent writes stream
// through. On any failure it falls back to stdout with the usual warning.
func (s *autoPagerSink) activatePager() {
	spec := resolvePager(s.cfg)
	args := strings.Fields(spec)
	if len(args) == 0 {
		s.fallbackToStdout()
		return
	}

	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	pw, err := cmd.StdinPipe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "stripes: pager pipe: %v; writing to stdout\n", err)
		s.fallbackToStdout()
		return
	}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "stripes: pager %q failed to start: %v; writing to stdout\n", spec, err)
		s.fallbackToStdout()
		return
	}
	if _, err := pw.Write(s.buf.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "stripes: pager write: %v; writing to stdout\n", err)
		_ = pw.Close()
		_ = cmd.Wait()
		s.fallbackToStdout()
		return
	}
	s.buf.Reset()
	s.out = pw
	s.pw = pw
	s.cmd = cmd
	s.decided = true
}

func (s *autoPagerSink) fallbackToStdout() {
	_, _ = os.Stdout.Write(s.buf.Bytes())
	s.buf.Reset()
	s.out = os.Stdout
	s.decided = true
}

func (s *autoPagerSink) finish() error {
	if !s.decided {
		_, err := os.Stdout.Write(s.buf.Bytes())
		s.buf.Reset()
		return err
	}
	if s.pw != nil {
		_ = s.pw.Close()
		return s.cmd.Wait()
	}
	return nil
}

func resolvePager(cfg *config) string {
	return cmp.Or(cfg.pager, os.Getenv("PAGER"), "less -R")
}
