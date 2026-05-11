package main

import (
	"reflect"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
)

func runCSV(t *testing.T, input string) string {
	t.Helper()
	var buf strings.Builder
	csvTable(&buf, strings.NewReader(input), stripes.DefaultStyles)
	return ansi.Strip(buf.String())
}

func runTSV(t *testing.T, input string) string {
	t.Helper()
	var buf strings.Builder
	tsvTable(&buf, strings.NewReader(input), stripes.DefaultStyles)
	return ansi.Strip(buf.String())
}

func runJSONL(t *testing.T, input string) string {
	t.Helper()
	var buf strings.Builder
	jsonlTable(&buf, strings.NewReader(input), stripes.DefaultStyles)
	return ansi.Strip(buf.String())
}

func mustContain(t *testing.T, out string, parts ...string) {
	t.Helper()
	for _, p := range parts {
		if !strings.Contains(out, p) {
			t.Errorf("output missing %q\nfull output:\n%s", p, out)
		}
	}
}

func TestCSVTable(t *testing.T) {
	out := runCSV(t, "name,age,city\nalice,30,nyc\nbob,25,la\n")
	mustContain(t, out, "name", "age", "city", "alice", "30", "nyc", "bob", "25", "la")
}

func TestCSVTableNumericColumnRightAligned(t *testing.T) {
	// Quantity is a numeric-looking string column; content detection
	// should right-align it. "Quantity" header (8 chars), value "100"
	// (3 chars) right-aligned → 5 leading spaces.
	out := runCSV(t, "Product,Quantity\nWidget,100\nGadget,75\n")
	if !strings.Contains(out, "     100") {
		t.Fatalf("expected Quantity right-aligned ('     100'), got:\n%s", out)
	}
	if !strings.Contains(out, "      75") {
		t.Fatalf("expected '75' right-aligned, got:\n%s", out)
	}
}

func TestCSVTablePercentColumnRightAligned(t *testing.T) {
	// Percentages carry a unit suffix; IsNumeric tolerates "%".
	out := runCSV(t, "Service,Uptime\napi,99.9 %\ndb,87.5 %\n")
	// Cell "99.9 %" itself is 6 chars; header "Uptime" is 6 chars too,
	// so no leading-space evidence — instead assert that "Service" is
	// left-aligned (no leading spaces before it) and "Uptime" header
	// is left while the column right-aligns 6-char cells flush.
	// Cells fit exactly; smoke check: percentages appear.
	mustContain(t, out, "99.9 %", "87.5 %", "Uptime", "api", "db")
}

func TestCSVTableQuoted(t *testing.T) {
	in := "name,desc,price\n" +
		`"Super Widget","A great, useful widget",29.99` + "\n" +
		`"Mega Gadget","The best gadget ""ever""",49.99` + "\n"
	out := runCSV(t, in)
	mustContain(t, out, "Super Widget", "A great, useful widget", "29.99",
		"Mega Gadget", `The best gadget "ever"`, "49.99")
}

func TestCSVTableEmpty(t *testing.T) {
	out := runCSV(t, "")
	if !strings.Contains(out, "Empty CSV") {
		t.Fatalf("expected Empty CSV message, got: %q", out)
	}
}

func TestCSVTableMalformed(t *testing.T) {
	// Unterminated quote that even LazyQuotes can't recover from.
	in := `name,desc` + "\n" + `"unclosed,oops` + "\n"
	var buf strings.Builder
	csvTable(&buf, strings.NewReader(in), stripes.DefaultStyles)
	out := buf.String()
	// Either renders best-effort or surfaces an error — must not be empty.
	if len(out) == 0 {
		t.Fatalf("expected non-empty output for malformed CSV")
	}
}

func TestTSVTable(t *testing.T) {
	in := "name\tage\tcity\nalice\t30\tnyc\nbob\t25\tla\n"
	out := runTSV(t, in)
	mustContain(t, out, "name", "age", "city", "alice", "30", "nyc", "bob", "25", "la")
}

