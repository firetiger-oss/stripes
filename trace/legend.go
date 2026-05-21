package trace

import (
	"bytes"
	"fmt"
	"io"
	"iter"
	"sort"
	"strings"
	"time"

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/table"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// serviceStats holds the per-service aggregates rendered in the
// trace header's Services legend.
type serviceStats struct {
	label   string        // namespace/name (or name alone)
	service string        // bare service.name — used to derive the gradient swatch
	time    time.Duration // wall-clock coverage: union length of [start, end] intervals
	spans   int
	errors  int
}

// computeServiceStats walks every span in t and produces one
// serviceStats per distinct service label, sorted by descending
// wall-clock coverage with a name tiebreak.
//
// "Time" is the wall-clock coverage of the service: the length of
// the union of all [start, end] intervals of the service's spans.
// Concurrent spans are merged so they don't double-count, which
// makes Ratio (time / trace.duration) always within [0, 100%]. A
// naïve sum-of-durations or sum-of-self-times can exceed the trace
// duration on parallel-heavy traces and produce >100% ratios.
func computeServiceStats(t *trace) []serviceStats {
	if len(t.spans) == 0 {
		return nil
	}

	type interval struct{ start, end time.Time }
	byLabel := make(map[string]*serviceStats, 4)
	intervals := make(map[string][]interval, 4)
	get := func(s *span) *serviceStats {
		lbl := s.label()
		ss, ok := byLabel[lbl]
		if !ok {
			ss = &serviceStats{label: lbl, service: s.service}
			byLabel[lbl] = ss
		}
		return ss
	}
	for _, s := range t.spans {
		ss := get(s)
		ss.spans++
		if s.pb.GetStatus().GetCode() == tracev1.Status_STATUS_CODE_ERROR {
			ss.errors++
		}
		intervals[ss.label] = append(intervals[ss.label], interval{start: s.start, end: s.end})
	}

	// Merge each service's intervals and sum the union length.
	for lbl, ivs := range intervals {
		sort.Slice(ivs, func(i, j int) bool { return ivs[i].start.Before(ivs[j].start) })
		var total time.Duration
		curStart := ivs[0].start
		curEnd := ivs[0].end
		for i := 1; i < len(ivs); i++ {
			if !ivs[i].start.After(curEnd) {
				if ivs[i].end.After(curEnd) {
					curEnd = ivs[i].end
				}
				continue
			}
			total += curEnd.Sub(curStart)
			curStart, curEnd = ivs[i].start, ivs[i].end
		}
		total += curEnd.Sub(curStart)
		byLabel[lbl].time = total
	}

	out := make([]serviceStats, 0, len(byLabel))
	for _, ss := range byLabel {
		out = append(out, *ss)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].time != out[j].time {
			return out[i].time > out[j].time
		}
		return out[i].label < out[j].label
	})
	return out
}

// maxLegendNameWidth caps the Name column so a pathologically long
// namespace/service.name pair can't blow out the table.
const maxLegendNameWidth = 40

// writeServicesLegend renders the `Services:` block via the
// stripes/table package. Each row carries the service swatch (a `█`
// in the service's hue, matching the bars below), the
// namespace/service.name identifier, the self-time aggregate, the
// share of trace duration as a Ratio, the span count and the error
// count. Rows are pre-sorted by descending self-time.
//
// The table renders borderless and is line-indented under the
// `Services:` heading so the block reads as part of the surrounding
// trace-header metadata.
func writeServicesLegend(w io.Writer, t *trace, opts *Options) error {
	stats := computeServiceStats(t)
	if len(stats) == 0 {
		return nil
	}

	ansiOn := stripes.IsANSIEnabled(opts.Styles)
	traceDur := t.duration()

	rows := make([][]string, 0, len(stats))
	for _, s := range stats {
		swatch := swatchGradient(s.service, ansiOn)
		ratio := "-"
		if traceDur > 0 {
			r := float64(s.time) / float64(traceDur) * 100
			switch {
			case r >= 10:
				ratio = fmt.Sprintf("%.0f%%", r)
			case r >= 1:
				ratio = fmt.Sprintf("%.1f%%", r)
			default:
				ratio = fmt.Sprintf("%.2f%%", r)
			}
		}
		rows = append(rows, []string{
			swatch,
			truncateWidth(s.label, maxLegendNameWidth),
			formatDurationAs(s.time, t.unit),
			ratio,
			fmt.Sprintf("%d", s.spans),
			fmt.Sprintf("%d", s.errors),
		})
	}

	if _, err := io.WriteString(w, strings.Repeat(" ", gutterLeft)+"Services:\n"); err != nil {
		return err
	}

	// Render the table into a buffer so we can left-indent every line
	// under the "Services:" heading. The table package emits a styled
	// borderless grid; the empty-header first column carries the
	// pre-coloured swatch glyph.
	seq := func(yield func([]string, error) bool) {
		for _, r := range rows {
			if !yield(r, nil) {
				return
			}
		}
	}
	var buf bytes.Buffer
	if err := table.Write[[]string](&buf, iter.Seq2[[]string, error](seq),
		table.WithHeaders("", "Name", "Time", "Ratio", "Spans", "Errors"),
		table.WithStyles(opts.Styles),
	); err != nil {
		return err
	}
	// The table package may emit the final row without a trailing
	// newline; we always want one so the bottom rule starts on its
	// own line. Split on '\n', skip empty trailing entries, and re-
	// add a newline after each non-empty line.
	tableIndent := strings.Repeat(" ", gutterLeft*2)
	for _, line := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if _, err := io.WriteString(w, tableIndent+line+"\n"); err != nil {
			return err
		}
	}
	return nil
}

// swatchGradient renders the 3-cell colour key shown in the
// Services legend: a `█` at depth-0 lightness (brightest), one at
// the base, and one at depth-max lightness (dimmest). The result
// mirrors the bright-to-dim gradient applied to the bars below, so
// the legend doubles as a key for both the service AND the depth
// fade.
//
// When ANSI is disabled the three cells are uncoloured and just
// read as `███` — still survives copy-paste; the gradient cue is
// the only thing lost.
func swatchGradient(serviceName string, ansiOn bool) string {
	const cells = "███"
	if !ansiOn {
		return cells
	}
	h, s, _ := serviceHSL(serviceName)
	bright := hslToHex(h, s, lightnessAtFade(0))
	mid := hslToHex(h, s, lightnessAtFade(0.5))
	dim := hslToHex(h, s, lightnessAtFade(1))
	render := func(hex, glyph string) string {
		return "\x1b[38;2;" + rgbANSI(hex) + "m" + glyph + "\x1b[m"
	}
	return render(bright, "█") + render(mid, "█") + render(dim, "█")
}

// rgbANSI extracts the "R;G;B" decimal triple from a "#rrggbb"
// truecolour hex string. Used to assemble SGR escape sequences
// without taking a lipgloss dependency at every call site.
func rgbANSI(hex string) string {
	if len(hex) != 7 || hex[0] != '#' {
		return "255;255;255"
	}
	hi := func(c byte) int {
		switch {
		case c >= '0' && c <= '9':
			return int(c - '0')
		case c >= 'a' && c <= 'f':
			return int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			return int(c-'A') + 10
		}
		return 0
	}
	r := hi(hex[1])*16 + hi(hex[2])
	g := hi(hex[3])*16 + hi(hex[4])
	b := hi(hex[5])*16 + hi(hex[6])
	return fmt.Sprintf("%d;%d;%d", r, g, b)
}
