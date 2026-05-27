package markdown

import (
	"bytes"
	"encoding/base64"
	"errors"
	"image"
	"image/color"
	stdpng "image/png"
	"io"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"

	// Register the PNG image renderer so stripes.Func("image/png", "")
	// returns a non-nil renderer. The markdown package itself does not
	// import any stripes/image/* sub-package — production code reaches
	// the image renderer only through the global registry.
	_ "github.com/firetiger-oss/stripes/image/png"
)

func TestMarkdownDataURIImageRenders(t *testing.T) {
	enableTerminalGraphics(t)

	pngBytes := makePNG(t, 4, 4)
	uri := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	input := "![alt](" + uri + ")\n"

	out := renderMarkdown(t, input, stripes.DefaultStyles.Clone())

	if !bytes.Contains(out, []byte("\x1b_G")) {
		t.Fatalf("kitty graphics header not emitted for data URI image\nfirst 64 bytes: %q", clip(out, 64))
	}
	if bytes.Contains(out, []byte("[image]")) {
		t.Fatalf("placeholder leaked into output: %q", out)
	}
}

func TestMarkdownFetcherSuppliesImage(t *testing.T) {
	enableTerminalGraphics(t)
	pngBytes := makePNG(t, 4, 4)

	var seenRef string
	styles := stripes.DefaultStyles.Clone()
	styles.SourceName = "/docs/readme.md"
	styles.ImageFetcher = func(ref string) (io.ReadCloser, string, error) {
		seenRef = ref
		return io.NopCloser(bytes.NewReader(pngBytes)), "image/png", nil
	}

	out := renderMarkdown(t, "![alt](pictures/foo.png)\n", styles)

	if seenRef != "/docs/pictures/foo.png" {
		t.Errorf("fetcher saw ref %q, want %q", seenRef, "/docs/pictures/foo.png")
	}
	if !bytes.Contains(out, []byte("\x1b_G")) {
		t.Fatalf("kitty header not emitted; got %q", clip(out, 96))
	}
}

func TestMarkdownInlineImageStaysPlaceholder(t *testing.T) {
	enableTerminalGraphics(t)
	pngBytes := makePNG(t, 4, 4)

	styles := stripes.DefaultStyles.Clone()
	styles.SourceName = "/docs/readme.md"
	styles.ImageFetcher = func(ref string) (io.ReadCloser, string, error) {
		t.Fatalf("fetcher must not be called for inline-in-paragraph image; got ref %q", ref)
		return io.NopCloser(bytes.NewReader(pngBytes)), "image/png", nil
	}

	out := renderMarkdown(t, "see ![alt](foo.png) below\n", styles)

	if bytes.Contains(out, []byte("\x1b_G")) {
		t.Fatalf("kitty header leaked into mid-paragraph image: %q", clip(out, 96))
	}
	if !bytes.Contains(out, []byte("[image]")) {
		t.Fatalf("placeholder missing for inline image: %q", out)
	}
}

func TestMarkdownFetcherErrorFallsBack(t *testing.T) {
	enableTerminalGraphics(t)

	styles := stripes.DefaultStyles.Clone()
	styles.SourceName = "/docs/readme.md"
	styles.ImageFetcher = func(ref string) (io.ReadCloser, string, error) {
		return nil, "", errors.New("no such file")
	}

	out := renderMarkdown(t, "![alt](missing.png)\n", styles)

	if bytes.Contains(out, []byte("\x1b_G")) {
		t.Fatalf("kitty header should not appear on fetcher error: %q", clip(out, 96))
	}
	if !bytes.Contains(out, []byte("[image]")) {
		t.Fatalf("expected placeholder, got %q", out)
	}
}

func TestMarkdownNoFetcherFallsBack(t *testing.T) {
	enableTerminalGraphics(t)

	styles := stripes.DefaultStyles.Clone()
	styles.SourceName = "/docs/readme.md"
	// No ImageFetcher set; non-data refs must fall back to placeholder.

	out := renderMarkdown(t, "![alt](foo.png)\n", styles)

	if bytes.Contains(out, []byte("\x1b_G")) {
		t.Fatalf("kitty header should not appear without a fetcher: %q", clip(out, 96))
	}
	if !bytes.Contains(out, []byte("[image]")) {
		t.Fatalf("expected placeholder, got %q", out)
	}
}

func TestResolveImageRef(t *testing.T) {
	cases := []struct {
		name, source, ref, want string
	}{
		{"stdin", "", "foo.png", "foo.png"},
		{"bare-basename-no-leading-slash", "README.md", "assets/foo.png", "assets/foo.png"},
		{"relative-dir", "docs/guide.md", "img/x.png", "docs/img/x.png"},
		{"absolute-source", "/repo/docs/guide.md", "img/x.png", "/repo/docs/img/x.png"},
		{"absolute-ref-passes-through", "README.md", "/abs/x.png", "/abs/x.png"},
		{"scheme-ref-passes-through", "README.md", "https://example.com/x.png", "https://example.com/x.png"},
		{"url-source", "https://example.com/docs/r.md", "img/x.png", "https://example.com/docs/img/x.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveImageRef(tc.source, tc.ref); got != tc.want {
				t.Errorf("resolveImageRef(%q, %q) = %q, want %q", tc.source, tc.ref, got, tc.want)
			}
		})
	}
}

func TestMarkdownImageLinkRendersInline(t *testing.T) {
	// "[![alt](src)](href)" — the badge-link pattern detected by
	// imageOnlyLink. The image content is the only renderable child of
	// the paragraph (via the wrapping link), so inline image rendering
	// applies.
	enableTerminalGraphics(t)
	pngBytes := makePNG(t, 4, 4)

	styles := stripes.DefaultStyles.Clone()
	styles.SourceName = "/docs/readme.md"
	styles.ImageFetcher = func(ref string) (io.ReadCloser, string, error) {
		return io.NopCloser(bytes.NewReader(pngBytes)), "image/png", nil
	}

	out := renderMarkdown(t, "[![alt](badge.png)](https://example.com)\n", styles)

	if !bytes.Contains(out, []byte("\x1b_G")) {
		t.Fatalf("kitty header not emitted for badge link: %q", clip(out, 96))
	}
}

func enableTerminalGraphics(t *testing.T) {
	t.Helper()
	for _, k := range []string{
		"TERM",
		"TERM_PROGRAM",
		"LC_TERMINAL",
		"VIM_TERMINAL",
		"KITTY_WINDOW_ID",
	} {
		t.Setenv(k, "")
	}
	t.Setenv("KITTY_WINDOW_ID", "1")
}

func renderMarkdown(t *testing.T, input string, styles *stripes.Styles) []byte {
	t.Helper()
	var buf bytes.Buffer
	Render(&buf, strings.NewReader(input), styles)
	return buf.Bytes()
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

func clip(b []byte, max int) []byte {
	if len(b) <= max {
		return b
	}
	return b[:max]
}
