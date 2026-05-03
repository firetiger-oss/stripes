package stripes

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	chromalexers "github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	lgtable "github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/reflow/wordwrap"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func Markdown(w io.Writer, r io.Reader, styles *Styles) {
	src, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := md.Parser().Parse(text.NewReader(src))

	width := styles.Width
	if width <= 0 {
		width = 80
	}
	ctx := &mdContext{src: src, styles: styles, width: width, color: stylesEmitANSI(styles)}
	renderMarkdownBlocks(w, root, ctx)
}

type mdContext struct {
	src    []byte
	styles *Styles
	width  int
	color  bool
}

func stylesEmitANSI(s *Styles) bool {
	return strings.ContainsRune(s.Title.Render("x"), 0x1b) ||
		strings.ContainsRune(s.Syntax.Render("x"), 0x1b) ||
		strings.ContainsRune(s.Anchor.Render("x"), 0x1b)
}

func renderMarkdownBlocks(w io.Writer, parent ast.Node, ctx *mdContext) {
	first := true
	for n := parent.FirstChild(); n != nil; n = n.NextSibling() {
		if !isRenderableBlock(n) {
			continue
		}
		if !first {
			io.WriteString(w, "\n\n")
		}
		first = false
		renderMarkdownBlock(w, n, ctx)
	}
}

func isRenderableBlock(n ast.Node) bool {
	switch n.Kind() {
	case ast.KindHTMLBlock, ast.KindLinkReferenceDefinition:
		return false
	}
	return true
}

func renderMarkdownBlock(w io.Writer, n ast.Node, ctx *mdContext) {
	switch n.Kind() {
	case ast.KindHeading:
		renderHeading(w, n.(*ast.Heading), ctx)
	case ast.KindParagraph, ast.KindTextBlock:
		renderParagraph(w, n, ctx)
	case ast.KindThematicBreak:
		io.WriteString(w, ctx.styles.Syntax.Render(strings.Repeat("─", ctx.width)))
	case ast.KindFencedCodeBlock:
		fb := n.(*ast.FencedCodeBlock)
		renderCodeBlock(w, fb.Lines().Value(ctx.src), string(fb.Language(ctx.src)), ctx)
	case ast.KindCodeBlock:
		renderCodeBlock(w, n.Lines().Value(ctx.src), "", ctx)
	case ast.KindBlockquote:
		renderBlockquote(w, n, ctx)
	case ast.KindList:
		renderList(w, n.(*ast.List), ctx)
	case extast.KindTable:
		renderTable(w, n, ctx)
	default:
		renderInlinesTo(w, n, ctx)
	}
}

func renderHeading(w io.Writer, h *ast.Heading, ctx *mdContext) {
	plain := inlineText(h, ctx.src)
	switch h.Level {
	case 1:
		title := strings.ToUpper(plain)
		styled := ctx.styles.Title.Bold(true).Render(title)
		io.WriteString(w, styled)
		io.WriteString(w, "\n")
		ruleW := runewidth.StringWidth(title)
		if ruleW < 1 {
			ruleW = 1
		}
		io.WriteString(w, ctx.styles.Syntax.Render(strings.Repeat("─", ruleW)))
	case 2:
		io.WriteString(w, ctx.styles.Title.Bold(true).Render(plain))
	default:
		indent := strings.Repeat(ctx.styles.Indent, h.Level-3)
		io.WriteString(w, indent)
		io.WriteString(w, ctx.styles.Name.Render(plain))
	}
}

// inlineText collects the unstyled UTF-8 text content of n's inline children,
// approximating how a heading or alt-text reads when stripped of formatting.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(src))
			if v.HardLineBreak() {
				b.WriteByte('\n')
			} else if v.SoftLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		case *ast.AutoLink:
			b.Write(v.URL(src))
		default:
			b.WriteString(inlineText(c, src))
		}
	}
	return b.String()
}

func renderParagraph(w io.Writer, n ast.Node, ctx *mdContext) {
	var buf bytes.Buffer
	renderInlinesTo(&buf, n, ctx)
	out := buf.String()
	if ctx.width > 0 {
		out = wordwrap.String(out, ctx.width)
	}
	io.WriteString(w, out)
}

func renderBlockquote(w io.Writer, n ast.Node, ctx *mdContext) {
	var buf bytes.Buffer
	renderMarkdownBlocks(&buf, n, ctx)
	prefix := ctx.styles.Comment.Render("│ ")
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		io.WriteString(w, prefix)
		io.WriteString(w, line)
	}
}

