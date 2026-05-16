package markdown

import (
	"bytes"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRender(t *testing.T) {
	hr80 := strings.Repeat("─", 80)
	tests := []struct {
		name   string
		input  string
		output string
		// bare renders with an unstyled Styles{} (color off) instead of
		// DefaultStyles. Used by cases that exercise the no-color path,
		// e.g. links degrade to "text (url)" rather than OSC 8 hyperlinks.
		bare bool
	}{
		{
			name:   "heading h1",
			input:  "# Hello",
			output: "HELLO\n─────",
		},
		{
			name:   "heading h2 + paragraph",
			input:  "## Subheading\n\nA paragraph here.",
			output: "Subheading\n\nA paragraph here.",
		},
		{
			name:   "heading h3-h6 indent ladder",
			input:  "### h3\n#### h4\n##### h5\n###### h6",
			output: "h3\n\n  h4\n\n    h5\n\n      h6",
		},
		{
			name:   "emphasis strong strikethrough code span strip markers",
			input:  "This is **bold** and *italic* and ~~strike~~ and `code`.",
			output: "This is bold and italic and strike and code.",
		},
		{
			name:   "fenced code block with language",
			input:  "```go\nfmt.Println(\"hi\")\n```",
			output: "  fmt.Println(\"hi\")",
		},
		{
			name:   "fenced code block without language",
			input:  "```\nplain code\n```",
			output: "  plain code",
		},
		{
			name:   "indented code block",
			input:  "    indented code\n",
			output: "  indented code",
		},
		{
			name:   "unordered list",
			input:  "- one\n- two\n- three",
			output: "• one\n• two\n• three",
		},
		{
			name:   "ordered list",
			input:  "1. first\n2. second",
			output: "1. first\n2. second",
		},
		{
			name:   "nested list",
			input:  "- outer\n  - inner1\n  - inner2\n- next",
			output: "• outer\n  • inner1\n  • inner2\n• next",
		},
		{
			name:   "blockquote",
			input:  "> quoted\n> line two",
			output: "│ quoted line two",
		},
		{
			name:   "link with distinct text",
			input:  "[Anthropic](https://anthropic.com)",
			output: "Anthropic (https://anthropic.com)",
			bare:   true,
		},
		{
			name:   "link text equals url collapses",
			input:  "[https://x.com](https://x.com)",
			output: "https://x.com",
		},
		{
			name:   "autolink",
			input:  "<https://x.com>",
			output: "https://x.com",
		},
		{
			name:   "image",
			input:  "![alt text](image.png)",
			output: "[image] alt text (image.png)",
		},
		{
			name:   "image without alt",
			input:  "![](pic.jpg)",
			output: "[image] (pic.jpg)",
		},
		{
			name:   "thematic break",
			input:  "---",
			output: hr80,
		},
		{
			name:   "task list",
			input:  "- [x] done\n- [ ] todo",
			output: "✓ done\n☐ todo",
		},
		{
			name:   "raw html inline dropped",
			input:  "Para with <span>html</span> tags.",
			output: "Para with html tags.",
		},
		{
			name:   "raw html block dropped",
			input:  "before\n\n<div>html</div>\n\nafter",
			output: "before\n\nafter",
		},
		{
			name:   "hard break",
			input:  "line one  \nline two",
			output: "line one\nline two",
		},
		{
			name:   "soft break joins with space",
			input:  "first\nsecond",
			output: "first second",
		},
		{
			name:   "mixed document",
			input:  "# Title\n\nIntro **bold** here.\n\n## Sub\n\n- a\n- b\n\n```go\nx := 1\n```\n",
			output: "TITLE\n─────\n\nIntro bold here.\n\nSub\n\n• a\n• b\n\n  x := 1",
		},
		{
			name:   "ordered list paragraph wraps with continuation indent",
			input:  "1. first item that is a bit long maybe\n2. second item",
			output: "1. first item that is a bit\n   long maybe\n2. second item",
		},
		{
			name:   "unordered list paragraph wraps within marker indent",
			input:  "- this is a long bullet point that should wrap nicely when narrow",
			output: "• this is a long bullet point\n  that should wrap nicely when\n  narrow",
		},
		{
			name:   "blockquote paragraph wraps within quote indent",
			input:  "> aaaa bbbb cccc dddd eeee ffff gggg",
			output: "│ aaaa bbbb cccc dddd eeee\n│ ffff gggg",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			styles := stripes.DefaultStyles.Clone()
			if tt.bare {
				styles = &stripes.Styles{}
			}
			// Force a small width for list-wrap tests deterministically.
			switch tt.name {
			case "ordered list paragraph wraps with continuation indent",
				"unordered list paragraph wraps within marker indent",
				"blockquote paragraph wraps within quote indent":
				styles.Width = 30
			default:
				styles.Width = 80
			}
			Render(&output, strings.NewReader(tt.input), styles)
			stripped := ansi.Strip(output.String())
			if stripped != tt.output {
				t.Errorf("Render() output mismatch\nInput:    %q\nExpected: %q\nGot:      %q",
					tt.input, tt.output, stripped)
			}
		})
	}
}

func TestMarkdownEmptyDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render() panicked on empty input: %v", r)
		}
	}()
	var buf strings.Builder
	Render(&buf, strings.NewReader(""), stripes.DefaultStyles)
	if got := ansi.Strip(buf.String()); got != "" {
		t.Errorf("expected empty output, got %q", got)
	}
}

func TestMarkdownStripsFrontmatter(t *testing.T) {
	input := "---\nname: rollout\ndescription: \"SRE-grade change control\"\n---\n\n# Rollout\n\nUmbrella skill body."
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	got := ansi.Strip(buf.String())
	for _, banned := range []string{"name:", "description:", "rollout\n", "SRE-grade"} {
		if strings.Contains(got, banned) {
			t.Errorf("frontmatter leaked into output: %q\nGot:\n%s", banned, got)
		}
	}
	if !strings.HasPrefix(got, "ROLLOUT\n") {
		t.Errorf("expected output to start with rendered H1 heading, got:\n%s", got)
	}
	if !strings.Contains(got, "Umbrella skill body.") {
		t.Errorf("body text missing\nGot:\n%s", got)
	}
}

func TestMarkdownPreservesThematicBreakNotFrontmatter(t *testing.T) {
	input := "# Title\n\nFirst paragraph.\n\n---\n\nSecond paragraph."
	var buf strings.Builder
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 40
	Render(&buf, strings.NewReader(input), styles)
	got := ansi.Strip(buf.String())
	if !strings.Contains(got, strings.Repeat("─", 40)) {
		t.Errorf("expected thematic break (40-char rule) in output\nGot:\n%s", got)
	}
}

func TestMarkdownLeavesUnclosedFrontmatterAlone(t *testing.T) {
	// Input starts with --- but never closes the fence; we must not strip
	// it (the file is not really frontmatter — keep current rendering).
	input := "---\nsome: yaml\nbut no closing fence\n\n# Heading\n"
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	got := ansi.Strip(buf.String())
	if !strings.Contains(got, "some") {
		t.Errorf("unclosed frontmatter should pass through unchanged\nGot:\n%s", got)
	}
}

func TestMarkdownParagraphWrap(t *testing.T) {
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 20
	var buf strings.Builder
	Render(&buf, strings.NewReader("this is a long paragraph that should wrap when the width is small enough"), styles)
	got := ansi.Strip(buf.String())
	want := "this is a long\nparagraph that\nshould wrap when the\nwidth is small\nenough"
	if got != want {
		t.Errorf("wrap mismatch:\nwant: %q\ngot:  %q", want, got)
	}
}

func TestMarkdownTable(t *testing.T) {
	input := "| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |"
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), stripes.DefaultStyles)
	got := ansi.Strip(buf.String())
	for _, want := range []string{"a", "b", "1", "2", "3", "4", "─", "│"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q\nGot:\n%s", want, got)
		}
	}
}

func TestMarkdownTableWraps(t *testing.T) {
	input := "| name | description |\n|------|-------------|\n" +
		"| alpha | this is a fairly long description that should wrap when the table is narrow |\n" +
		"| beta  | short |"
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 40
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), styles)
	got := ansi.Strip(buf.String())

	for _, want := range []string{"name", "description", "alpha", "beta", "fairly", "wrap"} {
		if !strings.Contains(got, want) {
			t.Errorf("table output missing %q\nGot:\n%s", want, got)
		}
	}
	for _, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w > styles.Width {
			t.Errorf("line exceeds width %d (got %d): %q", styles.Width, w, line)
		}
	}
}

func TestMarkdownTableNoStretch(t *testing.T) {
	input := "| a | b |\n|---|---|\n| 1 | 2 |"
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 80
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), styles)
	got := ansi.Strip(buf.String())
	for _, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w >= styles.Width {
			t.Errorf("small table was stretched to width %d: %q", w, line)
		}
	}
}

