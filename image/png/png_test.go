package png_test

import (
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/image/png"
)

func TestDetectByExtension(t *testing.T) {
	if got := stripes.Detect("foo.png", nil); got != "image/png" {
		t.Fatalf("Detect(foo.png) = %q, want image/png", got)
	}
}

func TestDetectByMagicBytes(t *testing.T) {
	peek := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0}
	if got := stripes.Detect("", peek); got != "image/png" {
		t.Fatalf("Detect(magic) = %q, want image/png", got)
	}
}

func TestFuncReturnsRenderer(t *testing.T) {
	if stripes.Func("image/png", "") == nil {
		t.Fatal("Func(image/png) returned nil")
	}
}

func BenchmarkDetectPNG(b *testing.B) {
	peek := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0}
	b.ReportAllocs()
	for b.Loop() {
		if stripes.Detect("", peek) != "image/png" {
			b.Fatal("misdetected")
		}
	}
}

