// Package markdown registers the Markdown renderer with the stripes
// registry. Import for side effects to enable text/markdown support:
//
//	import _ "github.com/firetiger-oss/stripes/markdown"
package markdown

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	lgtable "charm.land/lipgloss/v2/table"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	chromalexers "github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/text"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "markdown",
		ContentType: "text/markdown",
		Extensions:  []string{".md", ".markdown"},
		RendererFor: stripes.Simple(Render),
	})
}

// Render writes a styled rendering of the Markdown read from r to w.
func Render(w io.Writer, r io.Reader, styles *stripes.Styles) {
	src, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}
	src = stripFrontmatter(src)
	md := goldmark.New(goldmark.WithExtensions(extension.GFM))
	root := md.Parser().Parse(text.NewReader(src))

	ctx := &mdContext{src: src, styles: styles, width: styles.Width, color: stripes.IsANSIEnabled(styles)}
	renderMarkdownBlocks(w, root, ctx)
}

// stripFrontmatter returns src with a leading YAML frontmatter block
// (--- ... ---) removed, including any blank lines between the closing
// fence and the next content. If no well-formed frontmatter is present,
// src is returned unchanged.
func stripFrontmatter(src []byte) []byte {
	var skip int
	switch {
	case bytes.HasPrefix(src, []byte("---\n")):
		skip = 4
	case bytes.HasPrefix(src, []byte("---\r\n")):
		skip = 5
	default:
		return src
	}
	rest := src[skip:]
	for {
		nl := bytes.IndexByte(rest, '\n')
		if nl < 0 {
			return src
		}
		line := rest[:nl]
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		if bytes.Equal(line, []byte("---")) {
			after := rest[nl+1:]
			for len(after) > 0 {
				if after[0] == '\n' {
					after = after[1:]
					continue
				}
				if len(after) >= 2 && after[0] == '\r' && after[1] == '\n' {
					after = after[2:]
					continue
				}
				break
			}
			return after
		}
		rest = rest[nl+1:]
	}
}

type mdContext struct {
	src    []byte
	styles *stripes.Styles
	width  int
	color  bool
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
		hrW := ctx.width
		if hrW <= 0 {
			hrW = 80
		}
		io.WriteString(w, ctx.styles.Syntax.Render(strings.Repeat("─", hrW)))
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
	level := h.Level
	if level < 1 {
		level = 1
	}
	if level > 6 {
		level = 6
	}
	style := ctx.styles.Heading[level-1]
	switch h.Level {
	case 1:
		var buf bytes.Buffer
		renderHeadingInline(&buf, h, ctx, style, true)
		body := buf.String()
		io.WriteString(w, body)
		io.WriteString(w, "\n")
		ruleW := ansi.StringWidth(body)
		if ruleW < 1 {
			ruleW = 1
		}
		io.WriteString(w, style.Render(strings.Repeat("─", ruleW)))
	case 2:
		var buf bytes.Buffer
		renderHeadingInline(&buf, h, ctx, style, false)
		body := buf.String()
		if ctx.color {
			// Apply underline manually so lipgloss doesn't degrade into
			// per-rune rendering (it does that when its own .Underline(true)
			// is set under the TrueColor profile). Re-apply after every
			// internal reset so the underline spans the full heading.
			body = strings.ReplaceAll(body, "\x1b[0m", "\x1b[0m\x1b[4m")
			body = strings.ReplaceAll(body, "\x1b[24m", "\x1b[24m\x1b[4m")
			body = "\x1b[4m" + body + "\x1b[24m"
		}
		io.WriteString(w, body)
	default:
		indent := strings.Repeat(ctx.styles.Indent, h.Level-3)
		io.WriteString(w, indent)
		renderHeadingInline(w, h, ctx, style, false)
	}
}

