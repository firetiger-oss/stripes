// Package stripes is a streaming, ANSI-colored pretty-printer for structured
// data formats: JSON, YAML, XML, HTML, CSV, protobuf, and plain text.
//
// All renderers share a common shape:
//
//	stripes.JSON(w, r, &stripes.DefaultStyles)
//
// where w is the styled output sink, r is the raw input stream, and the
// [Styles] value selects colors and layout. [Func] dispatches by MIME type,
// and [Detect] sniffs an unknown stream into a content-type string.
//
// The companion command [github.com/firetiger-oss/stripes/cmd/stripes] is a
// file viewer that wraps these primitives with format auto-detection,
// TTY-aware coloring, and built-in paging.
package stripes
