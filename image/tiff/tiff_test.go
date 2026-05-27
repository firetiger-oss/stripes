package tiff_test

import (
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/image/tiff"
)

func TestDetectByExtension(t *testing.T) {
	for _, name := range []string{"foo.tif", "foo.tiff", "FOO.TIFF"} {
		if got := stripes.Detect(name, nil); got != "image/tiff" {
			t.Errorf("Detect(%q) = %q, want image/tiff", name, got)
		}
	}
}

func TestDetectByMagicBytes(t *testing.T) {
	cases := [][]byte{
		{'I', 'I', 0x2a, 0x00, 0, 0, 0, 0},
		{'M', 'M', 0x00, 0x2a, 0, 0, 0, 0},
	}
	for _, peek := range cases {
		if got := stripes.Detect("", peek); got != "image/tiff" {
			t.Errorf("Detect(%q...) = %q, want image/tiff", peek[:4], got)
		}
	}
}

func TestFuncReturnsRenderer(t *testing.T) {
	if stripes.Func("image/tiff", "") == nil {
		t.Fatal("Func(image/tiff) returned nil")
	}
}

func BenchmarkDetectTIFF(b *testing.B) {
	peek := []byte{'I', 'I', 0x2a, 0x00, 0, 0, 0, 0}
	b.ReportAllocs()
	for b.Loop() {
		if stripes.Detect("", peek) != "image/tiff" {
			b.Fatal("misdetected")
		}
	}
}