func renderList(w io.Writer, list *ast.List, ctx *mdContext) {
	ordered := list.IsOrdered()
	num := list.Start
	if num == 0 {
		num = 1
	}
	first := true
	for item := list.FirstChild(); item != nil; item = item.NextSibling() {
		if item.Kind() != ast.KindListItem {
			continue
		}
		if !first {
			io.WriteString(w, "\n")
		}
		first = false

		var marker string
		if itemHasTaskCheckbox(item) {
			marker = ""
		} else if ordered {
			marker = fmt.Sprintf("%d. ", num)
			num++
		} else {
			marker = "• "
		}
		styledMarker := marker
		if marker != "" {
			styledMarker = ctx.styles.Syntax.Render(marker)
		}
		indentW := runewidth.StringWidth(marker)
		if indentW == 0 {
			indentW = 2
		}
		indent := strings.Repeat(" ", indentW)

		var body bytes.Buffer
		renderListItemBody(&body, item, ctx)
		lines := strings.Split(strings.TrimRight(body.String(), "\n"), "\n")
		for i, line := range lines {
			if i > 0 {
				io.WriteString(w, "\n")
				io.WriteString(w, indent)
			} else {
				io.WriteString(w, styledMarker)
			}
			io.WriteString(w, line)
		}
	}
}

func renderListItemBody(w io.Writer, item ast.Node, ctx *mdContext) {
	first := true
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if !isRenderableBlock(child) {
			continue
		}
		if !first {
			io.WriteString(w, "\n")
		}
		first = false
		if cb, ok := child.(*extast.TaskCheckBox); ok {
			renderTaskCheckbox(w, cb, ctx)
			continue
		}
		// Detect task list: paragraph or text block whose first inline child
		// is a TaskCheckBox.
		if c := child.FirstChild(); c != nil {
			if cb, ok := c.(*extast.TaskCheckBox); ok {
				kind := child.Kind()
				if kind == ast.KindParagraph || kind == ast.KindTextBlock {
					renderTaskCheckbox(w, cb, ctx)
					for sib := cb.NextSibling(); sib != nil; sib = sib.NextSibling() {
						renderInlineNode(w, sib, ctx)
					}
					continue
				}
			}
		}
		renderMarkdownBlock(w, child, ctx)
	}
}

func renderTaskCheckbox(w io.Writer, cb *extast.TaskCheckBox, ctx *mdContext) {
	if cb.IsChecked {
		io.WriteString(w, ctx.styles.Boolean.Render("✓ "))
	} else {
		io.WriteString(w, ctx.styles.Null.Render("☐ "))
	}
}

func itemHasTaskCheckbox(item ast.Node) bool {
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		if !isRenderableBlock(child) {
			continue
		}
		if _, ok := child.(*extast.TaskCheckBox); ok {
			return true
		}
		k := child.Kind()
		if k == ast.KindParagraph || k == ast.KindTextBlock {
			if c := child.FirstChild(); c != nil {
				if _, ok := c.(*extast.TaskCheckBox); ok {
					return true
				}
			}
		}
		return false
	}
	return false
}

func renderTable(w io.Writer, n ast.Node, ctx *mdContext) {
	var headers []string
	var rows [][]string
	for child := n.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.Kind() {
		case extast.KindTableHeader:
			headers = collectRow(child, ctx)
		case extast.KindTableRow:
			rows = append(rows, collectRow(child, ctx))
		}
	}
	t := lgtable.New().
		Border(ctx.styles.Border).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240"))).
		Headers(headers...)
	for _, r := range rows {
		t = t.Row(r...)
	}
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		if row == lgtable.HeaderRow {
			base = ctx.styles.Columns
		} else {
			base = ctx.styles.Rows
		}
		return base.Padding(0, 1).Align(lipgloss.Left)
	})
	io.WriteString(w, t.Render())
}

func collectRow(row ast.Node, ctx *mdContext) []string {
	var cells []string
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		if c.Kind() != extast.KindTableCell {
			continue
		}
		var buf bytes.Buffer
		renderInlinesTo(&buf, c, ctx)
		cells = append(cells, buf.String())
	}
	return cells
}

// renderInlinesTo walks all inline children of n and writes their rendered
// styled output to w.
func renderInlinesTo(w io.Writer, n ast.Node, ctx *mdContext) {
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		renderInlineNode(w, c, ctx)
	}
}

