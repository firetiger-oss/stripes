package txtar

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/code"
	_ "github.com/firetiger-oss/stripes/json"
	_ "github.com/firetiger-oss/stripes/yaml"
	"github.com/muesli/termenv"
)

func TestRenderTxtar(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		output string
	}{
		{
			name: "comment and two files",
			input: `Some leading
comment text.

-- hello.txt --
hello world
-- nums.txt --
one
two
three
`,
			output: `Some leading
comment text.

-- hello.txt --
hello world

-- nums.txt --
one
two
three`,
		},
		{
			name: "no comment",
			input: `-- a.txt --
alpha
-- b.txt --
beta
`,
			output: `-- a.txt --
alpha

-- b.txt --
beta`,
		},
		{
			name: "only comment",
			input: `just a free-form
note, no files at all
`,
			output: `just a free-form
note, no files at all`,
		},
		{
			name:   "empty input",
			input:  ``,
			output: ``,
		},
		{
			name: "empty file body",
			input: `-- empty.txt --
-- next.txt --
content
`,
			output: `-- empty.txt --


-- next.txt --
content`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out strings.Builder
			Txtar(&out, strings.NewReader(tt.input), stripes.DefaultStyles)
			got := trimLineTails(ansi.Strip(out.String()))

			if got != tt.output {
				t.Errorf("Txtar() output mismatch\nInput:\n%s\nExpected:\n%q\nGot:\n%q", tt.input, tt.output, got)
			}
		})
	}
}

// trimLineTails strips trailing whitespace from each line and trailing
// blank lines from the whole string. The Text renderer (used for embedded
// plain-text bodies and for the leading comment) pads each line to align
// with the longest line — that padding is invisible on a terminal but
// shows up in raw-byte comparisons. Normalizing it keeps the test focused
// on the structure Txtar is responsible for.
func trimLineTails(s string) string {
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	out := strings.Join(lines, "\n")
	return strings.TrimRight(out, "\n")
}

func TestTxtarDispatchByFilename(t *testing.T) {
	// Each embedded file should be routed through Detect+Func, so a .go file
	// receives chroma highlighting (visible as ANSI escapes) while a .txt file
	// passes through Plain or Text (no chroma highlighting in the body).
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	input := `-- code.go --
package main

func main() {}
-- note.txt --
plain text body
`
	var out strings.Builder
	Txtar(&out, strings.NewReader(input), stripes.DefaultStyles)
	full := out.String()
	plain := ansi.Strip(full)

	if !strings.Contains(plain, "package main") {
		t.Fatalf("expected Go body in output, got:\n%s", plain)
	}
	if !strings.Contains(plain, "plain text body") {
		t.Fatalf("expected txt body in output, got:\n%s", plain)
	}

	// Look for chroma's keyword-color SGR around "package". Chroma's
	// terminal256 formatter uses 256-color foreground escapes (\x1b[38;5;…m);
	// just confirming any 38;5; sequence inside the .go body shows that the
	// file was routed through Code() rather than Plain.
	pkgIdx := strings.Index(full, "package")
	if pkgIdx < 0 {
		t.Fatalf("expected 'package' in output, got:\n%q", full)
	}
	preamble := full[:pkgIdx]
	if !strings.Contains(preamble, "\x1b[38;5;") {
		t.Errorf("expected chroma 256-color highlighting before 'package', got:\n%q", preamble)
	}
}

func TestTxtarEmbeddedJSON(t *testing.T) {
	// A .json embedded file should run through the JSON renderer, which
	// pretty-prints onto multiple indented lines.
	input := `-- data.json --
{"a":1,"b":2}
`
	var out strings.Builder
	Txtar(&out, strings.NewReader(input), stripes.DefaultStyles)
	got := ansi.Strip(out.String())
	body := got[strings.Index(got, "-- data.json --")+len("-- data.json --"):]
	if !strings.Contains(body, "\n  \"a\"") || !strings.Contains(body, "\n  \"b\"") {
		t.Fatalf("expected JSON-pretty-printed body, got:\n%s", got)
	}
}

func TestLooksLikeTestscript(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"empty", "", false},
		{"prose", "Hello there, this is just\nsome prose.", false},
		{"shebang-like", "#!/bin/sh\nfoo bar", false},
		{"only comments", "# a\n# b\n", false},
		{"plain exec line", "exec ls -la", true},
		{"leading comment then exec", "# setup\nexec stripes input.json\ncmp stdout want.txt", true},
		{"negated command", "! exec false", true},
		{"guard then command", "[short] skip", true},
		{"guard then negated command", "[!exec:foo] ! exec foo", true},
		{"unknown verb", "frobnicate something", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksLikeTestscript(tt.in); got != tt.want {
				t.Errorf("looksLikeTestscript(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestTxtarTestscriptHighlighting(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	input := `# --color=always forces ANSI even when stdout is a pipe.
exec stripes --color=always -p cat input.json
stdout '\x1b\['

-- input.json --
{
  "a": 1
}
`
	var out strings.Builder
	Txtar(&out, strings.NewReader(input), stripes.DefaultStyles)
	full := out.String()
	plain := ansi.Strip(full)

	for _, want := range []string{
		"# --color=always forces ANSI even when stdout is a pipe.",
		"exec stripes --color=always -p cat input.json",
		"stdout '\\x1b\\['",
		"-- input.json --",
	} {
		if !strings.Contains(plain, want) {
			t.Errorf("expected %q in plain output, got:\n%s", want, plain)
		}
	}

	// Each token type should be wrapped in the SGR sequence emitted by the
	// matching style. We don't pin exact escape codes — those depend on the
	// active color profile — only that the right styles were used.
	checks := []struct {
		label string
		token string
		style lipgloss.Style
	}{
		{"comment line", "# --color=always forces ANSI even when stdout is a pipe.", stripes.DefaultStyles.Comment},
		{"command keyword", "exec", stripes.DefaultStyles.Name},
		{"command keyword", "stdout", stripes.DefaultStyles.Name},
		{"flag stays plain", "--color=always", stripes.DefaultStyles.Text},
		{"short flag stays plain", "-p", stripes.DefaultStyles.Text},
		{"quoted string", `'\x1b\['`, stripes.DefaultStyles.String},
	}
	for _, c := range checks {
		styled := c.style.Render(c.token)
		if !strings.Contains(full, styled) {
			t.Errorf("%s: expected styled %q in output", c.label, c.token)
		}
	}
}
func BenchmarkTxtar(b *testing.B) {
	input := []byte(`Leading comment with a sentence or two so the parser has
something to do before reaching the first marker.

-- a.json --
{"a":1,"b":[2,3,4],"c":"hello"}
-- b.txt --
plain prose, several lines
of trivial body content so the
text renderer has work to do.
-- c.go --
package main

import "fmt"

func main() {
	fmt.Println("hello")
}
`)
	r := bytes.NewReader(input)

	b.ReportAllocs()
	b.SetBytes(int64(len(input)))
	for b.Loop() {
		r.Reset(input)
		Txtar(io.Discard, r, stripes.DefaultStyles)
	}
}
