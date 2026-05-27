// Package tiff registers the TIFF image renderer with the stripes
// registry. Import for side effects to enable image/tiff support:
//
//	import _ "github.com/firetiger-oss/stripes/image/tiff"
package tiff

import (
	_ "golang.org/x/image/tiff" // decoder for stripes/image fallback path

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/image"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "tiff",
		ContentType: "image/tiff",
		Extensions:  []string{".tif", ".tiff"},
		MagicBytes: [][]byte{
			{'I', 'I', 0x2a, 0x00}, // little-endian
			{'M', 'M', 0x00, 0x2a}, // big-endian
		},
		RendererFor: stripes.Simple(image.NewRenderer("image/tiff")),
	})
}
