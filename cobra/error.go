package cobra

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/pflag"
)

// DefaultErrorHandler writes a styled "Error: <msg>" line to w. For errors
// that look like flag- or usage-parsing failures, it also emits a hint
// pointing users at --help.
func DefaultErrorHandler(w io.Writer, s *Styles, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(w, "%s %s\n",
		s.Error.Render("Error:"),
		s.Description.Render(err.Error()),
	)
	if isUsageError(err) {
		fmt.Fprintln(w, s.Hint.Render("Try --help for usage."))
	}
}

// isUsageError reports whether err looks like a flag-parse or unknown-command
// error from cobra/pflag. Cobra has no typed error for these; matching on the
// message prefix is the same approach charmbracelet/fang takes.
func isUsageError(err error) bool {
	if errors.Is(err, pflag.ErrHelp) {
		return true
	}
	msg := err.Error()
	for _, prefix := range []string{
		"unknown flag",
		"unknown shorthand flag",
		"unknown command",
		"required flag",
		"invalid argument",
		"flag needs an argument",
	} {
		if strings.HasPrefix(msg, prefix) {
			return true
		}
	}
	return false
}