// renderHeadingInline walks heading inline children, styling literal text
// segments with textStyle (optionally uppercased) and rendering links as
// clickable OSC 8 hyperlinks whose visible text inherits textStyle so the
// link reads as part of the heading. Non-text/non-link inline nodes fall
// through to the regular inline renderer.
func renderHeadingInline(w io.Writer, parent ast.Node, ctx *mdContext, textStyle lipgloss.Style, upper bool) {
	for c := parent.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *ast.Text:
			s := string(v.Segment.Value(ctx.src))
			if upper {
				s = strings.ToUpper(s)
			}
			io.WriteString(w, textStyle.Render(s))
			if v.HardLineBreak() {
				io.WriteString(w, "\n")
			} else if v.SoftLineBreak() {
				io.WriteString(w, " ")
			}
		case *ast.String:
			s := string(v.Value)
			if upper {
				s = strings.ToUpper(s)
			}
			io.WriteString(w, textStyle.Render(s))
		case *ast.Link:
			var inner bytes.Buffer
			if img, ok := imageOnlyLink(v); ok {
				alt := inlineText(img, ctx.src)
				if alt == "" {
					alt = string(img.Destination)
				}
				if upper {
					alt = strings.ToUpper(alt)
				}
				inner.WriteString(textStyle.Render(alt))
			} else {
				renderHeadingInline(&inner, v, ctx, textStyle, upper)
			}
			text := inner.String()
			dest := string(v.Destination)
			if text == "" {
				if upper {
					text = textStyle.Render(strings.ToUpper(dest))
				} else {
					text = textStyle.Render(dest)
				}
			}
			if ctx.color {
				renderHyperlink(w, dest, text)
			} else {
				io.WriteString(w, text)
			}
		case *ast.AutoLink:
			url := string(v.URL(ctx.src))
			text := url
			if upper {
				text = strings.ToUpper(text)
			}
			styled := textStyle.Render(text)
			if ctx.color {
				renderHyperlink(w, url, styled)
			} else {
				io.WriteString(w, styled)
			}
		case *ast.Image:
			// Headings can't display images; render alt text in the heading's
			// style and drop the source URL.
			alt := inlineText(v, ctx.src)
			if alt == "" {
				alt = string(v.Destination)
			}
			if upper {
				alt = strings.ToUpper(alt)
			}
			io.WriteString(w, textStyle.Render(alt))
		default:
			renderInlineNode(w, c, ctx)
		}
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

// imageOnlyLink returns the embedded Image and true when l wraps a single
// image and nothing else — the GitHub-style badge pattern
// "[![alt](image)](href)". Such links should render as clickable text using
// the image's alt instead of dumping the SVG URL.
func imageOnlyLink(l *ast.Link) (*ast.Image, bool) {
	c := l.FirstChild()
	if c == nil || c.NextSibling() != nil {
		return nil, false
	}
	img, ok := c.(*ast.Image)
	return img, ok
}

func renderParagraph(w io.Writer, n ast.Node, ctx *mdContext) {
	var buf bytes.Buffer
	renderInlinesTo(&buf, n, ctx)
	out := buf.String()
	if ctx.width > 0 {
		out = ansi.Wrap(out, ctx.width, "")
	}
	io.WriteString(w, out)
}

func renderBlockquote(w io.Writer, n ast.Node, ctx *mdContext) {
	const indentW = 2 // display width of "│ "

	bodyCtx := ctx
	if ctx.width > 0 {
		cloned := *ctx
		cloned.width = ctx.width - indentW
		if cloned.width < 1 {
			cloned.width = 1
		}
		bodyCtx = &cloned
	}

	var buf bytes.Buffer
	renderMarkdownBlocks(&buf, n, bodyCtx)
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

		bodyCtx := ctx
		if ctx.width > 0 {
			cloned := *ctx
			cloned.width = ctx.width - indentW
			if cloned.width < 1 {
				cloned.width = 1
			}
			bodyCtx = &cloned
		}

		var body bytes.Buffer
		renderListItemBody(&body, item, bodyCtx)
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
			headers = collectRow(child, headerCellContext(ctx))
		case extast.KindTableRow:
			rows = append(rows, collectRow(child, ctx))
		}
	}
	t := lgtable.New().
		Border(ctx.styles.Border).
		BorderStyle(ctx.styles.Syntax).
		BorderRow(true).
		Headers(headers...)
	for _, r := range rows {
		t = t.Row(r...)
	}
	widths := balancedColumnWidths(headers, rows, ctx.width)
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		if row == lgtable.HeaderRow {
			base = ctx.styles.Columns
		} else {
			base = ctx.styles.Rows
		}
		s := base.Padding(0, 1).Align(lipgloss.Left)
		if col >= 0 && col < len(widths) {
			s = s.Width(widths[col])
		}
		return s
	})
	io.WriteString(w, t.Render())
}

