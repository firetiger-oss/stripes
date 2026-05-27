package jpeg_test

import (
	"testing"

	"github.com/firetiger-oss/stripes"
	_ "github.com/firetiger-oss/stripes/image/jpeg"
)

func TestDetectByExtension(t *testing.T) {
	for _, name := range []string{"foo.jpg", "foo.jpeg", "FOO.JPG"} {
		if got := stripes.Detect(name, nil); got != "image/jpeg" {
			t.Errorf("Detect(%q) = %q, want image/jpeg", name, got)
		}
	}
}

func TestDetectByMagicBytes(t *testing.T) {
	peek := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	if got := stripes.Detect("", peek); got != "image/jpeg" {
		t.Fatalf("Detect(magic) = %q, want image/jpeg", got)
	}
}

func TestFuncReturnsRenderer(t *testing.T) {
	if stripes.Func("image/jpeg", "") == nil {
		t.Fatal("Func(image/jpeg) returned nil")
	}
}

func BenchmarkDetectJPEG(b *testing.B) {
	peek := []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F'}
	b.ReportAllocs()
	for b.Loop() {
		if stripes.Detect("", peek) != "image/jpeg" {
			b.Fatal("misdetected")
		}
	}
}
