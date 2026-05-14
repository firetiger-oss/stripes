// Package stripes is a streaming, ANSI-colored pretty-printer for
// structured data formats: JSON, YAML, XML, HTML, CSV, protobuf,
// parquet, Dockerfiles, the Go toolchain's flat files, markdown,
// source code, txtar archives, WebAssembly, and plain text.
//
// All renderers share a common shape:
//
//	renderer(w, r, stripes.DefaultStyles)
//
// where w is the styled output sink, r is the raw input stream, and the
// [Styles] value selects colors and layout.
//
// # Format registry
//
// The root package holds only the registry and shared primitives; each
// format lives in its own sub-package that registers itself at init.
// Import the sub-packages for the formats you need:
//
//	import (
//	    "github.com/firetiger-oss/stripes"
//	    _ "github.com/firetiger-oss/stripes/json"
//	    _ "github.com/firetiger-oss/stripes/yaml"
//	)
//
// or import [github.com/firetiger-oss/stripes/all] for the full set.
// [Func] dispatches by MIME type against the registered formats, and
// [Detect] sniffs an unknown stream into a content-type string using
// their registered filenames, extensions, magic bytes, and heuristics.
// A format whose sub-package has not been imported is simply absent:
// [Func] returns nil and [Detect] falls through to the next candidate.
//
// Third-party code can register additional formats with [Register].
//
// The companion command [github.com/firetiger-oss/stripes/cmd/stripes]
// is a file viewer that wraps these primitives with format
// auto-detection, TTY-aware coloring, and built-in paging.
package stripes
