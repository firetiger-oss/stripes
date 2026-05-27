// Package png registers the PNG image renderer with the stripes
// registry. Import for side effects to enable image/png support:
//
//	import _ "github.com/firetiger-oss/stripes/image/png"
package png

import (
	_ "image/png" // decoder for stripes/image fallback path

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/image"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "png",
		ContentType: "image/png",
		Extensions:  []string{".png"},
		MagicBytes:  [][]byte{{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}},
		RendererFor: stripes.Simple(image.NewRenderer("image/png")),
	})
}
