// Package all imports every built-in stripes format sub-package for
// side-effect registration. Import it when you want [stripes.Func] and
// [stripes.Detect] to support every format the library ships:
//
//	import _ "github.com/firetiger-oss/stripes/all"
//
// Applications that need only a subset should import the individual
// sub-packages instead, which keeps their dependency graph (and binary)
// free of the formats they don't use.
package all

import (
	// code is imported before the structured-data formats so its
	// filename fallback (chroma's language detection) is registered
	// early; it also registers application/wasm.
	_ "github.com/firetiger-oss/stripes/code"
	_ "github.com/firetiger-oss/stripes/csv"
	_ "github.com/firetiger-oss/stripes/diff"
	_ "github.com/firetiger-oss/stripes/dockerfile"
	_ "github.com/firetiger-oss/stripes/gomod"
	// html is imported before xml so HTML wins when content starts with
	// a bare "<"; the xml detector also explicitly excludes HTML
	// doctypes, so the ordering is belt-and-suspenders.
	_ "github.com/firetiger-oss/stripes/html"
	_ "github.com/firetiger-oss/stripes/image/bmp"
	_ "github.com/firetiger-oss/stripes/image/gif"
	_ "github.com/firetiger-oss/stripes/image/jpeg"
	_ "github.com/firetiger-oss/stripes/image/png"
	_ "github.com/firetiger-oss/stripes/image/tiff"
	_ "github.com/firetiger-oss/stripes/image/webp"
	_ "github.com/firetiger-oss/stripes/json"
	_ "github.com/firetiger-oss/stripes/log"
	_ "github.com/firetiger-oss/stripes/markdown"
	_ "github.com/firetiger-oss/stripes/parquet"
	_ "github.com/firetiger-oss/stripes/protobuf"
	_ "github.com/firetiger-oss/stripes/trace"
	_ "github.com/firetiger-oss/stripes/txtar"
	_ "github.com/firetiger-oss/stripes/xml"
	_ "github.com/firetiger-oss/stripes/yaml"
)