// balancedColumnWidths allocates a per-column width (including Padding(0, 1))
// that sums to `totalWidth - (cols+1)` vertical-border chars. Returns nil
// when the table fits naturally — leave column widths unconstrained so
// lipgloss renders at the content's natural size.
//
// Allocation uses max-min water-filling: any column whose natural width
// fits within its fair share of the available space gets exactly that
// width (so "compact" columns aren't padded into empty space and don't
// wrap when they don't have to). Remaining space is then split among the
// columns that still need to stretch, in proportion to their natural
// width — wider columns get proportionally more of the slack so the
// layout feels balanced. Long unbreakable tokens may still break
// mid-word; lipgloss/ansi.Wrap handles that gracefully.
func balancedColumnWidths(headers []string, rows [][]string, totalWidth int) []int {
	cols := tableColumnCount(headers, rows)
	if cols == 0 || totalWidth <= 0 {
		return nil
	}

	nat := columnNaturalWidths(headers, rows, cols)
	natTotal := make([]int, cols)
	natSum := 0
	for i, w := range nat {
		natTotal[i] = w + 2
		natSum += natTotal[i]
	}

	borders := cols + 1
	avail := totalWidth - borders
	if avail <= 0 || natSum <= avail {
		return nil
	}
	return waterFillWidths(natTotal, avail, 3)
}

// waterFillWidths returns per-column widths summing to `avail`. Each
// column's natural width is fixed when it fits within its current
// fair share; the remaining columns split the leftover space
// proportionally to their natural widths, with each share clamped to
// `floor` chars at minimum.
func waterFillWidths(natTotal []int, avail, floor int) []int {
	n := len(natTotal)
	out := make([]int, n)
	if n == 0 || avail <= 0 {
		return out
	}
	if avail <= n*floor {
		for i := range out {
			out[i] = floor
		}
		return out
	}

	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return natTotal[order[a]] < natTotal[order[b]]
	})

	fixed := make([]bool, n)
	remaining := avail
	unfixed := n
	for _, i := range order {
		if unfixed == 0 {
			break
		}
		share := remaining / unfixed
		if natTotal[i] <= share {
			out[i] = natTotal[i]
			remaining -= natTotal[i]
			fixed[i] = true
			unfixed--
		}
	}
	if unfixed == 0 {
		return out
	}

	sumW := 0
	for i, w := range natTotal {
		if !fixed[i] {
			sumW += w
		}
	}
	rem := make([]float64, n)
	used := 0
	for i, w := range natTotal {
		if fixed[i] {
			continue
		}
		share := float64(w) / float64(sumW) * float64(remaining)
		v := int(share)
		if v < floor {
			v = floor
		}
		out[i] = v
		rem[i] = share - float64(v)
		used += v
	}
	diff := remaining - used
	for diff > 0 {
		bestIdx, bestRem := -1, -1.0
		for i, r := range rem {
			if !fixed[i] && r > bestRem {
				bestRem, bestIdx = r, i
			}
		}
		if bestIdx < 0 {
			break
		}
		out[bestIdx]++
		rem[bestIdx] = -1
		diff--
	}
	for diff < 0 {
		bestIdx, bestRem := -1, math.MaxFloat64
		for i, r := range rem {
			if !fixed[i] && out[i] > floor && r < bestRem {
				bestRem, bestIdx = r, i
			}
		}
		if bestIdx < 0 {
			break
		}
		out[bestIdx]--
		rem[bestIdx] = math.MaxFloat64
		diff++
	}
	return out
}

func tableColumnCount(headers []string, rows [][]string) int {
	cols := len(headers)
	for _, r := range rows {
		if len(r) > cols {
			cols = len(r)
		}
	}
	return cols
}

