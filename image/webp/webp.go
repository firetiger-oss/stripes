// Package webp registers the WebP image renderer with the stripes
// registry. Import for side effects to enable image/webp support:
//
//	import _ "github.com/firetiger-oss/stripes/image/webp"
package webp

import (
	"bytes"

	_ "golang.org/x/image/webp" // decoder for stripes/image fallback path

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/image"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "webp",
		ContentType: "image/webp",
		Extensions:  []string{".webp"},
		// WebP shares the "RIFF" prefix with WAV/AVI; the "WEBP" fourcc
		// at offset 8 is the disambiguator, so a Detect callback is
		// needed rather than a flat MagicBytes prefix match.
		Detect:      detectWebP,
		RendererFor: stripes.Simple(image.NewRenderer("image/webp")),
	})
}

func detectWebP(peek []byte) bool {
	return len(peek) >= 12 &&
		bytes.Equal(peek[:4], []byte("RIFF")) &&
		bytes.Equal(peek[8:12], []byte("WEBP"))
}
