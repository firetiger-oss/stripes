package main

import (
	"testing"

	// Side-effect import: registers image/* content types so
	// stripes.Detect classifies image filenames correctly.
	_ "github.com/firetiger-oss/stripes/all"
)

func TestAnyImageInputByExtension(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"none", []string{"README.md", "data.json"}, false},
		{"single png", []string{"logo.png"}, true},
		{"mixed with image", []string{"README.md", "logo.png"}, true},
		{"uppercase JPG", []string{"PHOTO.JPG"}, true},
		{"tiff", []string{"scan.tiff"}, true},
		{"webp", []string{"banner.webp"}, true},
		{"url with extension", []string{"https://example.com/x.png?token=abc"}, true},
		{"empty list", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyImageInput(&config{}, tc.files); got != tc.want {
				t.Errorf("anyImageInput(%v) = %v, want %v", tc.files, got, tc.want)
			}
		})
	}
}

func TestAnyImageInputByContentType(t *testing.T) {
	cfg := &config{contentType: "image/png"}
	if !anyImageInput(cfg, nil) {
		t.Errorf("explicit --content-type image/png should trigger image detection")
	}
	cfg = &config{contentType: "application/json"}
	if anyImageInput(cfg, []string{"README.md"}) {
		t.Errorf("non-image --content-type with markdown file should not trigger")
	}
}