func columnNaturalWidths(headers []string, rows [][]string, cols int) []int {
	out := make([]int, cols)
	measure := func(cells []string) {
		for i, c := range cells {
			if i >= cols {
				break
			}
			if w := ansi.StringWidth(c); w > out[i] {
				out[i] = w
			}
		}
	}
	measure(headers)
	for _, r := range rows {
		measure(r)
	}
	return out
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

// headerCellContext returns a clone of ctx whose Text style inherits Bold from
// Columns. lipgloss/ansi word-wraps cell content at whitespace and emits
// \x1b[0m boundaries between word fragments; if Bold is applied via Columns
// only as the outer wrapper, those resets clear it for every word after the
// first. Baking Bold into the inline Text SGR makes each wrap fragment
// re-emit Bold and the whole header stays bold.
func headerCellContext(ctx *mdContext) *mdContext {
	if !ctx.styles.Columns.GetBold() {
		return ctx
	}
	cloned := *ctx
	clonedStyles := *ctx.styles
	clonedStyles.Text = ctx.styles.Text.Bold(true)
	cloned.styles = &clonedStyles
	return &cloned
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
		io.WriteString(w, ctx.styles.Code.Render(inlineText(n, ctx.src)))
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
		// inlineText (plain) rather than plainInline (pre-styled): feeding a
		// string that already contains ANSI escapes into Strikethrough().Render()
		// corrupts the output under lipgloss v2.
		text := inlineText(n, ctx.src)
		io.WriteString(w, ctx.styles.Text.Strikethrough(true).Render(text))
	case ast.KindLink:
		l := n.(*ast.Link)
		var linkText string
		if img, ok := imageOnlyLink(l); ok {
			linkText = inlineText(img, ctx.src)
		} else {
			linkText = inlineText(l, ctx.src)
		}
		dest := string(l.Destination)
		if linkText == "" {
			linkText = dest
		}
		if ctx.color {
			renderHyperlink(w, dest, ctx.styles.Anchor.Render(linkText))
			return
		}
		if linkText == dest {
			io.WriteString(w, ctx.styles.Anchor.Render(dest))
			return
		}
		io.WriteString(w, ctx.styles.Anchor.Render(linkText))
		io.WriteString(w, " ")
		io.WriteString(w, ctx.styles.Comment.Render("("+dest+")"))
	case ast.KindAutoLink:
		al := n.(*ast.AutoLink)
		url := string(al.URL(ctx.src))
		styled := ctx.styles.Anchor.Render(url)
		if ctx.color {
			renderHyperlink(w, url, styled)
			return
		}
		io.WriteString(w, styled)
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

// renderHyperlink wraps styledText in an OSC 8 escape so terminals that
// support it (iTerm2, kitty, recent gnome-terminal, etc.) make the text
// clickable. Cmd+click opens the URL in the default browser. The text is
// also wrapped in a dashed underline SGR so the link reads as clickable.
func renderHyperlink(w io.Writer, url, styledText string) {
	io.WriteString(w, "\x1b]8;;")
	io.WriteString(w, url)
	io.WriteString(w, "\x1b\\")
	io.WriteString(w, "\x1b[4:5m")
	io.WriteString(w, styledText)
	io.WriteString(w, "\x1b[24m")
	io.WriteString(w, "\x1b]8;;\x1b\\")
}

func renderCodeBlock(w io.Writer, src []byte, lang string, ctx *mdContext) {
	body := strings.TrimRight(string(src), "\n")

	var highlighted string
	if lex := chromalexers.Get(lang); lex != nil && ctx.color {
		var buf bytes.Buffer
		iter, err := lex.Tokenise(nil, body+"\n")
		if err == nil {
			style := chromastyles.Get(chromaStyleName(ctx.styles))
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
		io.WriteString(w, "  ")
		io.WriteString(w, line)
	}
}

// chromaStyleName returns the chroma style name configured on styles,
// or the package default ("github-dark") when unset.
func chromaStyleName(styles *stripes.Styles) string {
	if styles != nil && styles.CodeStyle != "" {
		return styles.CodeStyle
	}
	return "github-dark"
}
