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

// runOnPTY runs cmd attached to a freshly allocated PTY of the requested
// size, reads all output, and returns it once the process exits or the
// deadline elapses. When rows/cols are 0 the kernel default is used.
func runOnPTY(t *testing.T, cmd *exec.Cmd, rows, cols uint16) []byte {
	t.Helper()
	var f *os.File
	var err error
	if rows == 0 && cols == 0 {
		f, err = pty.Start(cmd)
	} else {
		f, err = pty.StartWithSize(cmd, &pty.Winsize{Rows: rows, Cols: cols})
	}
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

// stubPagerEnv composes the environment needed to swap the pager out for
// the deterministic stub-pager binary built by TestMain.
func stubPagerEnv(binDir, logPath string) []string {
	return append(os.Environ(),
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"STRIPES_PAGER=stub-pager",
		"STUB_PAGER_LOG="+logPath,
	)
}

func TestPagingAlwaysOnTTY(t *testing.T) {
	bin := stripesBin(t)
	binDir := filepath.Dir(bin)
	logPath := filepath.Join(t.TempDir(), "pager.log")

	cmd := exec.Command(bin, "--color=never", "--paging=always")
	cmd.Env = stubPagerEnv(binDir, logPath)
	cmd.Stdin = strings.NewReader(`{"hello":"world"}`)

	_ = runOnPTY(t, cmd, 0, 0)

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

func TestPagingAutoSmallContentNoPager(t *testing.T) {
	bin := stripesBin(t)
	binDir := filepath.Dir(bin)
	logPath := filepath.Join(t.TempDir(), "pager.log")

	cmd := exec.Command(bin, "--color=never", "--paging=auto")
	cmd.Env = stubPagerEnv(binDir, logPath)
	cmd.Stdin = strings.NewReader(`{"a":1}`)

	_ = runOnPTY(t, cmd, 24, 80)

	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("expected no pager log (small content shouldn't page); err=%v", err)
	}
}

func TestPagingAutoTallContentPages(t *testing.T) {
	bin := stripesBin(t)
	binDir := filepath.Dir(bin)
	logPath := filepath.Join(t.TempDir(), "pager.log")

	// Build a JSON object with enough keys that the rendered output blows
	// past 10 rows on a 10-row PTY.
	var sb strings.Builder
	sb.WriteString("{")
	for i := range 30 {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`"k`)
		sb.WriteString(strings.Repeat("x", 1))
		sb.WriteString(`":`)
		sb.WriteString("1")
	}
	sb.WriteString("}")

	cmd := exec.Command(bin, "--color=never", "--paging=auto")
	cmd.Env = stubPagerEnv(binDir, logPath)
	cmd.Stdin = strings.NewReader(sb.String())

	_ = runOnPTY(t, cmd, 10, 80)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("pager log not written (tall content should have triggered paging): %v", err)
	}
	if !bytes.Contains(data, []byte("stub-pager")) {
		t.Errorf("pager log doesn't reference stub-pager: %q", data)
	}
}

func TestPagingAutoWideContentPages(t *testing.T) {
	bin := stripesBin(t)
	binDir := filepath.Dir(bin)
	logPath := filepath.Join(t.TempDir(), "pager.log")

	// Use a string value wider than 40 cells. The rendered "key": "value"
	// line will exceed the 40-column PTY width and trigger paging.
	long := strings.Repeat("x", 80)
	input := `{"k":"` + long + `"}`

	cmd := exec.Command(bin, "--color=never", "--paging=auto")
	cmd.Env = stubPagerEnv(binDir, logPath)
	cmd.Stdin = strings.NewReader(input)

	_ = runOnPTY(t, cmd, 24, 40)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("pager log not written (wide content should have triggered paging): %v", err)
	}
	if !bytes.Contains(data, []byte("stub-pager")) {
		t.Errorf("pager log doesn't reference stub-pager: %q", data)
	}
}

func TestColorAutoEnabledOnTTY(t *testing.T) {
	bin := stripesBin(t)

	cmd := exec.Command(bin, "-p", "cat", "--color=auto")
	cmd.Stdin = strings.NewReader(`{"a":1}`)

	out := runOnPTY(t, cmd, 0, 0)
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
