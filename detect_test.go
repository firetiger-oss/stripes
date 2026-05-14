package stripes_test

import (
	"mime"
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/all"
)

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

		// Go module toolchain files
		{"go.mod", "x", "text/x-go-mod"},
		{"path/to/go.mod", "x", "text/x-go-mod"},
		{"go.sum", "x", "text/x-go-sum"},
		{"go.work.sum", "x", "text/x-go-sum"},
		{"go.work", "x", "text/x-go-work"},
		{"vendor/modules.txt", "x", "text/x-go-vendor-modules"},
		{"path/to/vendor/modules.txt", "x", "text/x-go-vendor-modules"},
		{"notvendor/modules.txt", "x", "text/plain"},
		{"modules.txt", "x", "text/plain"},

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

		// chroma lexer names containing whitespace must be quoted so
		// mime.ParseMediaType can recover them downstream.
		{"foo.proto", "x", `text/x-source-code; lang="Protocol Buffer"`},
		{"foo.lisp", "x", `text/x-source-code; lang="Common Lisp"`},

		// WebAssembly
		{"foo.wasm", "x", "application/wasm"},
		{"foo.wat", "x", "text/x-source-code; lang=wat"},
		{"foo.wast", "x", "text/x-source-code; lang=wat"},
		{"", "\x00asm\x01\x00\x00\x00", "application/wasm"},
		{"unknown.bin", "\x00asm\x01\x00\x00\x00", "application/wasm"},

		// Parquet
		{"foo.parquet", "x", "application/vnd.apache.parquet"},
		{"data.PARQUET", "x", "application/vnd.apache.parquet"},
		{"", "PAR1\x00\x00\x00", "application/vnd.apache.parquet"},
		{"unknown.bin", "PAR1\x00\x00\x00", "application/vnd.apache.parquet"},

		// txtar
		{"foo.txtar", "x", "text/x-txtar"},
		{"archive.TXTAR", "x", "text/x-txtar"},
	}

	for _, tt := range tests {
		t.Run(tt.name+"|"+tt.peek, func(t *testing.T) {
			got := stripes.Detect(tt.name, []byte(tt.peek))
			if got != tt.want {
				t.Errorf("stripes.Detect(%q, %q) = %q, want %q", tt.name, tt.peek, got, tt.want)
			}
		})
	}
}

// TestDetectMIMERoundTrip verifies that values returned by Detect parse
// cleanly back through mime.ParseMediaType — the actual contract Func
// relies on. A previous version concatenated lang=Protocol Buffer
// unquoted, which broke mime parsing and silently fell back to chroma's
// content-sniffing for every multi-word lexer name.
func TestDetectMIMERoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		wantLang string
	}{
		{"foo.go", "Go"},
		{"foo.proto", "Protocol Buffer"},
		{"foo.lisp", "Common Lisp"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ct := stripes.Detect(tt.name, []byte("x"))
			_, params, err := mime.ParseMediaType(ct)
			if err != nil {
				t.Fatalf("mime.ParseMediaType(%q) error: %v", ct, err)
			}
			if params["lang"] != tt.wantLang {
				t.Fatalf("lang = %q, want %q (from %q)", params["lang"], tt.wantLang, ct)
			}
		})
	}
}
