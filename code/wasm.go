package code

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "wasm",
		ContentType: "application/wasm",
		Extensions:  []string{".wasm"},
		MagicBytes:  [][]byte{{0x00, 'a', 's', 'm'}},
		RendererFor: stripes.Simple(RenderWasm),
	})
}

// RenderWasm writes a styled rendering of the binary WebAssembly module
// (application/wasm) read from r to w. It disassembles the input by
// invoking an external tool and highlights the resulting WAT text with
// chroma's wat lexer.
//
// The preferred backend is "wasm-tools print" from the bytecodealliance
// (https://github.com/bytecodealliance/wasm-tools), which tracks the
// current WebAssembly specification including the component model. When
// wasm-tools is not on $PATH, RenderWasm falls back to "wasm2wat" from
// WABT (https://github.com/WebAssembly/wabt). When neither is
// available, RenderWasm writes a single-line diagnostic instead of
// failing silently. Install via "cargo install wasm-tools" or "brew
// install wasm-tools"; install WABT via "brew install wabt" or "apt
// install wabt".
func RenderWasm(w io.Writer, r io.Reader, styles *stripes.Styles) {
	src, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}

	if bin, err := exec.LookPath("wasm-tools"); err == nil {
		runWasmTools(w, bin, src, styles)
		return
	}
	if bin, err := exec.LookPath("wasm2wat"); err == nil {
		runWasm2Wat(w, bin, src, styles)
		return
	}
	fmt.Fprintln(w, "ERROR: neither wasm-tools nor wasm2wat found in $PATH; install wasm-tools (https://github.com/bytecodealliance/wasm-tools) or WABT to render binary .wasm files")
}

func runWasmTools(w io.Writer, bin string, src []byte, styles *stripes.Styles) {
	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "print", "--name-unnamed", "--fold-instructions", "--color", "never", "-")
	cmd.Stdin = bytes.NewReader(src)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		writeBackendError(w, "wasm-tools", stderr.String(), err)
		return
	}
	highlightCode(w, stdout.Bytes(), "wat", styles)
}

func runWasm2Wat(w io.Writer, bin string, src []byte, styles *stripes.Styles) {
	tmp, err := os.CreateTemp("", "stripes-*.wasm")
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(src); err != nil {
		tmp.Close()
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}
	if err := tmp.Close(); err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(bin, "--generate-names", "--fold-exprs", tmp.Name())
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		writeBackendError(w, "wasm2wat", stderr.String(), err)
		return
	}
	highlightCode(w, stdout.Bytes(), "wat", styles)
}

func writeBackendError(w io.Writer, tool, stderr string, err error) {
	msg := strings.TrimSpace(stderr)
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	if msg == "" {
		msg = err.Error()
	}
	fmt.Fprintf(w, "ERROR: %s: %s\n", tool, msg)
}
