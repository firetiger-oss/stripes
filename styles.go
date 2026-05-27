package stripes

import (
	"io"
	"strings"

	"charm.land/lipgloss/v2"
)

// Renderer writes styled output for a single input format. All format
// functions in this package (JSON, YAML, XML, ...) match this signature.
type Renderer func(io.Writer, io.Reader, *Styles)

// IsANSIEnabled reports whether styles emit ANSI escape codes. It samples
// a few styles that all renderers exercise; if any of them produces an
// escape byte, color output is considered enabled. Used by renderers that
// switch between styled and plain output paths (notably markdown's
// fenced-code blocks and source-code highlighting).
func IsANSIEnabled(s *Styles) bool {
	return strings.ContainsRune(s.Title.Render("x"), 0x1b) ||
		strings.ContainsRune(s.Syntax.Render("x"), 0x1b) ||
		strings.ContainsRune(s.Anchor.Render("x"), 0x1b)
}

// Styles defines the styling configuration for rendering various data types
type Styles struct {
	Name       lipgloss.Style
	Text       lipgloss.Style
	String     lipgloss.Style
	Number     lipgloss.Style
	Boolean    lipgloss.Style
	Null       lipgloss.Style
	Syntax     lipgloss.Style
	Code       lipgloss.Style
	Anchor     lipgloss.Style
	Comment    lipgloss.Style
	Title      lipgloss.Style
	LineNumber lipgloss.Style
	Insertion  lipgloss.Style
	Deletion   lipgloss.Style
	Heading    [6]lipgloss.Style
	Columns    lipgloss.Style
	Rows       lipgloss.Style
	Border     lipgloss.Border
	Indent     string
	Width      int

	// CodeStyle names the chroma style used for syntax highlighting in
	// code blocks (see github.com/alecthomas/chroma/v2/styles). Empty
	// falls back to "github-dark".
	CodeStyle string

	// Verbose toggles expanded output in renderers that have a compact
	// default and an attribute/event-bearing detail view. Currently
	// consulted by stripes/trace to reveal span attributes, events, and
	// status messages under each waterfall row. Other renderers ignore
	// it.
	Verbose bool

	// SourceName carries the display name of the input being rendered
	// (typically a filename) so renderers can include it in
	// out-of-band protocol fields. Currently consulted by
	// stripes/image to populate the iTerm2 inline-image protocol's
	// name= header so terminal prompts identify the file by name
	// instead of "Unnamed file". Empty for stdin or when the caller
	// has no name to attribute. Other renderers ignore it.
	SourceName string

	// ImageFetcher resolves a markdown image reference to a stream of
	// bytes plus a content type. It is called by stripes/markdown when
	// an image-only paragraph is encountered and the reference is not
	// a data: URI. nil disables remote-image rendering — non-data
	// references fall back to the textual "[image] alt (dest)"
	// placeholder. Callers must close the returned reader.
	ImageFetcher func(ref string) (io.ReadCloser, string, error)
}

// Clone creates a copy of the Styles struct
func (s *Styles) Clone() *Styles {
	clone := *s
	return &clone
}

// DefaultStyles provides a grayscale styling theme using shades of grey, dimming, and bold
var DefaultStyles = &Styles{
	Name:       lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Bold(true),
	Text:       lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	String:     lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	Number:     lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
	Boolean:    lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
	Null:       lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
	Syntax:     lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true),
	Code:       lipgloss.NewStyle().Foreground(lipgloss.Color("183")),
	Anchor:     lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
	Comment:    lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true),
	Title:      lipgloss.NewStyle().Foreground(lipgloss.Color("7")).Bold(true),
	LineNumber: lipgloss.NewStyle().Foreground(lipgloss.Color("244")),
	Insertion:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
	Deletion:   lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
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