func TestMarkdownTableProportionalWidths(t *testing.T) {
	// Short labels + long prose at narrow width: Description column should
	// get noticeably more horizontal space than Quarter, words should stay
	// intact (no mid-word breaks since the longest token "onboarding" is
	// shorter than the share Description gets).
	input := "| Quarter | Description |\n|---|---|\n" +
		"| Q1 | An overview of customer onboarding metrics across various product lines |\n" +
		"| Q2 | Detailed breakdown of churn and reasons reported by customers |"
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 60
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), styles)
	got := ansi.Strip(buf.String())

	// No line should exceed the configured width.
	for _, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w > styles.Width {
			t.Errorf("line exceeds width %d (got %d): %q", styles.Width, w, line)
		}
	}
	// Whole-word tokens must remain intact (no mid-word breaks for the
	// longest natural-prose word in the input).
	for _, want := range []string{"onboarding", "Detailed", "Quarter", "Description", "Q1", "Q2"} {
		if !strings.Contains(got, want) {
			t.Errorf("word %q got broken across lines\nGot:\n%s", want, got)
		}
	}
}

func TestMarkdownTableLongTokenFallback(t *testing.T) {
	// A column made of an 80-char hostname can't honor word-min protection
	// in a 60-col terminal — must fall back to proportional + floor and
	// produce a table that fits the requested width.
	input := "| name  | host |\n|-------|------|\n" +
		"| alpha | ft-20260303191446144900000029.cluster-cnky4cc4sk5k.us-west-2.rds.amazonaws.com |\n" +
		"| beta  | shorthost |"
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 50
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), styles)
	got := ansi.Strip(buf.String())

	for _, line := range strings.Split(got, "\n") {
		if w := ansi.StringWidth(line); w > styles.Width {
			t.Errorf("line exceeds width %d (got %d): %q", styles.Width, w, line)
		}
	}
	// All column contents present.
	for _, want := range []string{"alpha", "beta", "shorthost", "20260303"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q\nGot:\n%s", want, got)
		}
	}
}

func TestMarkdownCodeBlockChromaColor(t *testing.T) {
	styles := stripes.DefaultStyles.Clone()
	var buf strings.Builder
	Render(&buf, strings.NewReader("```go\npackage main\n\nfunc main() {}\n```\n"), styles)
	out := buf.String()
	if !strings.Contains(out, "\x1b[") {
		t.Errorf("expected ANSI escapes from chroma, got: %q", out)
	}
	stripped := ansi.Strip(out)
	for _, want := range []string{"package", "main", "func"} {
		if !strings.Contains(stripped, want) {
			t.Errorf("stripped output missing %q\nGot:\n%s", want, stripped)
		}
	}
	// Each non-empty code line should start with the 2-space indent.
	for _, line := range strings.Split(strings.Trim(stripped, "\n"), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "  ") {
			t.Errorf("code line not indented: %q", line)
		}
	}
}

func TestMarkdownNoColorPath(t *testing.T) {
	// Bare Styles (no lipgloss colors set) → chroma path should be skipped /
	// stripped, fallback indents per line.
	styles := &stripes.Styles{Indent: "  ", Width: 80}
	var buf strings.Builder
	Render(&buf, strings.NewReader("```go\nfmt.Println(\"x\")\n```\n"), styles)
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escapes, got: %q", out)
	}
	if !strings.Contains(out, "  fmt.Println") {
		t.Errorf("expected indented code, got: %q", out)
	}
}

func TestMarkdownLinkOSC8(t *testing.T) {
	styles := stripes.DefaultStyles.Clone()
	var buf strings.Builder
	Render(&buf, strings.NewReader("Pick a [Renderer](https://example.com/r) here."), styles)
	out := buf.String()

	// OSC 8 prelude with destination URL must be present.
	if !strings.Contains(out, "\x1b]8;;https://example.com/r\x1b\\") {
		t.Errorf("expected OSC 8 hyperlink open, got: %q", out)
	}
	if !strings.Contains(out, "\x1b]8;;\x1b\\") {
		t.Errorf("expected OSC 8 hyperlink close, got: %q", out)
	}
	if !strings.Contains(out, "\x1b[4:5m") {
		t.Errorf("expected dashed underline SGR, got: %q", out)
	}
	stripped := ansi.Strip(out)
	if want := "Pick a Renderer here."; stripped != want {
		t.Errorf("stripped output: want %q, got %q", want, stripped)
	}
}

