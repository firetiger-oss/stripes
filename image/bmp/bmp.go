// Package bmp registers the BMP image renderer with the stripes
// registry. Import for side effects to enable image/bmp support:
//
//	import _ "github.com/firetiger-oss/stripes/image/bmp"
package bmp

import (
	_ "golang.org/x/image/bmp" // decoder for stripes/image fallback path

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/image"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "bmp",
		ContentType: "image/bmp",
		Extensions:  []string{".bmp"},
		MagicBytes:  [][]byte{[]byte("BM")},
		RendererFor: stripes.Simple(image.NewRenderer("image/bmp")),
	})
}
