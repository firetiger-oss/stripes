//go:build !windows

package stripes_test

import (
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

// stripesBin returns the path to the stripes binary built by TestMain.
func stripesBin(t *testing.T) string {
	t.Helper()
	dir := os.Getenv("STRIPES_TEST_BIN")
	if dir == "" {
		t.Skip("STRIPES_TEST_BIN not set; PTY tests require TestMain build")
	}
	return filepath.Join(dir, "stripes")
}

// runOnPTY runs cmd attached to a freshly allocated PTY, reads all output,
// and returns it once the process exits or the deadline elapses.
func runOnPTY(t *testing.T, cmd *exec.Cmd) []byte {
	t.Helper()
	f, err := pty.Start(cmd)
	if err != nil {
		t.Skipf("pty.Start failed (sandboxed CI?): %v", err)
	}
	defer f.Close()

	var buf bytes.Buffer
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(&buf, f)
		close(done)
	}()

	if err := cmd.Wait(); err != nil {
		// pager exits cleanly even when stdin closes; non-zero may also be fine
		// for cat (broken pipe). Don't fail on it — just record.
		t.Logf("cmd.Wait: %v", err)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timed out reading PTY output")
	}
	return buf.Bytes()
}

func TestPagerActiveOnTTY(t *testing.T) {
	bin := stripesBin(t)
	binDir := filepath.Dir(bin)
	logPath := filepath.Join(t.TempDir(), "pager.log")

	cmd := exec.Command(bin, "--color=never")
	cmd.Env = append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STRIPES_PAGER=stub-pager",
		"STUB_PAGER_LOG="+logPath,
	)
	cmd.Stdin = strings.NewReader(`{"hello":"world"}`)

	_ = runOnPTY(t, cmd)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("pager log not written: %v", err)
	}
	if !bytes.Contains(data, []byte("argv:")) {
		t.Errorf("pager log missing argv line: %q", data)
	}
	if !bytes.Contains(data, []byte("stub-pager")) {
		t.Errorf("pager log doesn't reference stub-pager: %q", data)
	}
}

func TestColorAutoEnabledOnTTY(t *testing.T) {
	bin := stripesBin(t)

	cmd := exec.Command(bin, "-p", "cat", "--color=auto")
	cmd.Stdin = strings.NewReader(`{"a":1}`)

	out := runOnPTY(t, cmd)
	if !bytes.Contains(out, []byte("\x1b[")) {
		t.Errorf("expected ANSI escape in PTY output, got %q", out)
	}
}

func TestColorAutoDisabledOffTTY(t *testing.T) {
	// Confirm the inverse: without a PTY, --color=auto produces no ANSI.
	bin := stripesBin(t)

	cmd := exec.Command(bin, "-p", "cat", "--color=auto")
	cmd.Stdin = strings.NewReader(`{"a":1}`)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if bytes.Contains(out, []byte("\x1b[")) {
		t.Errorf("unexpected ANSI escape in non-TTY output: %q", out)
	}
}
