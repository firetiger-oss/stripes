package trace

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// renderResourceSpans groups the input into traces by trace_id and
// renders each as its own waterfall block, separated by a blank line.
// Empty input produces no output.
func renderResourceSpans(w io.Writer, seq iter.Seq[*tracev1.ResourceSpans], opts *Options) error {
	traces := collectTraces(seq)
	if len(traces) == 0 {
		return nil
	}
	for i, t := range traces {
		if i > 0 {
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
		if err := renderOneTrace(w, t, opts); err != nil {
			return err
		}
	}
	return nil
}

// span is the renderer's internal view of a single OTLP Span
// enriched with the service.name and service.namespace from its
// enclosing Resource. Children are resolved during tree
// construction; siblings under each parent are sorted by start time.
type span struct {
	pb        *tracev1.Span
	service   string                         // service.name, defaults to "unknown_service"
	namespace string                         // service.namespace (may be empty)
	scope     *commonv1.InstrumentationScope // enclosing ScopeSpans' scope (may be nil)
	start     time.Time
	end       time.Time
	children  []*span
	// index is the span's 0-based position in the chart's
	// depth-first render order. Shown as the left-column number and,
	// in the -v tiles, as "span N" so the two views correlate.
	index int
}

// label returns the bar's overlaid service identifier:
// "<namespace>/<service>" when namespace is set, otherwise just
// "<service>".
func (s *span) label() string {
	if s.namespace == "" {
		return s.service
	}
	return s.namespace + "/" + s.service
}

// trace is a single trace_id's worth of spans plus the time axis and
// roots used for rendering. roots are the spans whose parent was not
// found in this batch (true roots OR cross-batch orphans, both
// rendered as top-level entries — see design notes).
type trace struct {
	id         []byte
	idHex      string
	spans      []*span
	roots      []*span
	rangeStart time.Time
	rangeEnd   time.Time
	errors     int
	// serviceDepths records each service's observed depth range
	// across all of its spans in this trace. Keyed by raw
	// service.name (matching the hash used by [serviceHSL]). The
	// bright→dim gradient is then anchored on each service's own
	// minDepth..maxDepth window so the service's full colour range
	// is always used in the bars below — regardless of where in
	// the global tree the service happens to sit.
	serviceDepths map[string]depthRange
	// unit is the single duration unit used to render every
	// duration in this trace (header Duration row, legend self-times,
	// per-span durations, ruler tick labels). Picked once based on
	// the trace's total duration via [pickDurationUnit] so the eye
	// can compare values column-by-column without re-parsing the
	// suffix on each row.
	unit durationUnit
}

// depthRange captures the [min, max] tree depth at which a given
// service appears in a trace. min == max for single-span services.
type depthRange struct{ min, max int }

func (t *trace) duration() time.Duration { return t.rangeEnd.Sub(t.rangeStart) }

// collectTraces flattens the input ResourceSpans iterator into
// per-trace_id buckets, computes each trace's time range, and builds
// the parent/child links. Returned traces are in the order their
// first-seen span appeared (stable for tests).
func collectTraces(seq iter.Seq[*tracev1.ResourceSpans]) []*trace {
	bucketIdx := map[string]int{}
	var buckets []*trace

	for rs := range seq {
		if rs == nil {
			continue
		}
		serviceName, serviceNamespace := extractServiceInfo(rs)
		for _, ss := range rs.GetScopeSpans() {
			scope := ss.GetScope()
			for _, pbSpan := range ss.GetSpans() {
				if pbSpan == nil {
					continue
				}
				key := string(pbSpan.GetTraceId())
				idx, ok := bucketIdx[key]
				if !ok {
					idx = len(buckets)
					bucketIdx[key] = idx
					buckets = append(buckets, &trace{
						id:    pbSpan.GetTraceId(),
						idHex: traceIDHex(pbSpan.GetTraceId()),
					})
				}
				t := buckets[idx]
				s := &span{
					pb:        pbSpan,
					service:   serviceName,
					namespace: serviceNamespace,
					scope:     scope,
					start:     time.Unix(0, int64(pbSpan.GetStartTimeUnixNano())),
					end:       time.Unix(0, int64(pbSpan.GetEndTimeUnixNano())),
				}
				t.spans = append(t.spans, s)
				if pbSpan.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR {
					t.errors++
				}
			}
		}
	}

	for _, t := range buckets {
		buildTree(t)
		computeRange(t)
		computeServiceDepths(t)
		numberSpans(t)
		t.unit = pickDurationUnit(t.duration())
	}
	return buckets
}

// numberSpans assigns each span its 1-based index in depth-first
// render order (roots in order, then children) — the same order the
// chart and the -v tiles iterate, so the left-column number and the
// "span N" tile headers line up.
func numberSpans(t *trace) {
	i := 1
	var walk func(*span)
	walk = func(s *span) {
		s.index = i
		i++
		for _, c := range s.children {
			walk(c)
		}
	}
	for _, r := range t.roots {
		walk(r)
	}
}

// computeServiceDepths walks every root's subtree and records, per
// service, the minimum and maximum depths at which that service's
// spans appear. The bright→dim gradient applied to each span's bar
// is scaled against that service's own range so every service uses
// its full bright→dim spectrum, regardless of where it sits in the
// trace's global tree.
func computeServiceDepths(t *trace) {
	t.serviceDepths = make(map[string]depthRange, 4)
	var walk func(*span, int)
	walk = func(s *span, depth int) {
		dr, ok := t.serviceDepths[s.service]
		if !ok {
			dr = depthRange{min: depth, max: depth}
		} else {
			if depth < dr.min {
				dr.min = depth
			}
			if depth > dr.max {
				dr.max = depth
			}
		}
		t.serviceDepths[s.service] = dr
		for _, c := range s.children {
			walk(c, depth+1)
		}
	}
	for _, r := range t.roots {
		walk(r, 0)
	}
}

// buildTree links spans by parent_span_id and computes the root list.
// Spans whose ParentSpanId is empty OR not present in the same trace
// bucket become roots. Sibling order is ascending start_time, with a
// stable tiebreak on the span's index in the input.
func buildTree(t *trace) {
	bySpanID := make(map[string]*span, len(t.spans))
	for _, s := range t.spans {
		bySpanID[string(s.pb.GetSpanId())] = s
	}
	for _, s := range t.spans {
		parentID := s.pb.GetParentSpanId()
		if len(parentID) == 0 {
			t.roots = append(t.roots, s)
			continue
		}
		parent, ok := bySpanID[string(parentID)]
		if !ok {
			t.roots = append(t.roots, s)
			continue
		}
		parent.children = append(parent.children, s)
	}
	sortByStart(t.roots)
	for _, s := range t.spans {
		sortByStart(s.children)
	}
}

func sortByStart(ss []*span) {
	sort.SliceStable(ss, func(i, j int) bool { return ss[i].start.Before(ss[j].start) })
}

// computeRange sets rangeStart/rangeEnd to min(start) / max(end)
// across every span in the trace. Per design, parent/child
// out-of-bounds spans are rendered in-place rather than clipped, so
// the axis is the union of all spans.
func computeRange(t *trace) {
	for i, s := range t.spans {
		if i == 0 {
			t.rangeStart = s.start
			t.rangeEnd = s.end
			continue
		}
		if s.start.Before(t.rangeStart) {
			t.rangeStart = s.start
		}
		if s.end.After(t.rangeEnd) {
			t.rangeEnd = s.end
		}
	}
}

// extractServiceInfo pulls the service.name and service.namespace
// attributes from a ResourceSpans' Resource. Falls back to
// "unknown_service" when service.name is missing or empty (matching
// the OTel SDK convention). service.namespace is returned empty
// when absent.
func extractServiceInfo(rs *tracev1.ResourceSpans) (name, namespace string) {
	for _, kv := range rs.GetResource().GetAttributes() {
		switch kv.GetKey() {
		case "service.name":
			name = kv.GetValue().GetStringValue()
		case "service.namespace":
			namespace = kv.GetValue().GetStringValue()
		}
	}
	if name == "" {
		name = "unknown_service"
	}
	return name, namespace
}

// traceIDHex returns the full hex-encoded trace_id. OTel trace IDs
// are 16 bytes (32 hex chars) but the renderer doesn't enforce
// length — whatever bytes are present are encoded. Empty IDs render
// as "0" so the header still has a stable value to show.
func traceIDHex(id []byte) string {
	if len(id) == 0 {
		return "0"
	}
	const hexdigits = "0123456789abcdef"
	out := make([]byte, 0, len(id)*2)
	for _, b := range id {
		out = append(out, hexdigits[b>>4], hexdigits[b&0x0f])
	}
	return string(out)
}

// columnWidths captures the widths used for a single trace's layout.
// id + name + dur + bar (plus fixed gutters/gaps/status slot) sum to
// the terminal total.
type columnWidths struct {
	num   int // span-number column (right-aligned index)
	name  int // tree+kind+name column
	dur   int // duration column
	bar   int // bar column (always at least 8)
	total int
}

const (
	durColWidth    = 7 // matches "1m23s " / "999ns" / "45.6µs"; leaves room for one trailing space
	gapWidth       = 2 // gap between columns
	minBarWidth    = 8 // one cell == 8 eighths; we want at least 2 cells.
	gutterLeft     = 2 // left margin
	statusColWidth = 1 // 1-char slot between name's gap and duration: "✗" or " "
)

// layoutColumns picks name/dur/bar widths for t given the total
// terminal width. name auto-fits to content; dur is fixed; bar gets
// the remainder (clipped to minBarWidth). When the input is wider
// than total, name is truncated to keep the bar usable.
//
// The name column accounts for: tree-prefix (3 cols per depth level
// above 0) + connector (3 cols when depth > 0) + kind glyph slot
// (2 cols when the kind has a visible glyph, else 0) + name text.
func layoutColumns(t *trace, total int) columnWidths {
	if total <= 0 {
		total = 80
	}
	maxName := 0
	var walk func(s *span, depth int)
	walk = func(s *span, depth int) {
		prefixLen := 0
		if depth > 0 {
			prefixLen = depth * 3 // 3 cols per ancestor prefix + connector at this depth
		}
		kindLen := 0
		if kindGlyphFor(s.pb.GetKind()) != "" {
			kindLen = 2 // glyph + trailing space
		}
		nameW := prefixLen + kindLen + ansi.StringWidth(s.pb.GetName())
		if nameW > maxName {
			maxName = nameW
		}
		for _, c := range s.children {
			walk(c, depth+1)
		}
	}
	for _, r := range t.roots {
		walk(r, 0)
	}
	if maxName < 8 {
		maxName = 8
	}
	numWidth := len(strconv.Itoa(max(1, len(t.spans)))) // 1-based indices

	// fixedOverhead must equal what writeSpan emits between the
	// terminal's left edge and the bar column: gutter + num + gap +
	// name-gap + status-slot + delta-column + gap + dur-column +
	// bar-gap. writeTraceHeader uses the same offset to align its
	// bottom-rule tick marks under the bar.
	fixedOverhead := gutterLeft + numWidth + gapWidth + gapWidth + statusColWidth + durColWidth + gapWidth + durColWidth + gapWidth
	avail := total - fixedOverhead
	if avail < minBarWidth+8 {
		avail = minBarWidth + 8
	}

	name := maxName
	bar := avail - name
	if bar < minBarWidth {
		// Squeeze name to recover space for the bar.
		shrink := minBarWidth - bar
		if shrink > name-4 {
			shrink = name - 4
		}
		if shrink < 0 {
			shrink = 0
		}
		name -= shrink
		bar = avail - name
		if bar < minBarWidth {
			bar = minBarWidth
		}
	}
	return columnWidths{num: numWidth, name: name, dur: durColWidth, bar: bar, total: total}
}

// renderOneTrace emits the multi-line header (continuous rule,
// key:value metadata, ruler+separator with tick labels) and then
// every span's row.
func renderOneTrace(w io.Writer, t *trace, opts *Options) error {
	widths := layoutColumns(t, opts.Width)
	if err := writeTraceHeader(w, t, opts, widths); err != nil {
		return err
	}
	for _, r := range t.roots {
		if err := writeSpan(w, t, r, 0, opts, widths, "", true); err != nil {
			return err
		}
	}
	if opts.Verbose {
		if err := writeSpansSection(w, t, opts); err != nil {
			return err
		}
	}
	return nil
}

// writeTraceHeader emits the trace's multi-line header:
//
//	─────────────────────────────────────────────────... (top rule)
//	  Trace:    4f3a91d2c1e57b890000000000000001
//	  Root:     GET /api/v1/users
//	  Duration: 120ms
//	  Spans:    4
//	  Errors:   1
//	──────────────|─────|─────|─────|─────|────────... (bottom rule + ticks)
//	               20ms  40ms  60ms  80ms 100ms
//
// The top and bottom rules span the full terminal width and are
// rendered through styles.Comment so they recede as structural
// separators. The key/value rows in between render in the
// terminal's default colour so the metadata stays the visual
// foreground of the header. The bottom rule embeds vertical tick
// marks in the bar-column region; their labels appear on the line
// below (also styled Comment so they fade behind the data rows).
func writeTraceHeader(w io.Writer, t *trace, opts *Options, widths columnWidths) error {
	commentStyle := opts.Styles.Comment
	width := widths.total

	if _, err := io.WriteString(w, commentStyle.Render(strings.Repeat("─", width))+"\n"); err != nil {
		return err
	}

	rootName := "(no root span)"
	if len(t.roots) > 0 {
		rootName = t.roots[0].pb.GetName()
		if rootName == "" {
			rootName = "(unnamed)"
		}
	}
	kv := []struct{ k, v string }{
		{"Trace", t.idHex},
		{"Root", rootName},
		{"Duration", formatDurationAs(t.duration(), t.unit)},
		{"Spans", fmt.Sprintf("%d", len(t.spans))},
	}
	if t.errors > 0 {
		kv = append(kv, struct{ k, v string }{"Errors", fmt.Sprintf("%d", t.errors)})
	}
	maxK := 0
	for _, p := range kv {
		if len(p.k) > maxK {
			maxK = len(p.k)
		}
	}
	indent := strings.Repeat(" ", gutterLeft)
	valueStyle := lipgloss.NewStyle().Bold(true)
	for _, p := range kv {
		// Keys render plain; values render bold so the trace's
		// identity / timing pop against the surrounding rules and the
		// (rendered through Comment) labels below.
		if _, err := io.WriteString(w, indent+padRight(p.k+":", maxK+1)+" "+valueStyle.Render(p.v)+"\n"); err != nil {
			return err
		}
	}

	if err := writeServicesLegend(w, t, opts); err != nil {
		return err
	}

	// Bottom rule + embedded ticks. Everything to the left of the bar
	// column is plain ─; the bar column is ─ with | at tick positions;
	// the line below carries the tick labels. The delta column sits
	// before the bar; the duration column trails AFTER the bar, so
	// only one dur-width column is counted in the offset here. The
	// leading span-number column adds num + gap.
	barOffset := gutterLeft + widths.num + gapWidth + widths.name + gapWidth + statusColWidth + widths.dur + gapWidth
	dur := t.duration()
	cells := make([]rune, widths.bar)
	for i := range cells {
		cells[i] = '─'
	}
	labels := map[int]string{}
	if dur > 0 {
		tickInterval := pickTickInterval(dur, widths.bar)
		if tickInterval <= 0 {
			tickInterval = dur
		}
		for off := time.Duration(0); off <= dur; off += tickInterval {
			col := int(float64(off) / float64(dur) * float64(widths.bar))
			if col < 0 {
				col = 0
			}
			if col >= widths.bar {
				col = widths.bar - 1
			}
			cells[col] = '|'
			labels[col] = formatDurationAs(off, t.unit)
		}
	}
	// Extend the rule past the bar's right edge to the full terminal
	// width: the duration column now trails AFTER the bar, so the
	// rule needs the trailing run of ─ to span the whole row.
	trailing := width - barOffset - widths.bar
	if trailing < 0 {
		trailing = 0
	}
	bottomRule := strings.Repeat("─", barOffset) + string(cells) + strings.Repeat("─", trailing)
	if _, err := io.WriteString(w, commentStyle.Render(bottomRule)+"\n"); err != nil {
		return err
	}

	labelRow := make([]byte, widths.bar)
	for i := range labelRow {
		labelRow[i] = ' '
	}
	colsSorted := make([]int, 0, len(labels))
	for c := range labels {
		colsSorted = append(colsSorted, c)
	}
	sort.Ints(colsSorted)
	nextOK := 0
	for _, c := range colsSorted {
		if c < nextOK {
			continue
		}
		s := labels[c]
		col := c
		if col+len(s) > widths.bar {
			col = widths.bar - len(s)
			if col < 0 {
				continue
			}
		}
		copy(labelRow[col:], s)
		nextOK = col + len(s) + 1
	}
	prefix := strings.Repeat(" ", barOffset)
	_, err := io.WriteString(w, prefix+commentStyle.Render(strings.TrimRight(string(labelRow), " "))+"\n")
	return err
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// pickTickInterval returns a "round" time interval that produces ~6
// ticks across dur. Round = 1, 2, or 5 × 10^n for the unit of dur.
func pickTickInterval(dur time.Duration, barW int) time.Duration {
	if barW < 12 {
		return dur
	}
	target := dur / time.Duration(barW/12)
	if target <= 0 {
		target = dur
	}
	// Snap target to 1/2/5 × 10^n in nanoseconds.
	n := int64(target)
	if n <= 0 {
		return dur
	}
	mag := int64(1)
	for mag*10 <= n {
		mag *= 10
	}
	for _, m := range [...]int64{1, 2, 5, 10} {
		if mag*m >= n {
			return time.Duration(mag * m)
		}
	}
	return time.Duration(mag * 10)
}

// writeSpan emits one row for s plus any attribute/event detail
// rows (verbose mode), then recurses into children.
//
// Layout of a row:
//
//	<tree><kind?> <name>  <status> <dur>  <bar(with overlaid service?)>
//
// where:
//   - tree (prefix + connector) renders unstyled so the branch glyphs
//     stay in the terminal's default foreground.
//   - kind appears as a directional glyph (→ ← ⚑ ⚟) only for the
//     externally-observable kinds; INTERNAL/UNSPECIFIED have no glyph.
//   - the span name renders plain (default fg) — the service identity
//     no longer lives here.
//   - status is a 1-col slot: ✗ for ERROR, blank otherwise.
//   - the bar is a row of `█` characters in the service hue (see
//     [renderBar]); the Services legend at the top of the trace
//     header acts as the colour key, so the bar itself carries no
//     overlaid text.
//
// Per-span attribute/event detail is no longer shown inline; under
// -v it lives in the trailing "Spans" tile section (see
// [writeSpansSection]).
func writeSpan(w io.Writer, t *trace, s *span, depth int, opts *Options, widths columnWidths, prefix string, lastSibling bool) error {
	connector := ""
	if depth > 0 {
		if lastSibling {
			connector = "└─ "
		} else {
			connector = "├─ "
		}
	}
	kindGlyph := kindGlyphFor(s.pb.GetKind())
	statusGlyph := " "
	if s.pb.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR {
		statusGlyph = opts.Styles.Deletion.Render("✗")
	}

	rawName := s.pb.GetName()
	if rawName == "" {
		rawName = "(unnamed)"
	}

	// Name column: split into an unstyled tree part (prefix +
	// connector) and a plain-rendered span part (kind glyph + name).
	// Combined width must fit widths.name; we truncate the name part
	// if needed, keeping the tree intact so structure stays readable.
	treePart := prefix + connector
	spanPart := rawName
	if kindGlyph != "" {
		spanPart = kindGlyph + " " + rawName
	}
	treeW := ansi.StringWidth(treePart)
	spanBudget := widths.name - treeW
	if spanBudget < 0 {
		spanBudget = 0
	}
	spanPart = truncateWidth(spanPart, spanBudget)
	spanW := ansi.StringWidth(spanPart)
	namePad := widths.name - treeW - spanW
	if namePad < 0 {
		namePad = 0
	}

	durStr := formatDurationAs(s.end.Sub(s.start), t.unit)
	durPad := widths.dur - len(durStr)
	if durPad < 0 {
		durPad = 0
	}
	durStyled := opts.Styles.Comment.Render(durStr)

	// Delta column: the span's start offset from the trace's anchor
	// (i.e. how late this span starts relative to the root). Rendered
	// in the same unit as the duration column for trivial visual
	// comparison, and styled through Comment so the eye lands on the
	// duration value first.
	deltaStr := formatDurationAs(s.start.Sub(t.rangeStart), t.unit)
	deltaPad := widths.dur - len(deltaStr)
	if deltaPad < 0 {
		deltaPad = 0
	}
	deltaStyled := opts.Styles.Comment.Render(deltaStr)

	bar := renderBar(s, t, widths.bar, opts, depth, spanErrorMessage(s))

	// Bold the span-name portion (kind glyph + name), tinted in the
	// service's bright (first) gradient colour so the name reads in
	// the service hue. Width was already measured against the plain
	// text above, so the added SGR escapes don't shift alignment.
	nameStyle := lipgloss.NewStyle().Bold(true)
	if stripes.IsANSIEnabled(opts.Styles) {
		dr := t.serviceDepths[s.service]
		nameStyle = nameStyle.Foreground(lipgloss.Color(serviceColorNameAtDepth(s.service, depth, dr.min, dr.max)))
	}
	spanBold := nameStyle.Render(spanPart)

	// span-number column: the span's depth-first index, right-aligned
	// and rendered dim. Correlates with the "span N" tile headers
	// under -v.
	numStyled := opts.Styles.Comment.Render(padLeft(strconv.Itoa(s.index), widths.num))

	indent := strings.Repeat(" ", gutterLeft)
	parts := []string{
		indent,
		numStyled,
		strings.Repeat(" ", gapWidth),
		treePart,
		spanBold,
		strings.Repeat(" ", namePad+gapWidth),
		statusGlyph,
		strings.Repeat(" ", deltaPad),
		deltaStyled,
		strings.Repeat(" ", gapWidth),
		bar,
		strings.Repeat(" ", gapWidth),
		strings.Repeat(" ", durPad),
		durStyled,
		"\n",
	}
	if _, err := io.WriteString(w, strings.Join(parts, "")); err != nil {
		return err
	}

	childPrefix := prefix
	if depth > 0 {
		if lastSibling {
			childPrefix += "   "
		} else {
			childPrefix += "│  "
		}
	}
	for i, c := range s.children {
		if err := writeSpan(w, t, c, depth+1, opts, widths, childPrefix, i == len(s.children)-1); err != nil {
			return err
		}
	}
	return nil
}

// writeSpansSection renders the trailing "Spans" detail block shown
// under -v: one tile per span (in the same depth-first order as the
// chart), separated by full-width rules. Each tile carries the
// span's id + name header and its attributes, status message, and
// events. Span IDs are the join key back to the chart's left column.
func writeSpansSection(w io.Writer, t *trace, opts *Options) error {
	spans := flattenSpans(t)
	if len(spans) == 0 {
		return nil
	}
	width := opts.Width
	commentStyle := opts.Styles.Comment

	label := " Spans "
	rule := width - ansi.StringWidth(label)
	if rule < 2 {
		rule = 2
	}
	left := rule / 2
	header := strings.Repeat("─", left) + label + strings.Repeat("─", rule-left)
	if _, err := io.WriteString(w, "\n"+commentStyle.Render(header)+"\n"); err != nil {
		return err
	}

	for i, s := range spans {
		if i > 0 {
			if _, err := io.WriteString(w, commentStyle.Render(strings.Repeat("─", width))+"\n"); err != nil {
				return err
			}
		}
		if err := writeSpanTile(w, t, s, opts); err != nil {
			return err
		}
	}
	return nil
}

// flattenSpans returns every span in depth-first order, matching the
// row order produced by the waterfall chart.
func flattenSpans(t *trace) []*span {
	out := make([]*span, 0, len(t.spans))
	var walk func(*span)
	walk = func(s *span) {
		out = append(out, s)
		for _, c := range s.children {
			walk(c)
		}
	}
	for _, r := range t.roots {
		walk(r)
	}
	return out
}

// writeSpanTile renders a single span's detail tile: a header line
// ("span N" · name · service, status marker), then its attributes,
// status message, and events. Attribute values show their first line,
// truncated to the terminal width. Keys align on the `=`.
func writeSpanTile(w io.Writer, t *trace, s *span, opts *Options) error {
	keyStyle := opts.Styles.Name
	valStyle := opts.Styles.String
	dimStyle := opts.Styles.Comment

	name := s.pb.GetName()
	if name == "" {
		name = "(unnamed)"
	}
	dr := t.serviceDepths[s.service]
	nameStyle := lipgloss.NewStyle().Bold(true)
	if stripes.IsANSIEnabled(opts.Styles) {
		nameStyle = nameStyle.Foreground(lipgloss.Color(serviceColorNameAtDepth(s.service, 0, dr.min, dr.max)))
	}
	indent := strings.Repeat(" ", gutterLeft)
	statusGlyph := ""
	if s.pb.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR {
		statusGlyph = "  " + opts.Styles.Deletion.Render("✗")
	}
	headerLine := indent + dimStyle.Render(fmt.Sprintf("span %d", s.index)) + "  " + nameStyle.Render(name) +
		"  " + dimStyle.Render(s.label()) + statusGlyph + "\n"
	if _, err := io.WriteString(w, headerLine); err != nil {
		return err
	}

	// Body indent + value width budget (terminal width minus the
	// indent, key column, and " = " separator).
	body := strings.Repeat(" ", gutterLeft*2)
	attrs := append([]*commonv1.KeyValue(nil), s.pb.GetAttributes()...)
	slices.SortFunc(attrs, func(a, b *commonv1.KeyValue) int {
		return strings.Compare(a.GetKey(), b.GetKey())
	})

	hasErr := s.pb.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR && s.pb.GetStatus().GetMessage() != ""
	maxKey := 0
	for _, kv := range attrs {
		if k := kv.GetKey(); len(k) > maxKey {
			maxKey = len(k)
		}
	}
	if hasErr && len("status.message") > maxKey {
		maxKey = len("status.message")
	}
	if maxKey > 32 {
		maxKey = 32
	}
	valBudget := opts.Width - len(body) - maxKey - len(" = ")
	if valBudget < 8 {
		valBudget = 8
	}

	if hasErr {
		line := fmt.Sprintf("%s%s %s = %s\n",
			body,
			opts.Styles.Deletion.Render("⚠"),
			keyStyle.Render(padRight("status.message", maxKey)),
			valStyle.Render(clampValueTo(s.pb.GetStatus().GetMessage(), valBudget)),
		)
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	for _, kv := range attrs {
		line := fmt.Sprintf("%s%s = %s\n",
			body,
			keyStyle.Render(padRight(kv.GetKey(), maxKey)),
			valStyle.Render(clampAttrTo(kv.GetValue(), valBudget)),
		)
		if _, err := io.WriteString(w, line); err != nil {
			return err
		}
	}
	for _, ev := range s.pb.GetEvents() {
		off := time.Unix(0, int64(ev.GetTimeUnixNano())).Sub(s.start)
		head := fmt.Sprintf("%s%s %s %s\n",
			body,
			dimStyle.Render("◆"),
			keyStyle.Render(ev.GetName()),
			dimStyle.Render("@ +"+formatDurationAs(off, t.unit)),
		)
		if _, err := io.WriteString(w, head); err != nil {
			return err
		}
		evAttrs := append([]*commonv1.KeyValue(nil), ev.GetAttributes()...)
		slices.SortFunc(evAttrs, func(a, b *commonv1.KeyValue) int {
			return strings.Compare(a.GetKey(), b.GetKey())
		})
		evMaxKey := 0
		for _, kv := range evAttrs {
			if k := kv.GetKey(); len(k) > evMaxKey {
				evMaxKey = len(k)
			}
		}
		if evMaxKey > 32 {
			evMaxKey = 32
		}
		evBudget := opts.Width - len(body) - 2 - evMaxKey - len(" = ")
		if evBudget < 8 {
			evBudget = 8
		}
		for _, kv := range evAttrs {
			line := fmt.Sprintf("%s  %s = %s\n",
				body,
				keyStyle.Render(padRight(kv.GetKey(), evMaxKey)),
				valStyle.Render(clampAttrTo(kv.GetValue(), evBudget)),
			)
			if _, err := io.WriteString(w, line); err != nil {
				return err
			}
		}
	}
	return nil
}

// padRight returns s padded with spaces to width n. Strings already
// at or beyond n are returned unchanged.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

// truncateWidth shortens s to fit width n in terminal columns,
// appending an ellipsis when truncation occurs. n <= 1 returns at
// most one character.
func truncateWidth(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if ansi.StringWidth(s) <= n {
		return s
	}
	if n == 1 {
		return "…"
	}
	// Walk runes, accumulating width until adding the next would
	// overflow n-1 (leaving room for the ellipsis).
	target := n - 1
	out := make([]byte, 0, len(s))
	wsum := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if wsum+rw > target {
			break
		}
		out = append(out, string(r)...)
		wsum += rw
	}
	return string(out) + "…"
}

// kindGlyphFor maps an OTLP SpanKind to a single-rune glyph
// encoding the span's causality role for the externally-observable
// kinds. INTERNAL and UNSPECIFIED return the empty string: most
// spans in a typical trace are INTERNAL, and a dot/question-mark in
// every row would only add visual noise.
func kindGlyphFor(k tracev1.Span_SpanKind) string {
	switch k {
	case tracev1.Span_SPAN_KIND_CLIENT:
		return "→"
	case tracev1.Span_SPAN_KIND_SERVER:
		return "←"
	case tracev1.Span_SPAN_KIND_PRODUCER:
		return "⚑"
	case tracev1.Span_SPAN_KIND_CONSUMER:
		return "⚟"
	}
	return ""
}

// clampAttrTo renders an OTLP AnyValue for tile display: strings
// unquoted, non-strings via [formatAttrValue]'s structured form,
// then collapsed to the first line and truncated to max columns
// with a "..." marker.
func clampAttrTo(v *commonv1.AnyValue, max int) string {
	if v == nil {
		return "null"
	}
	if sv, ok := v.GetValue().(*commonv1.AnyValue_StringValue); ok {
		return clampValueTo(sv.StringValue, max)
	}
	return clampValueTo(formatAttrValue(v), max)
}

// clampValueTo collapses s to its first line and truncates it to
// max columns, appending "..." when truncation occurs (the marker
// counts toward the cap so the result never exceeds it).
func clampValueTo(s string, max int) string {
	s = firstLine(s)
	if max < 3 {
		max = 3
	}
	if ansi.StringWidth(s) <= max {
		return s
	}
	budget := max - len("...")
	var b strings.Builder
	w := 0
	for _, r := range s {
		rw := ansi.StringWidth(string(r))
		if w+rw > budget {
			break
		}
		b.WriteRune(r)
		w += rw
	}
	b.WriteString("...")
	return b.String()
}

// padLeft right-aligns s by prefixing spaces to width n.
func padLeft(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return strings.Repeat(" ", n-len(s)) + s
}

// formatAttrValue renders an OTLP AnyValue as a compact one-line
// string for verbose output. Strings are JSON-quoted; complex
// (kvlist / array) values are rendered as a minimal JSON-ish form.
func formatAttrValue(v *commonv1.AnyValue) string {
	if v == nil {
		return "null"
	}
	switch x := v.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return strconv.Quote(x.StringValue)
	case *commonv1.AnyValue_BoolValue:
		return strconv.FormatBool(x.BoolValue)
	case *commonv1.AnyValue_IntValue:
		return strconv.FormatInt(x.IntValue, 10)
	case *commonv1.AnyValue_DoubleValue:
		return strconv.FormatFloat(x.DoubleValue, 'f', -1, 64)
	case *commonv1.AnyValue_BytesValue:
		return fmt.Sprintf("0x%x", x.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		parts := make([]string, 0, len(x.ArrayValue.GetValues()))
		for _, vv := range x.ArrayValue.GetValues() {
			parts = append(parts, formatAttrValue(vv))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *commonv1.AnyValue_KvlistValue:
		parts := make([]string, 0, len(x.KvlistValue.GetValues()))
		for _, kv := range x.KvlistValue.GetValues() {
			parts = append(parts, kv.GetKey()+"="+formatAttrValue(kv.GetValue()))
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	return ""
}

// renderBar produces the glyph sequence for s within a barW-wide
// column. Every cell is a REAL character so the output round-trips
// through copy-paste without losing the bar visualisation:
//   - full cells use `█` in the service's foreground colour;
//   - the right edge gets eighth-block sub-cell precision via a
//     left-anchored partial block (`▏▎▍▌▋▊▉`) in the same hue, with
//     a transparent right side so the bar tapers cleanly;
//   - the left edge always snaps to a whole cell.
//
// depth is the span's tree depth, used to modulate brightness
// against the service's own min/max depth window via [barFillStyle]
// (a linear bright-to-dim gradient).
//
// errMsg, when non-empty, is overlaid on the bar area starting at
// column 0 (the LEFT edge of the bar column — not the bar's own
// start). Left-aligning means every error in the chart lines up
// vertically and the eye doesn't have to chase the bar's position.
// The overlay is `ERROR <message>` with "ERROR" rendered through
// styles.Deletion (red) and the message in default foreground;
// both are bold so the line reads as one band. In cells where the
// overlay and the bar overlap, the bar's hue is applied as the
// overlay character's BACKGROUND — keeping the bar's
// timing-on-axis information visible AS a highlight behind the
// text. Outside the bar's reach the overlay renders plain.
//
// Special cases:
//   - Zero-duration span: a `│` glyph at the snapped start position.
//   - Bar < 1 full cell wide after rounding: a 1-eighth left-anchored
//     block at the snapped start position.
//   - Trace duration is 0 (single instant): a centered `│` marker.
func renderBar(s *span, t *trace, barW int, opts *Options, depth int, errMsg string) string {
	if barW <= 0 {
		return ""
	}
	ansiOn := stripes.IsANSIEnabled(opts.Styles)
	fill := lipgloss.NewStyle()
	var barHex string
	if ansiOn {
		dr := t.serviceDepths[s.service]
		barHex = serviceColorAtDepth(s.service, depth, dr.min, dr.max)
		fill = lipgloss.NewStyle().Foreground(lipgloss.Color(barHex))
	}

	var overlay []rune
	if errMsg != "" {
		overlay = []rune("ERROR " + errMsg)
	}
	const errPrefixLen = 6 // "ERROR " — runes 0..5 render through styles.Deletion
	// The whole error line renders bold so the red "ERROR" prefix
	// and the trailing message read as a single emphasised band
	// against the surrounding bars.
	errStyle := opts.Styles.Deletion.Bold(true)
	plain := lipgloss.NewStyle().Bold(true)
	// renderOverlayCell returns the styled overlay character at the
	// bar-column position c. onBar, when true, applies the bar's hue
	// as the cell's BACKGROUND so the bar shows through behind the
	// text. Returns ("", false) when the overlay has no character at
	// this position.
	renderOverlayCell := func(c int, onBar bool) (string, bool) {
		if c < 0 || c >= len(overlay) {
			return "", false
		}
		st := plain
		if c < errPrefixLen {
			st = errStyle
		}
		if onBar && ansiOn {
			st = st.Background(lipgloss.Color(barHex))
		}
		return st.Render(string(overlay[c])), true
	}

	dur := t.duration()
	if dur <= 0 {
		return tickRowWithOverlay(barW, barW/2, fill, overlay, errPrefixLen, errStyle, plain)
	}

	totalEighths := barW * 8
	scale := float64(totalEighths) / float64(dur)
	startE := int(float64(s.start.Sub(t.rangeStart)) * scale)
	endE := int(float64(s.end.Sub(t.rangeStart)) * scale)

	if s.end.Equal(s.start) {
		col := startE / 8
		if col < 0 {
			col = 0
		}
		if col >= barW {
			col = barW - 1
		}
		return tickRowWithOverlay(barW, col, fill, overlay, errPrefixLen, errStyle, plain)
	}

	if endE <= startE {
		endE = startE + 1
	}
	if startE < 0 {
		startE = 0
	}
	if endE > totalEighths {
		endE = totalEighths
	}
	if startE >= totalEighths {
		startE = totalEighths - 1
	}
	startE = (startE / 8) * 8 // snap left edge to whole cells

	var b strings.Builder
	for c := 0; c < barW; c++ {
		cellStart := c * 8
		cellEnd := cellStart + 8
		visStart := max0(startE - cellStart)
		visEnd := min8(endE - cellStart)
		visible := visEnd - visStart
		onBar := visible > 0 && cellEnd > startE && cellStart < endE

		if rendered, ok := renderOverlayCell(c, onBar); ok {
			b.WriteString(rendered)
			continue
		}

		switch {
		case cellEnd <= startE, cellStart >= endE && visible <= 0:
			b.WriteByte(' ')
		case visStart == 0 && visEnd == 8:
			b.WriteString(fill.Render("█"))
		case visStart == 0 && visEnd < 8:
			b.WriteString(fill.Render(leftBlock(visEnd)))
		default:
			// Sub-cell span (very short, fits inside one cell).
			b.WriteString(fill.Render("▏"))
		}
	}
	return b.String()
}

// tickRowWithOverlay renders the single-cell `│` marker used for
// zero-duration spans (and degenerate zero-duration traces) at
// column `col`, with the same error overlay support as the main
// bar. The overlay is left-aligned to column 0 of the bar column
// (not anchored to the tick), so errors stack vertically across
// the whole chart.
func tickRowWithOverlay(barW, col int, fill lipgloss.Style, overlay []rune, errPrefixLen int, errStyle, plain lipgloss.Style) string {
	var b strings.Builder
	for c := 0; c < barW; c++ {
		if c < len(overlay) {
			st := plain
			if c < errPrefixLen {
				st = errStyle
			}
			b.WriteString(st.Render(string(overlay[c])))
			continue
		}
		if c == col {
			b.WriteString(fill.Render("│"))
			continue
		}
		b.WriteByte(' ')
	}
	return b.String()
}

func max0(v int) int {
	if v < 0 {
		return 0
	}
	if v > 8 {
		return 8
	}
	return v
}

func min8(v int) int {
	if v < 0 {
		return 0
	}
	if v > 8 {
		return 8
	}
	return v
}

// leftBlock returns the LEFT n/8 block glyph (n in [1,8]).
// n == 0 returns a space.
func leftBlock(n int) string {
	switch n {
	case 0:
		return " "
	case 1:
		return "▏"
	case 2:
		return "▎"
	case 3:
		return "▍"
	case 4:
		return "▌"
	case 5:
		return "▋"
	case 6:
		return "▊"
	case 7:
		return "▉"
	}
	return "█"
}
