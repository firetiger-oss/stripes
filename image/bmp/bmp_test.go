package bmp_test

import (
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/image/bmp"
)

func TestDetectByExtension(t *testing.T) {
	if got := stripes.Detect("foo.bmp", nil); got != "image/bmp" {
		t.Fatalf("Detect(foo.bmp) = %q, want image/bmp", got)
	}
}

func TestDetectByMagicBytes(t *testing.T) {
	peek := append([]byte("BM"), make([]byte, 12)...)
	if got := stripes.Detect("", peek); got != "image/bmp" {
		t.Fatalf("Detect(BM...) = %q, want image/bmp", got)
	}
}

func TestFuncReturnsRenderer(t *testing.T) {
	if stripes.Func("image/bmp", "") == nil {
		t.Fatal("Func(image/bmp) returned nil")
	}
}

func BenchmarkDetectBMP(b *testing.B) {
	peek := append([]byte("BM"), make([]byte, 12)...)
	b.ReportAllocs()
	for b.Loop() {
		if stripes.Detect("", peek) != "image/bmp" {
			b.Fatal("misdetected")
		}
	}
}
