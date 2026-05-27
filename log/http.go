package log

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/firetiger-oss/stripes"
)

// styleProtoStatus renders the metadata cell for an HTTP access log
// row: a single bold "HTTP" marker followed by the status code
// coloured by class. The protocol-specific scheme from the log line
// (http, https, h2, ws…) is intentionally collapsed to one marker —
// HTTP-shaped lines are visually identified by the marker's
// presence, not by which sub-protocol they used. Status colouring
// (2xx green, 3xx cyan, 4xx yellow, 5xx red) carries the row's
// severity since the LEVEL column is omitted for HTTP formats.
func styleProtoStatus(_, status string, styles *stripes.Styles) string {
	if !stripes.IsANSIEnabled(styles) {
		return "HTTP " + status
	}
	bold := lipgloss.NewStyle().Bold(true)
	return bold.Render("HTTP") + " " + statusStyle(status).Render(status)
}

// statusStyle returns the lipgloss style for an HTTP status code,
// coloured by its class (1xx informational, 2xx success, 3xx
// redirect, 4xx client error, 5xx server error). Matches the
// severity-class palette so the eye reads the row's overall
// severity from either the LEVEL column or the status code.
func statusStyle(status string) lipgloss.Style {
	if status == "" || status == "-" {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Faint(true)
	}
	switch status[0] {
	case '1':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("14")).Bold(true) // cyan
	case '2':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Bold(true) // green
	case '3':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("6")).Bold(true) // cyan (3xx is informational-ish)
	case '4':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true) // yellow
	case '5':
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true) // red
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("7"))
}

// styleRequestLine renders an HTTP request line ("METHOD path HTTP/x.y")
// with the method coloured by [methodStyle], the path in the default
// text colour, and the protocol-version suffix dimmed so the eye
// lands on what the request is and where it points. Malformed
// inputs (missing parts) fall back to plain text. With ANSI styling
// disabled the line is returned verbatim.
func styleRequestLine(req string, styles *stripes.Styles) string {
	if !stripes.IsANSIEnabled(styles) {
		return req
	}
	parts := strings.SplitN(req, " ", 3)
	switch len(parts) {
	case 1:
		return StyleText(styles).Render(req)
	case 2:
		return methodStyle(parts[0]).Render(parts[0]) + " " +
			StyleText(styles).Render(parts[1])
	default:
		return methodStyle(parts[0]).Render(parts[0]) + " " +
			StyleText(styles).Render(parts[1]) + " " +
			StyleDim(styles).Render(parts[2])
	}
}

// methodStyle returns the lipgloss style for an HTTP method token.
// Colours are picked from the warm / purple half of the wheel so
// they never collide with the status-code palette (which uses green
// / yellow / red / cyan for 2xx / 4xx / 5xx / 1xx-3xx). Without
// this split, `200 GET` rendered as one solid green run instead of
// two distinguishable elements. HEAD / OPTIONS render dim because
// they're usually noise (CORS pre-flight, cache validation) the
// reader scans past.
func methodStyle(method string) lipgloss.Style {
	bold := lipgloss.NewStyle().Bold(true)
	switch method {
	case "GET":
		return bold.Foreground(lipgloss.Color("33")) // blue
	case "POST":
		return bold.Foreground(lipgloss.Color("208")) // orange
	case "PUT":
		return bold.Foreground(lipgloss.Color("165")) // magenta
	case "PATCH":
		return bold.Foreground(lipgloss.Color("99")) // purple
	case "DELETE":
		return bold.Foreground(lipgloss.Color("197")) // hot pink / magenta-red
	case "HEAD", "OPTIONS":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("244")).Faint(true)
	case "CONNECT", "TRACE":
		return bold.Foreground(lipgloss.Color("129")) // deep purple
	}
	return lipgloss.NewStyle()
}
