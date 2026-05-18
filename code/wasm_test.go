package code

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"
)

// minimalWasm is a valid 30-byte WebAssembly module exporting a no-op
// function named "noop". Hand-crafted so the test does not require a
// committed binary fixture.
//
//	00 61 73 6d 01 00 00 00              header (\0asm v1)
//	01 04 01 60 00 00                    type section: () -> ()
//	03 02 01 00                          function section: 1 func, type 0
//	07 08 01 04 6e 6f 6f 70 00 00        export "noop" -> func 0
//	0a 04 01 02 00 0b                    code section: empty body
var minimalWasm = []byte{
	0x00, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00,
	0x01, 0x04, 0x01, 0x60, 0x00, 0x00,
	0x03, 0x02, 0x01, 0x00,
	0x07, 0x08, 0x01, 0x04, 0x6e, 0x6f, 0x6f, 0x70, 0x00, 0x00,
	0x0a, 0x04, 0x01, 0x02, 0x00, 0x0b,
}

func hasAnyWasmBackend() bool {
	if _, err := exec.LookPath("wasm-tools"); err == nil {
		return true
	}
	if _, err := exec.LookPath("wasm2wat"); err == nil {
		return true
	}
	return false
}

func selectedWasmBackend(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("wasm-tools"); err == nil {
		return "wasm-tools"
	}
	if _, err := exec.LookPath("wasm2wat"); err == nil {
		return "wasm2wat"
	}
	return ""
}

func TestWasmRendererNoBackend(t *testing.T) {
	t.Setenv("PATH", "")

	plain := &stripes.Styles{Indent: "  "}
	var buf bytes.Buffer
	RenderWasm(&buf, bytes.NewReader(minimalWasm), plain)

	got := buf.String()
	if !strings.Contains(got, "wasm-tools") || !strings.Contains(got, "wasm2wat") {
		t.Fatalf("expected diagnostic mentioning both wasm-tools and wasm2wat, got: %q", got)
	}
}

func TestWasmRenderer(t *testing.T) {
	if !hasAnyWasmBackend() {
		t.Skip("neither wasm-tools nor wasm2wat installed; skipping")
	}

	plain := &stripes.Styles{Indent: "  "}
	var buf bytes.Buffer
	RenderWasm(&buf, bytes.NewReader(minimalWasm), plain)

	got := buf.String()
	if !strings.Contains(got, "(module") {
		t.Fatalf("output missing (module: %q", got)
	}
	if !strings.Contains(got, "(func") {
		t.Fatalf("output missing (func: %q", got)
	}
	if !strings.Contains(got, "noop") {
		t.Fatalf("output missing export name noop: %q", got)
	}
}

func TestWasmRendererInvalidBinary(t *testing.T) {
	backend := selectedWasmBackend(t)
	if backend == "" {
		t.Skip("neither wasm-tools nor wasm2wat installed; skipping")
	}

	plain := &stripes.Styles{Indent: "  "}
	var buf bytes.Buffer
	RenderWasm(&buf, bytes.NewReader([]byte("not a wasm binary")), plain)

	got := buf.String()
	want := "ERROR: " + backend + ":"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("expected %s error diagnostic, got: %q", backend, got)
	}
}
