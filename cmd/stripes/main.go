// Command stripes pretty-prints structured data with ANSI styling and
// optional paging.
//
// Usage:
//
//	stripes [flags] [file]
//
// If no file is given, stripes reads from stdin. When stdout is a terminal it
// pipes the styled output through a pager (less -R by default). When stdout
// is not a terminal it writes directly.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/firetiger-oss/stripes"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const usage = `Usage: stripes [flags] [file]

Pretty-print structured data (JSON, YAML, XML, HTML, CSV, Dockerfile, markdown,
protobuf, text, source code, wasm) with ANSI colors and optional paging.

Flags:
  -f, --format string         json|yaml|xml|html|csv|dockerfile|markdown|text|code|protobuf|wasm|auto (default auto)
      --content-type string   Override MIME type (e.g. application/vnd.foo+json)
      --schema string         Schema URL (protobuf full name)
      --color string          always|never|auto (default auto)
      --profile string        Color profile name or path. Bare names resolve
                              against $XDG_CONFIG_HOME/stripes/profiles
                              (~/.config/stripes/profiles) and the built-in
                              set. A value containing "/" or ending in
                              .yaml/.yml is loaded as a file directly.
  -w, --width int             Output width in columns. 0 (default) =
                              auto-detect from the terminal; falls back
                              to no wrap when stdout is not a TTY.
  -p, --pager string          Pager command (e.g. "less -R", "bat --plain").
                              Use "cat" to bypass paging on a TTY.

Pager resolution: -p flag > $STRIPES_PAGER > $PAGER > "less -R"
Profile resolution: --profile flag > $STRIPES_PROFILE > built-in default
Color is auto-disabled when NO_COLOR is set or stdout is not a terminal.
`

type config struct {
	format      string
	contentType string
	schema      string
	color       string
	profile     string
	width       int
	pager       string
}

func main() {
	cfg, file, err := parseFlags(os.Args[1:])
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	if err := run(cfg, file); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintln(os.Stderr, "stripes:", err)
		os.Exit(1)
	}
}

func parseFlags(args []string) (*config, string, error) {
	cfg := &config{format: "auto", color: "auto"}

	fs := flag.NewFlagSet("stripes", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.Usage = func() { fmt.Fprint(os.Stderr, usage) }

	fs.StringVar(&cfg.format, "format", "auto", "input format")
	fs.StringVar(&cfg.format, "f", "auto", "input format (shorthand)")
	fs.StringVar(&cfg.contentType, "content-type", "", "override MIME type")
	fs.StringVar(&cfg.schema, "schema", "", "schema URL (protobuf)")
	fs.StringVar(&cfg.color, "color", "auto", "color mode")
	fs.StringVar(&cfg.profile, "profile", "", "color profile name")
	fs.IntVar(&cfg.width, "width", 0, "output width")
	fs.IntVar(&cfg.width, "w", 0, "output width (shorthand)")
	fs.StringVar(&cfg.pager, "pager", "", "pager command")
	fs.StringVar(&cfg.pager, "p", "", "pager command (shorthand)")

	if err := fs.Parse(args); err != nil {
		return nil, "", err
	}

	switch cfg.color {
	case "auto", "always", "never":
	default:
		return nil, "", fmt.Errorf("invalid --color %q (want auto|always|never)", cfg.color)
	}

	switch cfg.format {
	case "auto", "json", "yaml", "xml", "html", "csv", "dockerfile", "markdown", "text", "code", "protobuf", "wasm":
	default:
		return nil, "", fmt.Errorf("invalid --format %q", cfg.format)
	}

	if fs.NArg() > 1 {
		return nil, "", fmt.Errorf("too many positional arguments (got %d, want 0 or 1)", fs.NArg())
	}

	file := ""
	if fs.NArg() == 1 {
		file = fs.Arg(0)
	}
	return cfg, file, nil
}

func run(cfg *config, file string) error {
	var input io.ReadCloser
	var name string

	if file != "" {
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		input = f
		name = file
	} else {
		input = os.Stdin
	}
	defer input.Close()

	br := bufio.NewReader(input)
	peek, _ := br.Peek(512)

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

	styles := resolveStyles(cfg)

	sink, finish := openSink(cfg)
	tw := &trailingNewlineWriter{w: sink}
	renderer(tw, br, styles)
	tw.flush()
	return finish()
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
	case "text":
		return "text/plain"
	case "code":
		return "text/x-source-code"
	case "protobuf":
		return "application/protobuf"
	case "wasm":
		return "application/wasm"
	}
	return ""
}

func resolveStyles(cfg *config) *stripes.Styles {
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
		lipgloss.SetColorProfile(termenv.Ascii)
		return &stripes.Styles{Indent: "  ", Width: width}
	}

	lipgloss.SetColorProfile(termenv.TrueColor)
	s := loadStyles(cfg)
	s.Width = width
	return s
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

// openSink picks the output sink. If paging is active, it spawns the pager
// and returns its stdin pipe; otherwise returns os.Stdout directly. The
// returned finish func must be called after rendering completes; it closes
// the pipe and waits for the pager.
func openSink(cfg *config) (io.Writer, func() error) {
	if !pagingActive() {
		return os.Stdout, func() error { return nil }
	}

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

// pagingActive returns true when the pager should be spawned. A test-only
// override (STRIPES_FORCE_PAGER) is honored so non-PTY testscript scenarios
// can exercise the pager path.
func pagingActive() bool {
	if os.Getenv("STRIPES_FORCE_PAGER") == "1" {
		return true
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func resolvePager(cfg *config) string {
	if cfg.pager != "" {
		return cfg.pager
	}
	if v := os.Getenv("STRIPES_PAGER"); v != "" {
		return v
	}
	if v := os.Getenv("PAGER"); v != "" {
		return v
	}
	return "less -R"
}