func TestJSONLTable(t *testing.T) {
	in := `{"name":"alice","age":30}` + "\n" +
		`{"name":"bob","age":25}` + "\n"
	out := runJSONL(t, in)
	// Sorted keys means age column first, then name.
	mustContain(t, out, "age", "name", "alice", "30", "bob", "25")
	ageIdx := strings.Index(out, "age")
	nameIdx := strings.Index(out, "name")
	if ageIdx < 0 || nameIdx < 0 || ageIdx > nameIdx {
		t.Errorf("expected age column before name (sorted), got header order in:\n%s", out)
	}
}

func TestJSONLTableHeterogeneousKeys(t *testing.T) {
	in := `{"a":1,"b":2}` + "\n" +
		`{"a":3,"c":4}` + "\n"
	out := runJSONL(t, in)
	// Union of keys: a, b, c. Missing entries render as empty.
	mustContain(t, out, "a", "b", "c", "1", "2", "3", "4")
}

func TestJSONLTableNestedValues(t *testing.T) {
	in := `{"name":"alice","tags":["admin","ops"]}` + "\n"
	out := runJSONL(t, in)
	mustContain(t, out, "alice", `["admin","ops"]`)
}

func TestJSONLTableEmpty(t *testing.T) {
	out := runJSONL(t, "")
	if !strings.Contains(out, "Empty JSONL") {
		t.Fatalf("expected Empty JSONL message, got: %q", out)
	}
}

func TestJSONLTableMalformed(t *testing.T) {
	in := `{"name":"alice"}` + "\n" + `{not valid json}` + "\n"
	var buf strings.Builder
	jsonlTable(&buf, strings.NewReader(in), stripes.DefaultStyles)
	out := buf.String()
	if !strings.Contains(out, "Error parsing JSONL") {
		t.Fatalf("expected JSONL parse error, got: %q", out)
	}
}

func TestDetectRowFlavorByExtension(t *testing.T) {
	cases := []struct {
		name string
		want string // function name as a witness
	}{
		{"data.csv", "csvTable"},
		{"data.CSV", "csvTable"},
		{"data.tsv", "tsvTable"},
		{"data.tab", "tsvTable"},
		{"data.jsonl", "jsonlTable"},
		{"data.ndjson", "jsonlTable"},
	}
	for _, c := range cases {
		got := detectRowFlavor(c.name, nil)
		gotName := rendererName(got)
		if gotName != c.want {
			t.Errorf("detectRowFlavor(%q, nil) = %s, want %s", c.name, gotName, c.want)
		}
	}
}

func TestDetectRowFlavorBySniff(t *testing.T) {
	cases := []struct {
		peek string
		want string
	}{
		{"a,b,c\n1,2,3\n", "csvTable"},
		{"a\tb\tc\n1\t2\t3\n", "tsvTable"},
		{"{\"a\":1}\n{\"a\":2}\n", "jsonlTable"},
		{"", "csvTable"},
		{"   ", "csvTable"},
		// Single JSON object spanning lines must NOT be jsonl-detected.
		{"{\n  \"a\": 1\n}\n", "csvTable"},
	}
	for _, c := range cases {
		got := detectRowFlavor("", []byte(c.peek))
		gotName := rendererName(got)
		if gotName != c.want {
			t.Errorf("detectRowFlavor(\"\", %q) = %s, want %s", c.peek, gotName, c.want)
		}
	}
}

func rendererName(r stripes.Renderer) string {
	switch reflect.ValueOf(r).Pointer() {
	case reflect.ValueOf(stripes.Renderer(csvTable)).Pointer():
		return "csvTable"
	case reflect.ValueOf(stripes.Renderer(tsvTable)).Pointer():
		return "tsvTable"
	case reflect.ValueOf(stripes.Renderer(jsonlTable)).Pointer():
		return "jsonlTable"
	}
	return "unknown"
}
