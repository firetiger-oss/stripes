// stub-pager is a deterministic pager replacement used by stripes' integration
// tests. It writes "argv: <argv>\nstdin-bytes: <n>\n" to the file named by
// $STUB_PAGER_LOG (or stderr if unset) and drains stdin. Its exit code is 0
// unless $STUB_PAGER_EXIT is set to a non-empty integer.
package main

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func main() {
	exitCode := 0
	if v := os.Getenv("STUB_PAGER_EXIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			exitCode = n
		}
	}

	n, err := io.Copy(io.Discard, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "stub-pager:", err)
		os.Exit(1)
	}

	report := fmt.Sprintf("argv: %s\nstdin-bytes: %d\n", strings.Join(os.Args, " "), n)

	logPath := os.Getenv("STUB_PAGER_LOG")
	if logPath == "" {
		fmt.Fprint(os.Stderr, report)
	} else {
		if err := os.WriteFile(logPath, []byte(report), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "stub-pager:", err)
			os.Exit(1)
		}
	}

	os.Exit(exitCode)
}
