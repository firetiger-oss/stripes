// Package table renders typed iterators of struct values as styled
// CLI tables.
//
// Each column is derived by reflection from an exported struct field of
// the type parameter T: the header from the field's name (CamelCase split
// and uppercased, or overridden via a `table:"NAME"` struct tag), and the
// cell formatter from the field's type. Numeric fields are right-aligned;
// time.Time and time.Duration get dedicated human-friendly formats; any
// type implementing encoding.TextMarshaler, fmt.Formatter, or fmt.Stringer
// is honoured. Tag modifiers (`table:",bytes"`, `table:",%"`, `table:",%%"`) pin a
// specific formatter and alignment for cases the type system can't infer.
//
// The API mirrors what callers usually want: Write for streaming an
// iter.Seq2 of values + errors to an io.Writer, Format for a quick string,
// and NewWriter / NewFormatter for the precomputed-schema forms used in
// hot loops.
package table

import (
	"fmt"
	"io"
	"iter"
	"time"

	"github.com/charmbracelet/lipgloss"
	lipglosstable "github.com/charmbracelet/lipgloss/table"
	"github.com/firetiger-oss/stripes"
)

// Writer renders an iter.Seq2 of T-values (with optional per-row errors)
// into the destination io.Writer.
type Writer[T any] func(io.Writer, iter.Seq2[T, error]) error

// Formatter renders an iter.Seq of T-values into a string.
type Formatter[T any] func(iter.Seq[T]) string

// Options controls table rendering.
type Options struct {
	// Styles supplies colours and padding used by the underlying lipgloss
	// table. Defaults to stripes.DefaultStyles.
	Styles *stripes.Styles

	// Now, when non-nil, switches time.Time fields to relative rendering
	// ("5m ago", "3h ago", "5d ago") anchored at the returned instant.
	// When nil (the default) time.Time renders as an absolute timestamp.
	Now func() time.Time

	// Border draws table borders using the supplied lipgloss.Border
	// (e.g. lipgloss.NormalBorder(), lipgloss.RoundedBorder()). The zero
	// value renders borderless — only padding separates cells.
	Border lipgloss.Border

	// Columns supplies explicit column metadata. Required when T is a
	// slice or array (untyped rows); rejected when T is a struct, where
	// the schema is derived from struct fields and tags instead.
	Columns []Column

	// ColumnStyle, when non-nil, is called once per data cell. The
	// returned lipgloss.Style is composed with Styles.Rows via Inherit:
	// fields set on the returned style win, defaults fill the rest. col
	// is the zero-based column index; val is the post-width-fit,
	// post-internal-colorize cell text.
	ColumnStyle func(col int, val string) lipgloss.Style

	// HeaderStyle, when non-nil, is called once per header cell. The
	// returned lipgloss.Style is composed with Styles.Columns via
	// Inherit: fields set on the returned style win, defaults (e.g.
	// bold) fill the rest. col is the zero-based column index; val is
	// the header text.
	HeaderStyle func(col int, val string) lipgloss.Style

	// RowStyle, when non-nil, is called once per data row before any
	// per-cell ColumnStyle is applied. The returned lipgloss.Style is
	// composed with Styles.Rows via Inherit. row is the zero-based data
	// row index (the header row is not counted).
	RowStyle func(row int) lipgloss.Style

	// RowSelector, when non-nil, draws a one-cell gutter on the left:
	// rows for which the predicate returns true render
	// SelectedIndicator, other rows and the header render a single
	// space. The `row` argument is the absolute row index (unaffected
	// by ViewportTop).
	RowSelector func(row int) bool

	// SelectedIndicator is the gutter glyph drawn for rows matched by
	// RowSelector. Empty means use the package default ("❯").
	SelectedIndicator string

	// ViewportHeight, when > 0, restricts rendering to that many rows
	// starting at ViewportTop. When totalRows > ViewportHeight a
	// one-cell scrollbar is drawn on the right. Zero disables the
	// viewport — all rows render.
	ViewportHeight int

	// ViewportTop is the absolute row index of the first visible row
	// when ViewportHeight > 0. Clamped to [0, max(0, totalRows-height)].
	ViewportTop int
}

// Column describes one column when the row type isn't a struct. Each
// non-struct row (e.g. []string, []float64, []any) needs the caller to
// provide the column count and headers up front; cell formatters are
// otherwise derived from the element type or — for []any — from the
// dynamic type of each cell.
type Column struct {
	// Header is the column heading. It is rendered as-is; no case
	// transformation is applied (unlike struct-field-derived headers).
	Header string

	// Modifier optionally pins a formatter, mirroring the struct-tag
	// modifiers: "bytes", "count", "%", "%%", or "" to use the element type's
	// default formatter.
	Modifier string

	// Suffix is appended to every non-empty formatted cell, mirroring the
	// optional literal suffix in struct tags (e.g. `table:",count,/s"`).
	// Empty cells are left empty so column alignment is preserved.
	Suffix string
}

