// Package image is the shared dispatch helper for the per-format image
// sub-packages. It does not register any formats on its own — import
// one of [github.com/firetiger-oss/stripes/image/png],
// [github.com/firetiger-oss/stripes/image/jpeg], etc., or
// [github.com/firetiger-oss/stripes/all] to enable image rendering.
//
// The renderer returned by [NewRenderer] inspects environment variables
// for kitty graphics protocol or iTerm2 inline-image protocol support
// and emits the image inline. Terminals that don't advertise either
// protocol get a styled one-line placeholder describing the image.
package image

import (
	"bytes"
	"fmt"
	"image"
	"io"
	"path"
	"strings"

	"github.com/BourgeoisBear/rasterm"
	"github.com/firetiger-oss/stripes"
)

// NewRenderer returns a [stripes.Renderer] that emits an image inline
// for terminals that support kitty or iTerm2 graphics protocols, or a
// styled placeholder otherwise. contentType is the MIME type registered
// for the format (e.g. "image/png") — it selects the Kitty raw-PNG
// passthrough for PNG inputs and labels the fallback line.
func NewRenderer(contentType string) stripes.Renderer {
	label := formatLabel(contentType)
	defaultName := defaultName(contentType)
	isPNG := contentType == "image/png"
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		data, err := io.ReadAll(r)
		if err != nil {
			writeFallback(w, label, 0, 0, len(data), styles)
			return
		}
		if stripes.IsANSIEnabled(styles) {
			width := uint32(0)
			if styles.Width > 0 {
				width = uint32(styles.Width)
			}
			name := path.Base(styles.SourceName)
			if name == "" || name == "." || name == "/" {
				name = defaultName
			}
			switch {
			case rasterm.IsKittyCapable():
				if writeKitty(w, data, isPNG, width) == nil {
					return
				}
			case rasterm.IsItermCapable():
				if writeIterm(w, data, name, width) == nil {
					return
				}
			}
		}
		cfg, _, _ := image.DecodeConfig(bytes.NewReader(data))
		writeFallback(w, label, cfg.Width, cfg.Height, len(data), styles)
	}
}

func writeKitty(w io.Writer, data []byte, isPNG bool, width uint32) error {
	opts := rasterm.KittyImgOpts{DstCols: width}
	if isPNG {
		return rasterm.KittyCopyPNGInline(w, bytes.NewReader(data), opts)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return err
	}
	return rasterm.KittyWriteImage(w, img, opts)
}

func writeIterm(w io.Writer, data []byte, name string, width uint32) error {
	opts := rasterm.ItermImgOpts{
		DisplayInline: true,
		Size:          int64(len(data)),
		Name:          name,
	}
	if width > 0 {
		opts.Width = fmt.Sprintf("%d", width)
	}
	return rasterm.ItermCopyFileInlineWithOptions(w, bytes.NewReader(data), opts)
}

// formatLabel turns "image/png" into "PNG" for the fallback line.
func formatLabel(contentType string) string {
	if _, sub, ok := strings.Cut(contentType, "/"); ok {
		return strings.ToUpper(sub)
	}
	return strings.ToUpper(contentType)
}

// defaultName produces a synthetic filename for the iTerm name= field
// when the caller doesn't supply a SourceName, so iTerm prompts read
// "image.png" instead of rasterm's "Unnamed file" default.
func defaultName(contentType string) string {
	if _, sub, ok := strings.Cut(contentType, "/"); ok {
		return "image." + sub
	}
	return "image"
}

func writeFallback(w io.Writer, label string, width, height, size int, styles *stripes.Styles) {
	var head string
	switch {
	case width > 0 && height > 0:
		head = fmt.Sprintf("[image: %s %d×%d, %s]", label, width, height, humanSize(size))
	case size > 0:
		head = fmt.Sprintf("[image: %s, %s]", label, humanSize(size))
	default:
		head = fmt.Sprintf("[image: %s]", label)
	}
	io.WriteString(w, styles.Syntax.Render(head))
	io.WriteString(w, " ")
	io.WriteString(w, styles.Comment.Render("(terminal does not support inline images)"))
}

func humanSize(n int) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := int64(n) / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
