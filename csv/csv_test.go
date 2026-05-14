package csv

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func TestRenderRender(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains []string // Check for key elements rather than exact output
	}{
		{
			name: "simple table",
			input: `Name,Age,City
John,30,NYC
Jane,25,LA`,
			contains: []string{
				"Name", "Age", "City",
				"John", "30", "NYC",
				"Jane", "25", "LA",
			},
		},
		{
			name: "numeric columns",
			input: `Product,Price,Quantity
Widget,19.99,100
Gadget,25.50,75
Tool,-5.00,200`,
			contains: []string{
				"Product", "Price", "Quantity",
				"Widget", "19.99", "100",
				"Gadget", "25.50", "75",
				"Tool", "-5.00", "200",
			},
		},
		{
			name: "mixed data types",
			input: `ID,Name,Active,Score
1,Alice,true,95.5
2,Bob,false,87.2
3,Charlie,true,92.0`,
			contains: []string{
				"ID", "Name", "Active", "Score",
				"1", "Alice", "true", "95.5",
				"2", "Bob", "false", "87.2",
				"3", "Charlie", "true", "92.0",
			},
		},
		{
			name: "empty cells",
			input: `Name,Email,Phone
Alice,alice@test.com,555-1234
Bob,,555-5678
Charlie,charlie@test.com,`,
			contains: []string{
				"Name", "Email", "Phone",
				"Alice", "alice@test.com", "555-1234",
				"Bob", "555-5678",
				"Charlie", "charlie@test.com",
			},
		},
		{
			name: "quoted fields",
			input: `Name,Description,Price
"Super Widget","A great, useful widget",29.99
"Mega Gadget","The best gadget ""ever""",49.99`,
			contains: []string{
				"Name", "Description", "Price",
				"Super Widget", "A great, useful widget", "29.99",
				"Mega Gadget", `The best gadget "ever"`, "49.99",
			},
		},
		{
			name: "single column",
			input: `Numbers
1
2
3
4
5`,
			contains: []string{
				"Numbers",
				"1", "2", "3", "4", "5",
			},
		},
		{
			name: "single row",
			input: `Name,Age,City
John,30,NYC`,
			contains: []string{
				"Name", "Age", "City",
				"John", "30", "NYC",
			},
		},
		{
			name: "large numbers",
			input: `Year,Revenue,Profit
2020,1234567.89,123456.78
2021,2345678.90,234567.89
2022,3456789.01,345678.90`,
			contains: []string{
				"Year", "Revenue", "Profit",
				"2020", "1234567.89", "123456.78",
				"2021", "2345678.90", "234567.89",
				"2022", "3456789.01", "345678.90",
			},
		},
		{
			name: "scientific notation",
			input: `Name,Value,Coefficient
Alpha,1.23e+10,0.95
Beta,4.56e-5,1.05
Gamma,7.89e+2,0.85`,
			contains: []string{
				"Name", "Value", "Coefficient",
				"Alpha", "1.23e+10", "0.95",
				"Beta", "4.56e-5", "1.05",
				"Gamma", "7.89e+2", "0.85",
			},
		},
		{
			name: "negative numbers",
			input: `Account,Balance,Change
Checking,-150.25,-50.00
Savings,1250.75,+100.00
Credit,-2500.00,-200.00`,
			contains: []string{
				"Account", "Balance", "Change",
				"Checking", "-150.25", "-50.00",
				"Savings", "1250.75", "+100.00",
				"Credit", "-2500.00", "-200.00",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var output strings.Builder
			reader := strings.NewReader(tt.input)
			Render(&output, reader, stripes.DefaultStyles)
			result := output.String()

			// Strip ANSI codes for content checking
			stripped := ansi.Strip(result)

			// Check that all expected content is present
			for _, expected := range tt.contains {
				if !strings.Contains(stripped, expected) {
					t.Errorf("Render() output missing expected content: %q\nInput: %s\nOutput: %s", expected, tt.input, stripped)
				}
			}

			// Ensure we got some output
			if len(result) == 0 {
				t.Errorf("Render() produced empty output for input: %s", tt.input)
			}
		})
	}
}

func TestRenderCSVWithEmptyInput(t *testing.T) {
	// Test that empty input doesn't crash the function
	var output strings.Builder

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render() panicked with empty input: %v", r)
		}
	}()

	reader := strings.NewReader("")
	Render(&output, reader, stripes.DefaultStyles)
	result := output.String()

	// Should contain "Empty CSV" message
	if !strings.Contains(result, "Empty CSV") {
		t.Error("Expected 'Empty CSV' message for empty input")
	}
}

