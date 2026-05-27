// Package gif registers the GIF image renderer with the stripes
// registry. Import for side effects to enable image/gif support:
//
//	import _ "github.com/firetiger-oss/stripes/image/gif"
package gif

import (
	_ "image/gif" // decoder for stripes/image fallback path

	"github.com/firetiger-oss/stripes"
	"github.com/firetiger-oss/stripes/image"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "gif",
		ContentType: "image/gif",
		Extensions:  []string{".gif"},
		MagicBytes: [][]byte{
			[]byte("GIF87a"),
			[]byte("GIF89a"),
		},
		RendererFor: stripes.Simple(image.NewRenderer("image/gif")),
	})
}
