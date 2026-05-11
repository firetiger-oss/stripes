package table

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// fitToWidth truncates row cells so the rendered table fits within
// targetWidth visible columns. Headers are never truncated; the algorithm
// repeatedly shrinks the column with the most slack (current width minus
// the header width) by one character until the table fits, so the widest
// columns lose the most and narrow columns are left untouched whenever
// possible. Cells that exceed the new column width are trimmed to
// width-3 runes and suffixed with "...".
//
// The chrome overhead (cell padding and optional borders) is subtracted
// from targetWidth to obtain the content budget. When the budget is
// non-positive, or when headers alone exceed it, the rows are returned
// unmodified — fitting can't fix that case and we'd rather overflow than
// drop information.
func fitToWidth(headers []string, rows [][]string, targetWidth int, bordered bool) [][]string {
	n := len(headers)
	if n == 0 || targetWidth <= 0 {
		return rows
	}

	var chrome int
	if bordered {
		// 1 left + 1 right pad per cell, plus N+1 vertical borders.
		chrome = 3*n + 1
	} else {
		// Borderless: 2-char gap between adjacent cells, flush on outer
		// edges. (n-1) gaps total.
		chrome = 2 * (n - 1)
		if chrome < 0 {
			chrome = 0
		}
	}
	budget := targetWidth - chrome
	if budget <= 0 {
		return rows
	}

	widths := make([]int, n)
	mins := make([]int, n)
	for i, h := range headers {
		w := lipgloss.Width(h)
		widths[i] = w
		mins[i] = w
	}
	for _, r := range rows {
		for i := 0; i < n && i < len(r); i++ {
			if w := lipgloss.Width(r[i]); w > widths[i] {
				widths[i] = w
			}
		}
	}

	total := 0
	for _, w := range widths {
		total += w
	}
	if total <= budget {
		return rows
	}

	newWidths := make([]int, n)
	copy(newWidths, widths)
	overage := total - budget
	for overage > 0 {
		// Pick the column with the most slack. Stable tie-break on the
		// lowest index gives reproducible output when slacks are equal.
		bestIdx := -1
		bestSlack := 0
		for i := range newWidths {
			slack := newWidths[i] - mins[i]
			if slack > bestSlack {
				bestSlack = slack
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			break // every column is at its header-width minimum
		}
		newWidths[bestIdx]--
		overage--
	}

	out := make([][]string, len(rows))
	for ri, r := range rows {
		nr := make([]string, len(r))
		for i, c := range r {
			if i < n && lipgloss.Width(c) > newWidths[i] {
				nr[i] = truncate(c, newWidths[i])
			} else {
				nr[i] = c
			}
		}
		out[ri] = nr
	}
	return out
}

// truncate shortens s to at most width visual columns, appending "..." when
// there is room, or a hard cut otherwise.
func truncate(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	const ellipsis = "..."
	if width <= len(ellipsis) {
		return cutRunes(s, width)
	}
	return cutRunes(s, width-len(ellipsis)) + ellipsis
}

// cutRunes returns the first n runes of s. It approximates visual width as
// one column per rune, which is correct for ASCII / most Latin scripts.
// CJK and emoji content may render slightly wider than expected.
func cutRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	var b strings.Builder
	b.Grow(n)
	count := 0
	for _, r := range s {
		if count >= n {
			break
		}
		b.WriteRune(r)
		count++
	}
	return b.String()
}