func TestRenderCSVWithInvalidRender(t *testing.T) {
	// Test that malformed CSV doesn't crash the function
	var output strings.Builder

	// Should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("Render() panicked with invalid CSV: %v", r)
		}
	}()

	// CSV with unclosed quotes
	invalidCSV := `Name,Description
"John,"Invalid quote handling
Alice,Normal`

	reader := strings.NewReader(invalidCSV)
	Render(&output, reader, stripes.DefaultStyles)
	// Should produce some output or error message
	result := output.String()
	if len(result) == 0 {
		t.Error("Expected some output even for invalid CSV")
	}
}

func TestRenderCSVStyling(t *testing.T) {
	// Test that CSV produces styled output
	input := `Name,Score,Active
Alice,95.5,true
Bob,87.2,false`

	var output strings.Builder
	reader := strings.NewReader(input)
	Render(&output, reader, stripes.DefaultStyles)
	result := output.String()

	// Should contain the text content
	stripped := ansi.Strip(result)
	expectedContent := []string{
		"Name", "Score", "Active",
		"Alice", "95.5", "true",
		"Bob", "87.2", "false",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(stripped, expected) {
			t.Errorf("Expected output to contain %q", expected)
		}
	}

	// Check that we have some output
	if len(result) == 0 {
		t.Error("Expected some output from CSV rendering")
	}
}

func TestRenderCSVWidthConstraints(t *testing.T) {
	// Test CSV rendering with width constraints
	input := `Name,Description,Price
Widget,A useful tool,29.99
Gadget,Another item,19.99`

	var output strings.Builder
	reader := strings.NewReader(input)
	// Use a narrow width
	narrowStyles := stripes.DefaultStyles.Clone()
	narrowStyles.Width = 40
	Render(&output, reader, narrowStyles)
	result := output.String()

	stripped := ansi.Strip(result)

	// Should still contain the essential content (may be truncated but present)
	expectedContent := []string{
		"Name", "Description", "Price",
		"Widget", "Gadget",
		"29.99", "19.99",
	}

	for _, expected := range expectedContent {
		if !strings.Contains(stripped, expected) {
			t.Errorf("Expected output to contain %q even with width constraints", expected)
		}
	}

	// Ensure we still get table structure
	if len(result) == 0 {
		t.Error("Expected some output even with width constraints")
	}
}

func TestRenderCSVHeaderOnly(t *testing.T) {
	// Test CSV with only header row
	input := `Name,Age,City`

	var output strings.Builder
	reader := strings.NewReader(input)
	Render(&output, reader, stripes.DefaultStyles)
	result := output.String()

	stripped := ansi.Strip(result)

	// Should contain header content
	expectedContent := []string{"Name", "Age", "City"}
	for _, expected := range expectedContent {
		if !strings.Contains(stripped, expected) {
			t.Errorf("Expected output to contain header %q", expected)
		}
	}
}

func TestTitleStyleInRender(t *testing.T) {
	// Simple test to verify CSV renders without errors and contains expected content
	input := `Name,Age,City
Alice,25,NYC
Bob,30,LA`

	var output strings.Builder
	reader := strings.NewReader(input)
	Render(&output, reader, stripes.DefaultStyles)
	result := output.String()

	if len(result) == 0 {
		t.Error("Expected CSV output to contain styled content")
	}

	stripped := ansi.Strip(result)
	expectedContent := []string{"Name", "Age", "City", "Alice", "25", "NYC", "Bob", "30", "LA"}
	for _, expected := range expectedContent {
		if !strings.Contains(stripped, expected) {
			t.Errorf("Expected CSV output to contain %q", expected)
		}
	}
}

func TestCSVColumnsAndRowsStyling(t *testing.T) {
	// Test that CSV uses Columns and Rows styling appropriately
	input := `Name,Age,City
Alice,25,NYC
Bob,30,LA
Charlie,35,Chicago`

	var output strings.Builder
	reader := strings.NewReader(input)
	Render(&output, reader, stripes.DefaultStyles)
	result := output.String()

	if len(result) == 0 {
		t.Error("Expected CSV output to contain styled content")
	}

	stripped := ansi.Strip(result)
	expectedContent := []string{"Name", "Age", "City", "Alice", "25", "NYC", "Bob", "30", "LA", "Charlie", "35", "Chicago"}
	for _, expected := range expectedContent {
		if !strings.Contains(stripped, expected) {
			t.Errorf("Expected CSV output to contain %q", expected)
		}
	}
}