// Option configures Options.
type Option func(*Options)

// WithStyles overrides the rendering styles.
func WithStyles(s *stripes.Styles) Option {
	return func(o *Options) { o.Styles = s }
}

// WithNow enables relative-time rendering for time.Time fields, using the
// supplied clock as "now". Pass time.Now in production; pin a fixed
// time.Time in tests.
func WithNow(now func() time.Time) Option {
	return func(o *Options) { o.Now = now }
}

// WithBorder enables a border around and within the table using the
// supplied lipgloss.Border (e.g. lipgloss.NormalBorder(),
// lipgloss.RoundedBorder()). Tables are borderless by default.
func WithBorder(b lipgloss.Border) Option {
	return func(o *Options) { o.Border = b }
}

// WithColumns sets explicit column metadata, required when the row type T
// is a slice or array (untyped rows). It is rejected at schema-build time
// when T is a struct.
func WithColumns(cols ...Column) Option {
	return func(o *Options) { o.Columns = cols }
}

// WithHeaders is sugar for WithColumns when only headers are needed —
// every cell's formatter is then derived from the row element type.
func WithHeaders(names ...string) Option {
	cols := make([]Column, len(names))
	for i, n := range names {
		cols[i].Header = n
	}
	return WithColumns(cols...)
}

// WithColumnStyle registers a per-cell style callback for data rows.
// fn is invoked for every data cell with its zero-based column index
// and rendered text; the returned style composes with Styles.Rows (and
// any RowStyle) via Inherit, so fields the caller sets win and unset
// fields fall back to the defaults.
func WithColumnStyle(fn func(col int, val string) lipgloss.Style) Option {
	return func(o *Options) { o.ColumnStyle = fn }
}

// WithHeaderStyle registers a per-cell style callback for header
// cells. fn is invoked for every header cell with its zero-based
// column index and header text; the returned style composes with
// Styles.Columns via Inherit.
func WithHeaderStyle(fn func(col int, val string) lipgloss.Style) Option {
	return func(o *Options) { o.HeaderStyle = fn }
}

// WithRowStyle registers a per-row style callback. fn is invoked once
// per data row with its zero-based index (the header is not counted);
// the returned style composes with Styles.Rows via Inherit and runs
// before any ColumnStyle.
func WithRowStyle(fn func(row int) lipgloss.Style) Option {
	return func(o *Options) { o.RowStyle = fn }
}

// WithRowSelector sets the row-selection predicate. When non-nil it enables
// the one-cell left gutter: matching rows render the indicator (default
// "❯", overridable via WithSelectedIndicator), the header and non-matching
// rows render a single space.
//
// The `row` argument is the absolute index into the input sequence,
// unaffected by WithViewport's top offset. Compose multiple conditions
// inside the predicate when needed.
func WithRowSelector(fn func(row int) bool) Option {
	return func(o *Options) {
		o.RowSelector = fn
	}
}

// WithSelectedIndicator sets the gutter glyph for rows matched by the
// WithRowSelector predicate. Defaults to "❯". Must be a single visual cell
// (e.g. "❯", "▶", "→"); multi-cell strings will misalign columns.
func WithSelectedIndicator(s string) Option {
	return func(o *Options) { o.SelectedIndicator = s }
}

// WithViewport restricts rendering to `height` rows starting at row `top`
// (0-indexed, absolute). When totalRows > height a one-cell scrollbar is
// drawn on the right of every line (track "│", thumb "▌") with the thumb's
// position computed from (top, height, totalRows). When the data fits,
// no scrollbar is drawn. height must be >= 1; top is clamped to
// [0, max(0, totalRows-height)].
func WithViewport(height, top int) Option {
	return func(o *Options) {
		o.ViewportHeight = height
		o.ViewportTop = top
	}
}

func resolveOptions(opts []Option) Options {
	out := Options{Styles: stripes.DefaultStyles}
	for _, opt := range opts {
		opt(&out)
	}
	if out.Styles == nil {
		out.Styles = stripes.DefaultStyles
	}
	return out
}

// Write renders seq into w. Equivalent to NewWriter[T](opts...)(w, seq).
func Write[T any](w io.Writer, seq iter.Seq2[T, error], opts ...Option) error {
	return NewWriter[T](opts...)(w, seq)
}

// Format renders seq into a string. Equivalent to NewFormatter[T](opts...)(seq).
func Format[T any](seq iter.Seq[T], opts ...Option) string {
	return NewFormatter[T](opts...)(seq)
}

// NewWriter precomputes the column schema for T and returns a Writer that
// reuses it across calls.
func NewWriter[T any](opts ...Option) Writer[T] {
	o := resolveOptions(opts)
	sch, schemaErr := buildSchema[T](&o)
	return func(w io.Writer, seq iter.Seq2[T, error]) error {
		if schemaErr != nil {
			return schemaErr
		}
		rows := make([][]string, 0, 16)
		for v, err := range seq {
			if err != nil {
				return err
			}
			rows = append(rows, sch.formatRow(v))
		}
		_, err := io.WriteString(w, sch.render(rows, &o))
		return err
	}
}

