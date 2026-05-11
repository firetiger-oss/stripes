// Command stripes pretty-prints structured data with ANSI styling and
// optional paging.
//
// Usage:
//
//	stripes [flags] [file...]
//
// If no file is given, stripes reads from stdin. When multiple files are
// given, they are rendered back to back, separated by a labeled rule. When
// stdout is a terminal it pipes the styled output through a pager (less -R
// by default). When stdout is not a terminal it writes directly.
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
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const usage = `Usage: stripes [flags] [file...]

Pretty-print structured data (JSON, YAML, XML, HTML, CSV, Dockerfile, markdown,
protobuf, text, source code, wasm) with ANSI colors and optional paging.

When multiple files are given, each is preceded by a centered rule
(───── filename ─────) so the source is visible inline. --format,
--content-type, and --schema apply to all of them.

Flags:
  -f, --format string         json|yaml|xml|html|csv|dockerfile|markdown|text|code|protobuf|wasm|table|auto (default auto)
                              "table" routes CSV/TSV/JSONL through the
                              new typed-table renderer with width-fitting
                              and JSON-cell colorization.
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
  -n, --line-numbers          Show line numbers in a left-aligned gutter.

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

	switch cfg.format {
	case "auto", "json", "yaml", "xml", "html", "csv", "dockerfile", "markdown", "text", "code", "protobuf", "wasm", "table":
	default:
		return nil, nil, fmt.Errorf("invalid --format %q", cfg.format)
	}

	return cfg, fs.Args(), nil
}

func run(cfg *config, files []string) error {
	styles := resolveStyles(cfg)
	sink, finish := openSink(cfg)

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
// faint and doesn't compete with content; under termenv.Ascii Comment is a
// no-op.
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
