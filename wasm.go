package stripes

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Wasm is a [Renderer] for binary WebAssembly modules
// (application/wasm). It disassembles the input by invoking the
// external wasm2wat tool from WABT, then highlights the resulting WAT
// text with chroma's wat lexer.
//
// wasm2wat must be on $PATH. When it is not, Wasm writes a single-line
// diagnostic instead of failing silently. Install via "brew install
// wabt" on macOS or "apt install wabt" on Debian-derived systems.
func Wasm(w io.Writer, r io.Reader, styles *Styles) {
	bin, err := exec.LookPath("wasm2wat")
	if err != nil {
		fmt.Fprintln(w, "ERROR: wasm2wat not found in $PATH; install WABT to render binary .wasm files")
		return
	}

	src, err := io.ReadAll(r)
	if err != nil {
		fmt.Fprintf(w, "ERROR: %s\n", err)
		return
	}

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
		msg := strings.TrimSpace(stderr.String())
		if i := strings.IndexByte(msg, '\n'); i >= 0 {
			msg = msg[:i]
		}
		if msg == "" {
			msg = err.Error()
		}
		fmt.Fprintf(w, "ERROR: wasm2wat: %s\n", msg)
		return
	}

	highlightCode(w, stdout.Bytes(), "wat", styles)
}
