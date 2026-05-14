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
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/all"
	"golang.org/x/term"
)

const usage = `Usage: stripes [flags] [file...]

Pretty-print structured data (JSON, YAML, XML, HTML, CSV, Dockerfile, markdown,
protobuf, parquet, text, source code, txtar, wasm) with ANSI colors and optional paging.

When multiple files are given, each is preceded by a centered rule
(───── filename ─────) so the source is visible inline. --format,
--content-type, and --schema apply to all of them.

Flags:
  -f, --format string         json|yaml|xml|html|csv|dockerfile|markdown|gomod|gosum|gowork|modulestxt|text|code|protobuf|parquet|txtar|wasm|table|auto (default auto)
                              "table" routes CSV/TSV/JSONL/parquet through the
                              new typed-table renderer with width-fitting,
                              JSON-cell colorization, and (for parquet) schema-
                              driven column formatting.
      --content-type string   Override MIME type (e.g. application/vnd.foo+json)
      --schema string         Schema URL (protobuf full name)
      --color string          always|never|auto (default auto)
      --paging string         always|never|auto (default auto). In "auto",
                              the pager is spawned only when the rendered
                              output is wider or taller than the terminal,
                              or when more than one file is rendered.
      --profile string        Color profile name or path. Bare names resolve
                              against $XDG_CONFIG_HOME/stripes/profiles
                              (~/.config/stripes/profiles) and the built-in
                              set. A value containing "/" or ending in
                              .yaml/.yml is loaded as a file directly.
  -w, --width int             Output width in columns. 0 (default) =
                              auto-detect from the terminal; falls back
                              to no wrap when stdout is not a TTY.
  -p, --pager string          Pager command (e.g. "less -R", "bat --plain").
                              Use --paging=never to bypass paging.
  -n, --line-numbers          Show line numbers in a left-aligned gutter.

Pager resolution: -p flag > $PAGER > "less -R"
Profile resolution: --profile flag > $STRIPES_PROFILE > built-in default
Color is auto-disabled when NO_COLOR is set or stdout is not a terminal.
`

type config struct {
	format      string
	contentType string
	schema      string
	color       string
	paging      string
	profile     string
	width       int
	pager       string
	lineNumbers bool
}

func main() {
	cfg, files, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := run(cfg, files); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "stripes:", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (*config, []string, error) {
	cfg := &config{format: "auto", color: "auto", paging: "auto"}

	fs := flag.NewFlagSet("stripes", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	fs.StringVar(&cfg.format, "format", "auto", "input format")
	fs.StringVar(&cfg.format, "f", "auto", "input format (shorthand)")
	fs.StringVar(&cfg.contentType, "content-type", "", "override MIME type")
	fs.StringVar(&cfg.schema, "schema", "", "schema URL (protobuf)")
	fs.StringVar(&cfg.color, "color", "auto", "color mode")
	fs.StringVar(&cfg.paging, "paging", "auto", "paging mode")
	fs.StringVar(&cfg.profile, "profile", "", "color profile name")
	fs.IntVar(&cfg.width, "width", 0, "output width")
	fs.IntVar(&cfg.width, "w", 0, "output width (shorthand)")
	fs.StringVar(&cfg.pager, "pager", "", "pager command")
	fs.StringVar(&cfg.pager, "p", "", "pager command (shorthand)")
	fs.BoolVar(&cfg.lineNumbers, "line-numbers", false, "show line numbers")
	fs.BoolVar(&cfg.lineNumbers, "n", false, "show line numbers (shorthand)")

	if err := fs.Parse(args); err != nil {
		return nil, nil, err
	}

	switch cfg.color {
	case "auto", "always", "never":
	default:
		return nil, nil, fmt.Errorf("invalid --color %q (want auto|always|never)", cfg.color)
	}

	switch cfg.paging {
	case "auto", "always", "never":
	default:
		return nil, nil, fmt.Errorf("invalid --paging %q (want auto|always|never)", cfg.paging)
	}

	switch cfg.format {
	case "auto", "json", "yaml", "xml", "html", "csv", "dockerfile", "markdown", "gomod", "gosum", "gowork", "modulestxt", "text", "code", "protobuf", "wasm", "parquet", "txtar", "table":
	default:
		return nil, nil, fmt.Errorf("invalid --format %q", cfg.format)
	}

	return cfg, fs.Args(), nil
}

func run(cfg *config, files []string) error {
	styles, profile := resolveStyles(cfg)
	rawSink, finish := openSink(cfg, len(files))
	sink := &colorprofile.Writer{Forward: rawSink, Profile: profile}

	if len(files) == 0 {
		renderOne(sink, "", os.Stdin, cfg, styles)
		return finish()
	}

	for i, file := range files {
		f, err := os.Open(file)
		if err != nil {
			_ = finish()
			return err
		}
		if len(files) > 1 {
			if i > 0 {
				io.WriteString(sink, "\n")
			}
			writeSeparator(sink, file, styles)
		}
		renderOne(sink, file, f, cfg, styles)
		_ = f.Close()
	}
	return finish()
}

// renderOne renders a single input into sink. name is the source filename
// (used for content-type detection); empty for stdin.
func renderOne(sink io.Writer, name string, input io.Reader, cfg *config, styles *stripes.Styles) {
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
	}
	if contentType == "" {
		contentType = stripes.Detect(name, peek)
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
	case "wasm":
		return "application/wasm"
	case "parquet":
		return "application/vnd.apache.parquet"
	case "txtar":
		return "text/x-txtar"
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
		return &stripes.Styles{Indent: "  ", Width: width}, colorprofile.NoTTY
	}

	s := loadStyles(cfg)
	s.Width = width
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
