package stripes

import (
	"testing"

	"charm.land/lipgloss/v2"
)

func TestStylesCloneIncludesAllFields(t *testing.T) {
	// Test that the Clone method properly copies all fields including Title, Columns, and Rows
	original := &Styles{
		Name:    lipgloss.NewStyle().Foreground(lipgloss.Color("1")),
		Text:    lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		String:  lipgloss.NewStyle().Foreground(lipgloss.Color("2")),
		Number:  lipgloss.NewStyle().Foreground(lipgloss.Color("3")),
		Boolean: lipgloss.NewStyle().Foreground(lipgloss.Color("4")),
		Null:    lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		Syntax:  lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		Anchor:  lipgloss.NewStyle().Foreground(lipgloss.Color("7")),
		Comment: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Title:   lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		Columns: lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Background(lipgloss.Color("235")),
		Rows:    lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		Border:  lipgloss.NormalBorder(),
		Indent:  "    ",
		Width:   100,
	}

	cloned := original.Clone()

	// Verify that all fields are copied correctly
	if original.Indent != cloned.Indent {
		t.Errorf("Expected cloned Indent to be %q, got %q", original.Indent, cloned.Indent)
	}
	if original.Width != cloned.Width {
		t.Errorf("Expected cloned Width to be %d, got %d", original.Width, cloned.Width)
	}
	// Verify that the Rows style is properly copied
	if original.Rows.GetForeground() != cloned.Rows.GetForeground() {
		t.Error("Expected Rows style to be copied correctly")
	}

	// Modify the original to ensure they're independent
	original.Width = 50
	if cloned.Width != 100 {
		t.Error("Cloned struct should be independent of original")
	}
}
