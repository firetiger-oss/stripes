package log

import (
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

// Row is the unified shape every text log renderer builds. Format
// implementations parse one input line and fill in a Row; the shared
// renderer turns it into a small block:
//
//	yyyy/mm/dd hh:mm:ss.mmm LEVEL metadata message
//	  attr1      = value1
//	  attr2.key1 = value2
//
// When the row has no attributes the block is a single line — the
// per-record vertical cost is paid only when there's structured
// data to show. Columns are separated by a single space. The
// timestamp and (when [LineFormat.HasLevel] is true) level columns
// have fixed widths, but metadata is variable-width: padding it
// pushes the message column too far right and hurts readability
// more than it helps once the next-line attrs anchor each record.
type Row struct {
	// Timestamp is the raw timestamp captured from the line. It is
	// passed through [NormalizeTimestamp] before rendering so every
	// format ends up in the canonical "yyyy/mm/dd hh:mm:ss.mmm"
	// shape in local time.
	Timestamp string

	// Level is the raw severity token captured from the line (e.g.
	// "INFO", "WARNING", "crit"). [ClassifyLevel] maps it to a
	// fixed-width coloured label. Ignored when the format declares
	// HasLevel=false.
	Level string

	// Metadata is the format's "visual identity" slot — a
	// pre-formatted, pre-styled string the format builds. Plain
	// formats can call [StyleMetadata] for the default purple;
	// HTTP access logs compose styled segments (proto + status)
	// directly. Empty elides the slot entirely.
	Metadata string

	// Message is the pre-styled free-form body of the log record.
	// Plain formats can call [StyleText] for the default text
	// colour; HTTP access logs compose multi-coloured spans
	// directly (e.g. [styleRequestLine] colours the HTTP method
	// distinctively). Empty elides the slot.
	Message string

	// Attrs are key/value pairs that render indented under the
	// header line, alphabetically sorted with keys aligned on the
	// "=" separator. Insertion order is not preserved.
	Attrs []KV
}

// KV is one rendered attribute. Pre-formatted values land in Value
// as-is; rendering applies no quoting or escaping, so the format
// owns its value presentation.
type KV struct{ Key, Value string }

// renderRow returns the multi-line styled block for r, sized
// according to lf (level column inclusion). No trailing newline —
// the caller appends one so the stream stays one line per Write.
func renderRow(r Row, lf LineFormat, styles *stripes.Styles) string {
	var b strings.Builder

	// Header line. Columns separated by a single space; the
	// fixed-width slots (timestamp 23, level 4) carry their own
	// internal padding so a stream's level column stays aligned.
	// Metadata is variable-width — see the rationale on [Row].
	ts := NormalizeTimestamp(r.Timestamp)
	b.WriteString(StyleTs(styles).Render(padRight(ts, canonicalTimestampWidth)))
	b.WriteByte(' ')

	if lf.HasLevel {
		sev := ClassifyLevel(r.Level)
		if sev == 0 {
			// Unknown / missing → assume INFO. A green INFO for
			// every level-less line is more readable than a blank
			// slot, and it makes the rare WARN/ERRO line in an
			// otherwise level-less stream pop visually.
			sev = SevInfo
		}
		b.WriteString(sev.Style(styles).Render(sev.Label()))
		b.WriteByte(' ')
	}

	if r.Metadata != "" {
		// Metadata is rendered as-is — the format owns its styling
		// (StyleMetadata for the default purple, custom segments
		// for HTTP access logs, etc.). No padding: forcing a
		// fixed-width metadata column pushes the message column so
		// far right that the table-look stops aiding readability
		// and starts hurting it, especially since the next-line
		// attrs already anchor each record visually.
		b.WriteString(r.Metadata)
		b.WriteByte(' ')
	}

	if r.Message != "" {
		// Message is pre-styled by the format — see the [Row.Message]
		// doc. Multi-line content (folded Go panics, JSON-embedded
		// newlines, exception traces) is laid out verbatim: no
		// wrapper indent, so the content's own indentation (tab-
		// indented stack frames, etc.) shows through unchanged.
		// The next record's timestamp at column 0 still acts as a
		// clear record boundary.
		writeMultilineRaw(&b, r.Message, "")
	}

	// Attribute lines.
	if len(r.Attrs) > 0 {
		attrs := append([]KV(nil), r.Attrs...)
		sort.SliceStable(attrs, func(i, j int) bool { return attrs[i].Key < attrs[j].Key })
		// Compute the key column width, capped so a single
		// pathologically-long key doesn't push every value off the
		// screen.
		const maxKeyWidth = 24
		keyW := 0
		for _, kv := range attrs {
			if w := ansi.StringWidth(kv.Key); w > keyW {
				keyW = w
			}
		}
		if keyW > maxKeyWidth {
			keyW = maxKeyWidth
		}
		// Continuation indent for multi-line values: repeats the
		// "│ " at the same column as the first line so the bar
		// runs uninterrupted from the first value line to the
		// last, bracketing the whole multi-line value.
		valCont := strings.Repeat(" ", 2+keyW+1) + StyleSyntax(styles).Bold(false).Render("│") + " "
		for _, kv := range attrs {
			b.WriteByte('\n')
			b.WriteString("  ")
			b.WriteString(StyleKey(styles).Render(padRight(kv.Key, keyW)))
			b.WriteString(" ")
			b.WriteString(StyleSyntax(styles).Bold(false).Render("│"))
			b.WriteString(" ")
			writeMultilineValue(&b, kv.Value, StyleVal(styles), valCont)
		}
	}

	return b.String()
}

// writeMultilineRaw writes a pre-styled (potentially multi-line)
// string, inserting indent before every line after the first.
// Used for [Row.Message]: the format already styled it, so the
// renderer's job is just continuation indentation.
func writeMultilineRaw(b *strings.Builder, s, indent string) {
	first := true
	for {
		i := strings.IndexByte(s, '\n')
		if i < 0 {
			if !first {
				b.WriteString(indent)
			}
			b.WriteString(s)
			return
		}
		if !first {
			b.WriteString(indent)
		}
		b.WriteString(s[:i])
		b.WriteByte('\n')
		s = s[i+1:]
		first = false
	}
}

// writeMultilineValue writes value through style, splitting on
// '\n' and indenting continuation lines to indent. Each line is
// styled independently so SGR escapes don't bleed across newlines.
func writeMultilineValue(b *strings.Builder, value string, style lipgloss.Style, indent string) {
	if !strings.ContainsRune(value, '\n') {
		b.WriteString(style.Render(value))
		return
	}
	first := true
	for _, line := range strings.Split(value, "\n") {
		if !first {
			b.WriteByte('\n')
			b.WriteString(indent)
		}
		b.WriteString(style.Render(line))
		first = false
	}
}

// padRight pads s with spaces to width n. Strings already at or
// beyond n are returned unchanged. Width is measured in terminal
// columns (so wide runes count as 2) for correct alignment.
func padRight(s string, n int) string {
	w := ansi.StringWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}
