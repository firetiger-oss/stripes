// Package csv registers the CSV renderer with the stripes registry.
// Import for side effects to enable text/csv support:
//
//	import _ "github.com/firetiger-oss/stripes/csv"
package csv

import (
	"encoding/csv"
	"io"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "csv",
		ContentType: "text/csv",
		Extensions:  []string{".csv"},
		RendererFor: stripes.Simple(CSV),
	})
}

// CSV renders comma-separated values as a styled lipgloss table.
// Columns containing only numeric cells are right-aligned.
func CSV(w io.Writer, r io.Reader, styles *stripes.Styles) {
	width := styles.Width
	if width <= 0 {
		width = 80
	}
	printCSV(w, r, width, styles)
}

func printCSV(w io.Writer, r io.Reader, maxWidth int, styles *stripes.Styles) {
	csvReader := csv.NewReader(r)
	csvReader.TrimLeadingSpace = true

	records, err := csvReader.ReadAll()
	if err != nil {
		io.WriteString(w, "Error reading CSV: "+err.Error())
		return
	}

	if len(records) == 0 {
		io.WriteString(w, "Empty CSV")
		return
	}

	// Determine which columns are numeric by checking all data rows
	numCols := len(records[0])
	isNumericCol := make([]bool, numCols)

	for col := range numCols {
		// Check all data rows (skip header row 0)
		numeric := 0
		totalDataRows := len(records) - 1 // Exclude header row
		for row := 1; row < len(records); row++ {
			if col < len(records[row]) && stripes.IsNumeric(records[row][col]) {
				numeric++
			}
		}
		isNumericCol[col] = numeric == totalDataRows
	}

	// Create table using lipgloss table package - auto-size to content
	t := table.New().
		Border(styles.Border).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color("240")))

	// Add headers if we have records
	if len(records) > 0 {
		t = t.Headers(records[0]...)
	}

	// Add data rows
	for i := 1; i < len(records); i++ {
		t = t.Row(records[i]...)
	}

	// Apply styling using StyleFunc
	t = t.StyleFunc(func(row, col int) lipgloss.Style {
		var baseStyle lipgloss.Style
		// In lipgloss table, row -1 is header, row 0+ are data rows
		if row == -1 {
			// Header row
			baseStyle = styles.Columns
		} else {
			// Data rows
			baseStyle = styles.Rows
		}

		// Add padding and alignment
		style := baseStyle.Padding(0, 1)

		// Right-align numeric columns (except headers)
		if row >= 0 && col < len(isNumericCol) && isNumericCol[col] {
			style = style.Align(lipgloss.Right)
		} else {
			style = style.Align(lipgloss.Left)
		}

		return style
	})

	io.WriteString(w, t.Render())
}
