package image_test

import (
	"bytes"
	"encoding/base64"
	"image"
	"image/color"
	stdpng "image/png"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"
	stripesimage "github.com/firetiger-oss/stripes/image"
	_ "github.com/firetiger-oss/stripes/image/png"
)

func encodedB64(s string) string {
	return base64.StdEncoding.EncodeToString([]byte(s))
}

func clipHeader(b []byte) []byte {
	const max = 96
	if len(b) > max {
		return b[:max]
	}
	return b
}

func TestFallbackWritesPlaceholder(t *testing.T) {
	resetTerminalEnv(t)
	data := makePNG(t, 4, 4)

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader(data), &stripes.Styles{})

	got := buf.String()
	want := "[image: PNG 4×4,"
	if !strings.Contains(got, want) {
		t.Fatalf("output missing %q\ngot: %q", want, got)
	}
	if !strings.Contains(got, "terminal does not support inline images") {
		t.Fatalf("missing fallback parenthetical\ngot: %q", got)
	}
	if strings.ContainsRune(got, 0x1b) {
		t.Fatalf("zero-value styles should not emit ANSI\ngot: %q", got)
	}
}

func TestFallbackOnCorruptInput(t *testing.T) {
	resetTerminalEnv(t)

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader([]byte("not a real png")), &stripes.Styles{})

	got := buf.String()
	// Width/height unknown on a corrupt header → "[image: PNG, N B]".
	if !strings.HasPrefix(got, "[image: PNG, ") {
		t.Fatalf("expected size-only fallback for corrupt input\ngot: %q", got)
	}
}

func TestKittyPathEmitsGraphicsHeader(t *testing.T) {
	resetTerminalEnv(t)
	t.Setenv("KITTY_WINDOW_ID", "1")

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader(makePNG(t, 4, 4)), stripes.DefaultStyles)

	if !bytes.HasPrefix(buf.Bytes(), []byte("\x1b_G")) {
		t.Fatalf("kitty graphics header not emitted; first 16 bytes: %q", buf.Bytes()[:min(16, buf.Len())])
	}
}

func TestItermPathEmitsImageHeader(t *testing.T) {
	resetTerminalEnv(t)
	t.Setenv("LC_TERMINAL", "iterm2")

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader(makePNG(t, 4, 4)), stripes.DefaultStyles)

	if !bytes.HasPrefix(buf.Bytes(), []byte("\x1b]1337;File=")) {
		t.Fatalf("iterm inline-image header not emitted; first 16 bytes: %q", buf.Bytes()[:min(16, buf.Len())])
	}
}

func TestItermNameFallsBackToContentType(t *testing.T) {
	// Empty SourceName must not surface rasterm's "Unnamed file" default;
	// the renderer derives "image.png" from the content type instead.
	resetTerminalEnv(t)
	t.Setenv("LC_TERMINAL", "iterm2")

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader(makePNG(t, 4, 4)), stripes.DefaultStyles)

	if !bytes.Contains(buf.Bytes(), []byte(encodedB64("image.png"))) {
		t.Fatalf("default name 'image.png' not in iterm header; got %q", clipHeader(buf.Bytes()))
	}
}

func TestItermUsesSourceName(t *testing.T) {
	resetTerminalEnv(t)
	t.Setenv("LC_TERMINAL", "iterm2")

	styles := stripes.DefaultStyles.Clone()
	styles.SourceName = "/some/path/MyPhoto.png"

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader(makePNG(t, 4, 4)), styles)

	// The iTerm name= field is base64-encoded; rasterm should encode the
	// basename, not the full path.
	if !bytes.Contains(buf.Bytes(), []byte(encodedB64("MyPhoto.png"))) {
		t.Fatalf("SourceName basename not in iterm header; got %q", clipHeader(buf.Bytes()))
	}
	if bytes.Contains(buf.Bytes(), []byte(encodedB64("/some/path/MyPhoto.png"))) {
		t.Fatal("iterm header should carry basename, not full path")
	}
}

func TestColorDisabledForcesFallback(t *testing.T) {
	// Even with KITTY_WINDOW_ID set, zero-value styles (no ANSI) must
	// take the fallback path so --color=never users never see escape
	// sequences in their output.
	resetTerminalEnv(t)
	t.Setenv("KITTY_WINDOW_ID", "1")

	var buf bytes.Buffer
	render := stripesimage.NewRenderer("image/png")
	render(&buf, bytes.NewReader(makePNG(t, 2, 2)), &stripes.Styles{})

	if bytes.HasPrefix(buf.Bytes(), []byte("\x1b_G")) {
		t.Fatalf("kitty path taken despite ANSI disabled; got %q", buf.String())
	}
	if !strings.HasPrefix(buf.String(), "[image: PNG") {
		t.Fatalf("expected fallback line, got %q", buf.String())
	}
}

func BenchmarkRenderPNGFallback(b *testing.B) {
	resetTerminalEnv(b)
	data := makePNG(b, 16, 16)
	render := stripesimage.NewRenderer("image/png")
	styles := &stripes.Styles{}

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		render(&buf, bytes.NewReader(data), styles)
	}
}

func BenchmarkRenderPNGKitty(b *testing.B) {
	resetTerminalEnv(b)
	b.Setenv("KITTY_WINDOW_ID", "1")
	data := makePNG(b, 16, 16)
	render := stripesimage.NewRenderer("image/png")
	styles := stripes.DefaultStyles

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		var buf bytes.Buffer
		render(&buf, bytes.NewReader(data), styles)
	}
}

// resetTerminalEnv clears every env var rasterm consults so each test
// starts from a known "no-terminal-support" baseline.
func resetTerminalEnv(t testing.TB) {
	for _, k := range []string{
		"TERM",
		"TERM_PROGRAM",
		"LC_TERMINAL",
		"VIM_TERMINAL",
		"KITTY_WINDOW_ID",
	} {
		t.Setenv(k, "")
	}
}

func makePNG(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.NRGBA{R: uint8(x * 16), G: uint8(y * 16), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := stdpng.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}
