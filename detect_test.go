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
		{"foo.md", "x", "text/markdown"},
		{"foo.MD", "x", "text/markdown"},
		{"foo.markdown", "x", "text/markdown"},
		{"foo.txt", "<html>", "text/plain"},

		// Dockerfile detection
		{"Dockerfile", "x", "text/x-dockerfile"},
		{"Containerfile", "x", "text/x-dockerfile"},
		{"foo.dockerfile", "x", "text/x-dockerfile"},
		{"path/to/Dockerfile", "x", "text/x-dockerfile"},

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

		// sniffing — Dockerfile
		{"", "FROM alpine\n", "text/x-dockerfile"},
		{"", "from alpine:3\nRUN ls\n", "text/x-dockerfile"},
		{"", "ARG VERSION=3\nFROM alpine:${VERSION}\n", "text/x-dockerfile"},
		{"", "# syntax=docker/dockerfile:1\nFROM alpine\n", "text/x-dockerfile"},
		{"", "# escape=`\nFROM alpine\n", "text/x-dockerfile"},

		// fallback
		{"", "just plain text\n", "text/plain"},
		{"", "", "text/plain"},
		{"unknownext.bin", "", "text/plain"},

		// http.DetectContentType bridge
		{"", "<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>", "application/xml"},

		// chroma-driven extension fallback for source code
		{"foo.go", "x", "text/x-source-code; lang=Go"},
		{"foo.py", "x", "text/x-source-code; lang=Python"},
		{"foo.rs", "x", "text/x-source-code; lang=Rust"},

		// WebAssembly
		{"foo.wasm", "x", "application/wasm"},
		{"foo.wat", "x", "text/x-source-code; lang=wat"},
		{"foo.wast", "x", "text/x-source-code; lang=wat"},
		{"", "\x00asm\x01\x00\x00\x00", "application/wasm"},
		{"unknown.bin", "\x00asm\x01\x00\x00\x00", "application/wasm"},
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