func renderInlineNode(w io.Writer, n ast.Node, ctx *mdContext) {
	switch n.Kind() {
	case ast.KindText:
		t := n.(*ast.Text)
		io.WriteString(w, ctx.styles.Text.Render(string(t.Segment.Value(ctx.src))))
		if t.HardLineBreak() {
			io.WriteString(w, "\n")
		} else if t.SoftLineBreak() {
			io.WriteString(w, " ")
		}
	case ast.KindString:
		s := n.(*ast.String)
		io.WriteString(w, ctx.styles.Text.Render(string(s.Value)))
	case ast.KindCodeSpan:
		io.WriteString(w, ctx.styles.Syntax.Render(plainInline(n, ctx)))
	case ast.KindEmphasis:
		em := n.(*ast.Emphasis)
		text := plainInline(n, ctx)
		var styled string
		if em.Level >= 2 {
			styled = ctx.styles.Text.Bold(true).Render(text)
		} else {
			styled = ctx.styles.Text.Italic(true).Render(text)
		}
		io.WriteString(w, styled)
	case extast.KindStrikethrough:
		text := plainInline(n, ctx)
		io.WriteString(w, ctx.styles.Text.Strikethrough(true).Render(text))
	case ast.KindLink:
		l := n.(*ast.Link)
		linkText := plainInline(l, ctx)
		dest := string(l.Destination)
		if linkText == "" || linkText == dest {
			io.WriteString(w, ctx.styles.Anchor.Render(dest))
			return
		}
		io.WriteString(w, ctx.styles.Anchor.Render(linkText))
		io.WriteString(w, " ")
		io.WriteString(w, ctx.styles.Comment.Render("("+dest+")"))
	case ast.KindAutoLink:
		al := n.(*ast.AutoLink)
		io.WriteString(w, ctx.styles.Anchor.Render(string(al.URL(ctx.src))))
	case ast.KindImage:
		img := n.(*ast.Image)
		alt := plainInline(img, ctx)
		dest := string(img.Destination)
		io.WriteString(w, ctx.styles.Syntax.Render("[image] "))
		if alt != "" {
			io.WriteString(w, ctx.styles.Anchor.Render(alt))
			io.WriteString(w, " ")
		}
		io.WriteString(w, ctx.styles.Comment.Render("("+dest+")"))
	case ast.KindRawHTML:
		// drop raw HTML for minimalist rendering
	default:
		// Unknown inline kind: descend, render children (covers some
		// extension nodes that wrap plain text).
		renderInlinesTo(w, n, ctx)
	}
}

// plainInline returns the rendered styled inline content of n, like
// renderInlinesTo but as a string.
func plainInline(n ast.Node, ctx *mdContext) string {
	var buf bytes.Buffer
	renderInlinesTo(&buf, n, ctx)
	return buf.String()
}

func renderCodeBlock(w io.Writer, src []byte, lang string, ctx *mdContext) {
	body := strings.TrimRight(string(src), "\n")
	indent := ctx.styles.Indent
	if indent == "" {
		indent = "  "
	}

	var highlighted string
	if lex := chromalexers.Get(lang); lex != nil && ctx.color {
		var buf bytes.Buffer
		iter, err := lex.Tokenise(nil, body+"\n")
		if err == nil {
			style := chromastyles.Get("github-dark")
			if style == nil {
				style = chromastyles.Fallback
			}
			fmtr := formatters.Get("terminal256")
			if fmtr == nil {
				fmtr = chroma.Formatter(formatters.Fallback)
			}
			if err := fmtr.Format(&buf, style, iter); err == nil {
				highlighted = strings.TrimRight(buf.String(), "\n")
			}
		}
	}

	if highlighted == "" {
		// Fallback: per-line Syntax style.
		lines := strings.Split(body, "\n")
		styled := make([]string, len(lines))
		for i, line := range lines {
			styled[i] = ctx.styles.Syntax.Render(line)
		}
		highlighted = strings.Join(styled, "\n")
	} else if !ctx.color {
		highlighted = ansi.Strip(highlighted)
	}

	lines := strings.Split(highlighted, "\n")
	for i, line := range lines {
		if i > 0 {
			io.WriteString(w, "\n")
		}
		io.WriteString(w, indent)
		io.WriteString(w, line)
	}
}
