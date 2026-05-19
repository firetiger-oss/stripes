package cobra

import (
	"charm.land/lipgloss/v2"

	"github.com/firetiger-oss/stripes"
)

// Styles defines the styling configuration for cobra help, usage, and error
// output. Each field is applied to a specific token type when rendering.
type Styles struct {
	Title       lipgloss.Style // section headings: "Usage:", "Flags:", etc.
	Program     lipgloss.Style // program name in the usage line
	Command     lipgloss.Style // subcommand names in the commands list
	Flag        lipgloss.Style // flag tokens like --verbose / -v
	Argument    lipgloss.Style // <file>, [flags], placeholders
	Description lipgloss.Style // descriptions / Long text
	Default     lipgloss.Style // "(default 5)" trailing fragments
	Example     lipgloss.Style // example command lines
	Error       lipgloss.Style // "Error:" prefix
	Hint        lipgloss.Style // "Try --help for usage." footer
	Indent      string
	Width       int
}

// Clone creates a copy of the Styles struct.
func (s *Styles) Clone() *Styles {
	clone := *s
	return &clone
}

// DefaultStyles reuses fields of [stripes.DefaultStyles] so the two palettes
// cannot drift. lipgloss.Style is a value type, so the assignments below are
// independent copies — later mutation of stripes.DefaultStyles does not
// affect this instance.
var DefaultStyles = &Styles{
	Title:       stripes.DefaultStyles.Name,
	Program:     stripes.DefaultStyles.Name,
	Command:     stripes.DefaultStyles.Anchor,
	Flag:        stripes.DefaultStyles.String,
	Argument:    lipgloss.NewStyle().Foreground(lipgloss.Color("229")),
	Description: lipgloss.NewStyle(),
	Default:     stripes.DefaultStyles.Comment,
	Example:     stripes.DefaultStyles.Code,
	Error:       lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true),
	Hint:        stripes.DefaultStyles.Comment,
	Indent:      stripes.DefaultStyles.Indent,
	Width:       stripes.DefaultStyles.Width,
}
