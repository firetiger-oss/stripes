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
	Anchor  lipgloss.Style
	Comment lipgloss.Style
	Title   lipgloss.Style
	Columns lipgloss.Style
	Rows    lipgloss.Style
	Border  lipgloss.Border
	Indent  string
	Width   int
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
	Anchor:  lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
	Comment: lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true),
	Title:   lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true),
	Columns: lipgloss.NewStyle().Bold(true),
	Rows:    lipgloss.NewStyle(),
	Border:  lipgloss.NormalBorder(),
	Indent:  "  ",
	Width:   80,
}