func TestMarkdownAutolinkOSC8(t *testing.T) {
	styles := stripes.DefaultStyles.Clone()
	var buf strings.Builder
	Render(&buf, strings.NewReader("see <https://example.com>"), styles)
	out := buf.String()
	if !strings.Contains(out, "\x1b]8;;https://example.com\x1b\\") {
		t.Errorf("expected OSC 8 wrapper, got: %q", out)
	}
	if want := "see https://example.com"; ansi.Strip(out) != want {
		t.Errorf("stripped: want %q, got %q", want, ansi.Strip(out))
	}
}

func TestMarkdownLinkWrapAccountsForOSC8Width(t *testing.T) {
	// The OSC 8 wrapper inserts URL bytes that must NOT count toward the
	// visible width when wrapping a paragraph.
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 80
	input := "Pass stripes.DefaultStyles for the [built-in](https://example.com/very/long/url) grayscale theme, a Clone() to customize, or &stripes.Styles{} for unstyled output."
	var buf strings.Builder
	Render(&buf, strings.NewReader(input), styles)
	want := "Pass stripes.DefaultStyles for the built-in grayscale theme, a Clone() to\ncustomize, or &stripes.Styles{} for unstyled output."
	if got := ansi.Strip(buf.String()); got != want {
		t.Errorf("wrap mismatch:\nwant: %q\n got: %q", want, got)
	}
}

// chunkReader feeds src to Read in fixed-size chunks, returning io.EOF on
// the read that exhausts src. Used to simulate slow-arriving input.
type chunkReader struct {
	src  []byte
	size int
	off  int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	if c.off >= len(c.src) {
		return 0, io.EOF
	}
	n := c.size
	if n <= 0 || n > len(p) {
		n = len(p)
	}
	if n > len(c.src)-c.off {
		n = len(c.src) - c.off
	}
	copy(p, c.src[c.off:c.off+n])
	c.off += n
	return n, nil
}

func TestRenderStreamingMatchesBatch(t *testing.T) {
	cases := []struct {
		name  string
		input string
		width int
	}{
		{"empty", "", 80},
		{"heading h1", "# Hello", 80},
		{"heading + paragraph", "# Title\n\nFirst paragraph here.\n", 80},
		{"mixed document", "# Title\n\nIntro **bold** here.\n\n## Sub\n\n- a\n- b\n\n```go\nx := 1\n```\n", 80},
		{"nested list", "- outer\n  - inner1\n  - inner2\n- next", 80},
		{"blockquote", "> quoted\n> line two\n\nafter", 80},
		{"table", "| a | b |\n|---|---|\n| 1 | 2 |\n| 3 | 4 |\n", 80},
		{"thematic break between paragraphs", "First\n\n---\n\nSecond", 40},
		{"frontmatter", "---\nname: foo\n---\n\n# Body\n\npara", 80},
		{"unclosed frontmatter", "---\nsome: yaml\nno close\n\n# Heading\n", 80},
		{"setext h1", "Title\n=====\n\nBody.", 80},
		{"setext h2 hazard", "First paragraph.\n\nSome text\n---\n\nNext", 80},
		{"code fence", "```go\nfmt.Println(\"hi\")\n```\n\nafter", 80},
		{"narrow paragraph wrap", "this is a long paragraph that should wrap when the width is small enough", 20},
		{"image", "![alt text](image.png)\n\nafter", 80},
		{"task list", "- [x] done\n- [ ] todo\n\nafter", 80},
		{"hard break", "line one  \nline two\n\nafter", 80},
		{"trailing partial line", "# Heading", 80}, // no terminating newline
	}

	chunkSizes := []int{1, 2, 3, 7, 64, 4096}

	for _, tc := range cases {
		for _, sz := range chunkSizes {
			name := tc.name + "/chunk=" + itoa(sz)
			t.Run(name, func(t *testing.T) {
				batchStyles := stripes.DefaultStyles.Clone()
				batchStyles.Width = tc.width
				streamStyles := stripes.DefaultStyles.Clone()
				streamStyles.Width = tc.width

				var batch strings.Builder
				Render(&batch, strings.NewReader(tc.input), batchStyles)

				var stream strings.Builder
				Render(&stream, &chunkReader{src: []byte(tc.input), size: sz}, streamStyles)

				if batch.String() != stream.String() {
					t.Fatalf("streamed output differs from batch\nchunk size: %d\ninput: %q\nbatch:\n%s\n\nstream:\n%s",
						sz, tc.input, batch.String(), stream.String())
				}
			})
		}
	}
}

