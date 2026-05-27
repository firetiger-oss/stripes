// Package jpeg registers the JPEG image renderer with the stripes
// registry. Import for side effects to enable image/jpeg support:
//
//	import _ "github.com/firetiger-oss/stripes/image/jpeg"
package jpeg

import (
	_ "image/jpeg" // decoder for stripes/image fallback path

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/image"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "jpeg",
		ContentType: "image/jpeg",
		Extensions:  []string{".jpg", ".jpeg"},
		MagicBytes:  [][]byte{{0xff, 0xd8, 0xff}},
		RendererFor: stripes.Simple(image.NewRenderer("image/jpeg")),
	})
}
