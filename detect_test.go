package stripes

import "testing"

func TestDetect(t *testing.T) {
	tests := []struct {
		name string
		peek string
		want string
	}{
		// extension wins
		{"foo.json", "not even json", "application/json"},
		{"foo.JSON", "x", "application/json"},
		{"foo.yaml", "x", "application/yaml"},
		{"foo.yml", "x", "application/yaml"},
		{"foo.xml", "x", "application/xml"},
		{"foo.html", "x", "text/html"},
		{"foo.htm", "x", "text/html"},
		{"foo.csv", "x", "text/csv"},
		{"foo.txt", "<html>", "text/plain"},

		// sniffing — JSON
		{"", `{"a":1}`, "application/json"},
		{"", "  \n  [1,2,3]", "application/json"},

		// sniffing — XML
		{"", `<?xml version="1.0"?><root/>`, "application/xml"},
		{"", `<root><a>1</a></root>`, "application/xml"},

		// sniffing — HTML
		{"", `<!DOCTYPE html><html></html>`, "text/html"},
		{"", `<html><body></body></html>`, "text/html"},
		{"", `<HTML>`, "text/html"},

		// sniffing — YAML
		{"", "---\nfoo: bar\n", "application/yaml"},
		{"", "foo: bar\nbaz: 1\n", "application/yaml"},
		{"", "# comment\nfoo: bar\n", "application/yaml"},

		// fallback
		{"", "just plain text\n", "text/plain"},
		{"", "", "text/plain"},
		{"unknownext.bin", "", "text/plain"},

		// http.DetectContentType bridge
		{"", "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "application/xml"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"|"+tt.peek, func(t *testing.T) {
			got := Detect(tt.name, []byte(tt.peek))
			if got != tt.want {
				t.Errorf("Detect(%q, %q) = %q, want %q", tt.name, tt.peek, got, tt.want)
			}
		})
	}
}
