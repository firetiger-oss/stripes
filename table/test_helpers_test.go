package table

import (
	"bytes"
	"iter"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// seqOf turns a slice into an iter.Seq[T].
func seqOf[T any](vs []T) iter.Seq[T] {
	return func(yield func(T) bool) {
		for _, v := range vs {
			if !yield(v) {
				return
			}
		}
	}
}

// seq2Of turns a slice into an iter.Seq2[T, error] with no error.
func seq2Of[T any](vs []T) iter.Seq2[T, error] {
	return func(yield func(T, error) bool) {
		for _, v := range vs {
			if !yield(v, nil) {
				return
			}
		}
	}
}

// render formats rows via Format[T] and strips ANSI. Use for plain-text
// equality assertions on the rendered table.
func render[T any](rows []T, opts ...Option) string {
	return ansi.Strip(Format[T](seqOf(rows), opts...))
}

// renderWrite is like render but uses Write[T] (Seq2) so callers can also
// inspect a returned error. The output is ANSI-stripped.
func renderWrite[T any](rows []T, opts ...Option) (string, error) {
	var buf bytes.Buffer
	err := Write[T](&buf, seq2Of(rows), opts...)
	return ansi.Strip(buf.String()), err
}

// forceColor flips the lipgloss profile to TrueColor for the test and
// resets it on cleanup, so the renderer emits ANSI escape sequences even
// when the test process isn't attached to a terminal.
func forceColor(t *testing.T) {
	t.Helper()
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })
}

// equal asserts strict equality between got and want. On mismatch it
// prints both in raw multi-line form (easy to scan for whitespace/layout
// regressions) and again in Go-quoted form (revealing escape characters
// or trailing whitespace).
func equal(t *testing.T, got, want string) {
	t.Helper()
	if got == want {
		return
	}
	t.Errorf(
		"output mismatch:\n--- got (%d bytes) ---\n%s\n--- want (%d bytes) ---\n%s\n--- raw ---\ngot:  %q\nwant: %q",
		len(got), got, len(want), want, got, want,
	)
}

// equalErr asserts that err is non-nil and err.Error() == want.
func equalErr(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error %q, got nil", want)
	}
	if got := err.Error(); got != want {
		t.Errorf("err.Error()\n  got:  %q\n  want: %q", got, want)
	}
}
