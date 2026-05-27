package log

import (
	"charm.land/lipgloss/v2"
	"github.com/firetiger-oss/stripes"
)

// ClassifyLevel is an alias for [ClassifySeverity], named for the
// text-log side of the package where "level" is the convention.
// Both names live on the same severity classifier.
func ClassifyLevel(s string) SeverityClass { return ClassifySeverity(s) }

// The Stylexxx helpers return the lipgloss styles applied by [Row]
// to each column. Centralised here so any new text log format gets
// the same visual language without re-implementing colour choices:
//
//   - Ts:       dim grey timestamp
//   - Metadata: purple — format-specific identity (logger, tag, status…)
//   - Text:     default foreground — the message body
//   - Key:      bright blue — attribute name
//   - Syntax:   bold white — the "=" separator
//   - Val:      green — attribute value
//   - Dim:      comment / faint — used for stack traces and continuations

func StyleTs(styles *stripes.Styles) lipgloss.Style       { return styles.Comment }
func StyleMetadata(styles *stripes.Styles) lipgloss.Style { return styles.Code }
func StyleText(styles *stripes.Styles) lipgloss.Style     { return styles.Text }
func StyleKey(styles *stripes.Styles) lipgloss.Style      { return styles.Anchor }
func StyleSyntax(styles *stripes.Styles) lipgloss.Style   { return styles.Syntax }
func StyleVal(styles *stripes.Styles) lipgloss.Style      { return styles.String }
func StyleDim(styles *stripes.Styles) lipgloss.Style      { return styles.Comment }