// NewFormatter precomputes the column schema for T and returns a Formatter
// that reuses it across calls. If the schema is invalid (non-struct T,
// unknown tag modifier, ...), the error message is returned as the table
// body — the Formatter signature has no other channel.
func NewFormatter[T any](opts ...Option) Formatter[T] {
	o := resolveOptions(opts)
	sch, schemaErr := buildSchema[T](&o)
	return func(seq iter.Seq[T]) string {
		if schemaErr != nil {
			return fmt.Sprintf("stripes/table: %v", schemaErr)
		}
		rows := make([][]string, 0, 16)
		for v := range seq {
			rows = append(rows, sch.formatRow(v))
		}
		return sch.render(rows, &o)
	}
}

func (s *schema) render(rows [][]string, opts *Options) string {
	styles := opts.Styles
	totalRows := len(rows)

	viewportTop := opts.ViewportTop
	if opts.ViewportHeight > 0 {
		if viewportTop < 0 {
			viewportTop = 0
		}
		if maxTop := totalRows - opts.ViewportHeight; maxTop < 0 {
			viewportTop = 0
		} else if viewportTop > maxTop {
			viewportTop = maxTop
		}
		end := viewportTop + opts.ViewportHeight
		if end > totalRows {
			end = totalRows
		}
		rows = rows[viewportTop:end]
	}

	t := lipglosstable.New()
	if opts.Border != (lipgloss.Border{}) {
		t = t.
			Border(opts.Border).
			BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240")))
	} else {
		t = t.
			BorderTop(false).
			BorderBottom(false).
			BorderLeft(false).
			BorderRight(false).
			BorderColumn(false).
			BorderRow(false).
			BorderHeader(false)
	}

	headers := make([]string, len(s.columns))
	for i, c := range s.columns {
		headers[i] = c.header
	}

	bordered := opts.Border != (lipgloss.Border{})

	// Apply per-column colorization (e.g. JSON token highlighting) before
	// width fitting. fitToWidth measures with lipgloss.Width, which strips
	// ANSI, so per-column budgets are still computed against visible width;
	// truncation in fit.go is ANSI-aware and emits a defensive SGR reset
	// before the ellipsis so styles never leak past the cut.
	for _, r := range rows {
		for i := 0; i < len(r) && i < len(s.columns); i++ {
			r[i] = s.columns[i].colorize(r[i])
		}
	}

	if styles != nil && styles.Width > 0 {
		rows = fitToWidth(headers, rows, styles.Width, bordered)
	}

	// Per-render alignment: start from each column's static alignment,
	// then for slice-row schemas let cell content upgrade left-aligned
	// columns to right-aligned when every non-empty cell looks numeric.
	// Mutating a local slice (instead of s.columns) keeps the cached
	// schema state read-only across NewWriter reuse.
	alignments := make([]align, len(s.columns))
	for i, c := range s.columns {
		alignments[i] = c.align
	}
	if s.shape == rowSlice {
		detectNumericAlignment(rows, alignments)
	}

	t = t.Headers(headers...)
	for _, r := range rows {
		t = t.Row(r...)
	}
	lastCol := len(s.columns) - 1
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		var base lipgloss.Style
		isHeader := row == lipglosstable.HeaderRow
		if isHeader {
			base = styles.Columns
			if opts.HeaderStyle != nil && col >= 0 && col < len(headers) {
				base = opts.HeaderStyle(col, headers[col]).Inherit(base)
			}
		} else {
			base = styles.Rows
			if opts.RowStyle != nil && row >= 0 {
				base = opts.RowStyle(row + viewportTop).Inherit(base)
			}
			if opts.ColumnStyle != nil && row >= 0 && row < len(rows) && col >= 0 && col < len(rows[row]) {
				base = opts.ColumnStyle(col, rows[row][col]).Inherit(base)
			}
		}
		// Padding strategy:
		//   - Bordered: 1-char pad on each side (between content and the
		//     surrounding │ characters).
		//   - Borderless: 2-char right-pad acts as the inter-column gap;
		//     the leftmost column has no left-pad and the rightmost no
		//     right-pad so the table is flush on both outer edges.
		var leftPad, rightPad int
		if bordered {
			leftPad, rightPad = 1, 1
		} else if col != lastCol {
			rightPad = 2
		}
		st := base.Padding(0, rightPad, 0, leftPad)
		// Headers always left-align; data cells follow the (possibly
		// content-detected) column alignment.
		if !isHeader && col >= 0 && col < len(alignments) && alignments[col] == alignRight {
			st = st.Align(lipgloss.Right)
		} else {
			st = st.Align(lipgloss.Left)
		}
		return st
	})
	return decorate(t.Render(), totalRows, viewportTop, opts)
}
