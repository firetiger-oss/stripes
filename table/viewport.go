package table

import "strings"

const (
	defaultSelectedIndicator = "❯"
	scrollbarTrack           = "│"
	scrollbarThumb           = "▌"
)

// decorate prepends a one-cell selector gutter and/or appends a one-cell
// scrollbar to the rendered table.
//
// Layout assumption: the lipgloss-rendered table is one line per visible
// row, preceded by a single header line. This holds in borderless mode (the
// package default). When WithBorder is set, lipgloss inserts additional
// border lines; v1 treats them like data rows — the gutter/scrollbar floats
// next to the border, which is acceptable as a first pass.
//
// totalRows is the count BEFORE viewport cropping; viewportTop is the
// clamped value computed by render(). When neither feature is in use the
// output is returned unchanged.
func decorate(rendered string, totalRows int, viewportTop int, opts *Options) string {
	hasSelector := opts.RowSelector != nil
	height := opts.ViewportHeight
	hasScrollbar := height > 0 && totalRows > height
	if !hasSelector && !hasScrollbar {
		return rendered
	}

	indicator := opts.SelectedIndicator
	if indicator == "" {
		indicator = defaultSelectedIndicator
	}

	var thumbStart, thumbSize int
	if hasScrollbar {
		thumbStart, thumbSize = scrollbarThumbBounds(totalRows, height, viewportTop)
	}

	lines := strings.Split(rendered, "\n")
	var b strings.Builder
	b.Grow(len(rendered) + 2*len(lines))
	for i, line := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		if hasSelector {
			if i == 0 {
				b.WriteByte(' ')
			} else if opts.RowSelector(viewportTop + i - 1) {
				b.WriteString(indicator)
			} else {
				b.WriteByte(' ')
			}
		}
		b.WriteString(line)
		if hasScrollbar {
			if i == 0 {
				b.WriteString(scrollbarTrack)
			} else if vis := i - 1; vis >= thumbStart && vis < thumbStart+thumbSize {
				b.WriteString(scrollbarThumb)
			} else {
				b.WriteString(scrollbarTrack)
			}
		}
	}
	return b.String()
}

// scrollbarThumbBounds returns the [start, start+size) range (in
// visible-row coordinates) covered by the scrollbar thumb. Caller must
// have already established that total > height > 0.
func scrollbarThumbBounds(total, height, top int) (start, size int) {
	size = (height*height + total/2) / total // round
	if size < 1 {
		size = 1
	}
	if size > height {
		size = height
	}
	start = top * height / total // floor
	if start+size > height {
		start = height - size
	}
	if start < 0 {
		start = 0
	}
	return start, size
}
