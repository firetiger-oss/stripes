package gif_test

import (
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/image/gif"
)

func TestDetectByExtension(t *testing.T) {
	if got := stripes.Detect("foo.gif", nil); got != "image/gif" {
		t.Fatalf("Detect(foo.gif) = %q, want image/gif", got)
	}
}

func TestDetectByMagicBytes(t *testing.T) {
	for _, magic := range []string{"GIF87a", "GIF89a"} {
		peek := append([]byte(magic), 0, 0, 0, 0)
		if got := stripes.Detect("", peek); got != "image/gif" {
			t.Errorf("Detect(%q...) = %q, want image/gif", magic, got)
		}
	}
}

func TestFuncReturnsRenderer(t *testing.T) {
	if stripes.Func("image/gif", "") == nil {
		t.Fatal("Func(image/gif) returned nil")
	}
}

func BenchmarkDetectGIF(b *testing.B) {
	peek := append([]byte("GIF89a"), 0, 0, 0, 0)
	b.ReportAllocs()
	for b.Loop() {
		if stripes.Detect("", peek) != "image/gif" {
			b.Fatal("misdetected")
		}
	}
}
