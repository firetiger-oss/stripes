package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	"github.com/muesli/termenv"
)

func TestRender(t *testing.T) {
	hr80 := strings.Repeat("─", 80)
	tests := []struct {
		name   string
		input  string
		output string
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			styles := stripes.DefaultStyles.Clone()
			// Force a small width for list-wrap tests deterministically.
			switch tt.name {
			case "ordered list paragraph wraps with continuation indent",
				"unordered list paragraph wraps within marker indent":
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
	// Force lipgloss to emit ANSI so chroma path activates.
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

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
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

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
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

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
	prev := lipgloss.ColorProfile()
	lipgloss.SetColorProfile(termenv.TrueColor)
	defer lipgloss.SetColorProfile(prev)

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
