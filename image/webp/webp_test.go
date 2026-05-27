package webp_test

import (
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/image/webp"
)

// minimalWebPHeader is the smallest byte sequence that satisfies the
// "RIFF...WEBP" detection — enough for the Detect callback to trip,
// but not a decodable image.
var minimalWebPHeader = []byte{
	'R', 'I', 'F', 'F',
	0x00, 0x00, 0x00, 0x00, // file size (don't care for detection)
	'W', 'E', 'B', 'P',
}

func TestDetectByExtension(t *testing.T) {
	if got := stripes.Detect("foo.webp", nil); got != "image/webp" {
		t.Fatalf("Detect(foo.webp) = %q, want image/webp", got)
	}
}

func TestDetectByContent(t *testing.T) {
	if got := stripes.Detect("", minimalWebPHeader); got != "image/webp" {
		t.Fatalf("Detect(magic) = %q, want image/webp", got)
	}
}

func TestDetectNotWebPWithRIFFOnly(t *testing.T) {
	// RIFF...WAVE (or any non-WEBP fourcc) must not resolve to image/webp.
	peek := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'A', 'V', 'E'}
	if got := stripes.Detect("", peek); got == "image/webp" {
		t.Fatalf("Detect(RIFF/WAVE) misidentified as image/webp")
	}
}

func TestFuncReturnsRenderer(t *testing.T) {
	if stripes.Func("image/webp", "") == nil {
		t.Fatal("Func(image/webp) returned nil")
	}
}

func BenchmarkDetectWebP(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if stripes.Detect("", minimalWebPHeader) != "image/webp" {
			b.Fatal("misdetected")
		}
	}
}
