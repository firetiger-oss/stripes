package stripes

import (
	"io"

	"github.com/charmbracelet/lipgloss"
)

// Renderer writes styled output for a single input format. All format
// functions in this package (JSON, YAML, XML, ...) match this signature.
type Renderer func(io.Writer, io.Reader, *Styles)

// Styles defines the styling configuration for rendering various data types
type Styles struct {
	Name    lipgloss.Style
	Text    lipgloss.Style
	String  lipgloss.Style
	Number  lipgloss.Style
	Boolean lipgloss.Style
	Null    lipgloss.Style
	Syntax  lipgloss.Style
	Code    lipgloss.Style
	Anchor  lipgloss.Style
	Comment lipgloss.Style
	Title   lipgloss.Style
	Heading [6]lipgloss.Style
	Columns lipgloss.Style
	Rows    lipgloss.Style
	Border  lipgloss.Border
	Indent  string
	Width   int

	// CodeStyle names the chroma style used for syntax highlighting in
	// code blocks (see github.com/alecthomas/chroma/v2/styles). Empty
	// falls back to "github-dark".
	CodeStyle string
}

// Clone creates a copy of the Styles struct
func (s *Styles) Clone() *Styles {
	clone := *s
	return &clone
}

// DefaultStyles provides a grayscale styling theme using shades of grey, dimming, and bold
var DefaultStyles = &Styles{
	Name:    lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	Text:    lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	String:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	Number:  lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	Boolean: lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	Null:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	Syntax:  lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true),
	Code:    lipgloss.NewStyle().Foreground(lipgloss.Color("183")),
	Anchor:  lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
	Comment: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true),
	Title:   lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true),
	Heading: [6]lipgloss.Style{
		lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true), // H1: bright blue (rule below)
		lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true), // H2: bright blue (underlined)
		lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true), // H3: bright blue (no underline)
		lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Bold(true),  // H4: blue
		lipgloss.NewStyle().Foreground(lipgloss.Color("4")),             // H5: blue
		lipgloss.NewStyle().Foreground(lipgloss.Color("4")).Faint(true), // H6: blue faint
	},
	Columns: lipgloss.NewStyle().Bold(true),
	Rows:    lipgloss.NewStyle(),
	Border:  lipgloss.NormalBorder(),
	Indent:  "  ",
	Width:   80,
}