// itoa is a tiny replacement for strconv.Itoa to avoid an import just for tests.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	pos := len(b)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		pos--
		b[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		b[pos] = '-'
	}
	return string(b[pos:])
}

// concurrentBuffer is a goroutine-safe bytes.Buffer for the io.Pipe-driven
// liveness tests.
type concurrentBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (c *concurrentBuffer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.Write(p)
}

func (c *concurrentBuffer) String() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.buf.String()
}

// waitForContains polls buf for substring until found or the deadline
// elapses; returns the final buffer contents and whether substring was seen.
func waitForContains(buf *concurrentBuffer, substr string, deadline time.Duration) (string, bool) {
	end := time.Now().Add(deadline)
	for {
		if got := buf.String(); strings.Contains(ansi.Strip(got), substr) {
			return got, true
		}
		if time.Now().After(end) {
			return buf.String(), false
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestRenderStreamingFastPathHeading(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	var out concurrentBuffer
	done := make(chan struct{})
	go func() {
		Render(&out, pr, stripes.DefaultStyles)
		close(done)
	}()

	if _, err := pw.Write([]byte("# Title\n")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	// H1 renders the title in uppercase ("TITLE") followed by a rule line.
	// The heading must appear before EOF — assert it shows up promptly.
	if got, ok := waitForContains(&out, "TITLE", 2*time.Second); !ok {
		t.Fatalf("expected H1 to render before EOF; got: %q", got)
	}
	pw.Close()
	<-done
}

func TestRenderStreamingParagraphWaitsForSuccessor(t *testing.T) {
	// A trailing paragraph must NOT render before either EOF or a successor
	// block — otherwise a later `===` or `---` line would have it
	// retroactively become a setext heading, which the stream cannot undo.
	pr, pw := io.Pipe()
	defer pw.Close()
	var out concurrentBuffer
	done := make(chan struct{})
	go func() {
		Render(&out, pr, stripes.DefaultStyles)
		close(done)
	}()

	if _, err := pw.Write([]byte("Just a paragraph.\n")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	// Give the renderer a window to (incorrectly) emit anything.
	time.Sleep(50 * time.Millisecond)
	if got := ansi.Strip(out.String()); got != "" {
		t.Fatalf("trailing paragraph leaked before successor/EOF: %q", got)
	}
	pw.Close()
	<-done
	if got := ansi.Strip(out.String()); got != "Just a paragraph." {
		t.Errorf("expected flushed paragraph; got: %q", got)
	}
}

func TestRenderStreamingSetextHazardLateUnderline(t *testing.T) {
	// "Some text" arrives first; later "---" arrives. The renderer must not
	// have committed "Some text" as a paragraph — the final output is a
	// setext H2, not paragraph + thematic break.
	pr, pw := io.Pipe()
	defer pw.Close()
	var out concurrentBuffer
	done := make(chan struct{})
	go func() {
		Render(&out, pr, stripes.DefaultStyles)
		close(done)
	}()

	if _, err := pw.Write([]byte("Some text\n")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	time.Sleep(20 * time.Millisecond)
	if _, err := pw.Write([]byte("---\n")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	pw.Close()
	<-done

	// Setext H2 renders with the H2 underline pathway. Verify we did not
	// produce a thematic-break rule (long row of ─) on its own line after
	// "Some text" — i.e. the `---` was consumed by the setext heading, not
	// turned into an HR.
	stripped := ansi.Strip(out.String())
	if !strings.Contains(stripped, "Some text") {
		t.Fatalf("expected heading text in output, got: %q", stripped)
	}
	if strings.Contains(stripped, "\n"+strings.Repeat("─", 80)) {
		t.Errorf("setext underline rendered as thematic break; output: %q", stripped)
	}
}

func TestRenderStreamingThematicBreakFastPath(t *testing.T) {
	// A thematic break following a blank line should fast-path even without
	// a successor block: it cannot be reinterpreted by later input.
	pr, pw := io.Pipe()
	defer pw.Close()
	var out concurrentBuffer
	done := make(chan struct{})
	go func() {
		styles := stripes.DefaultStyles.Clone()
		styles.Width = 20
		Render(&out, pr, styles)
		close(done)
	}()

	if _, err := pw.Write([]byte("# Title\n\n---\n")); err != nil {
		t.Fatalf("pipe write: %v", err)
	}
	if _, ok := waitForContains(&out, strings.Repeat("─", 20), 2*time.Second); !ok {
		t.Fatalf("expected thematic break rule before EOF; got: %q", ansi.Strip(out.String()))
	}
	pw.Close()
	<-done
}
