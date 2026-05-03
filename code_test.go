package stripes

import (
	"bytes"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestCodeNoColor(t *testing.T) {
	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	plain := &Styles{Indent: "  "}
	var buf bytes.Buffer
	Code("Go")(&buf, strings.NewReader(src), plain)
	got := buf.String()
	want := strings.TrimRight(src, "\n")
	if got != want {
		t.Fatalf("color-off output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestCodeColorEmitsANSI(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	var buf bytes.Buffer
	Code("Go")(&buf, strings.NewReader(src), DefaultStyles)

	if !bytes.Contains(buf.Bytes(), []byte{0x1b}) {
		t.Fatalf("expected ANSI escapes, got plain output: %q", buf.String())
	}
	if got := ansi.Strip(buf.String()); !strings.Contains(got, "package main") {
		t.Fatalf("stripped output missing source: %q", got)
	}
}

func TestCodeUnknownLangAnalyseFallback(t *testing.T) {
	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	plain := &Styles{Indent: "  "}
	var buf bytes.Buffer
	Code("")(&buf, strings.NewReader(src), plain)
	if got := buf.String(); !strings.Contains(got, "package main") {
		t.Fatalf("output missing input: %q", got)
	}
}

func TestCodeBogusLangFallsThrough(t *testing.T) {
	src := "totally not source code, no language\n"
	plain := &Styles{Indent: "  "}
	var buf bytes.Buffer
	Code("not-a-real-lexer")(&buf, strings.NewReader(src), plain)
	if got := buf.String(); !strings.Contains(got, "totally not source") {
		t.Fatalf("output missing input: %q", got)
	}
}
