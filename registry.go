package stripes

import (
	"fmt"
	"mime"
	"path/filepath"
	"strings"
	"sync"
)

// Format describes one renderable content type. Sub-packages register
// at init time so that [Func] and [Detect] resolve them. The zero value
// is not valid; Name, ContentType, and RendererFor are required.
type Format struct {
	// Name is a short identifier, e.g. "json". Must be unique across
	// registered formats. The stripes CLI accepts Name as a --format
	// alias.
	Name string

	// ContentType is the canonical MIME media type, e.g. "application/json".
	// Must be unique.
	ContentType string

	// Filenames are exact basenames that imply this format
	// (e.g. "Dockerfile", "go.mod"). Optional.
	Filenames []string

	// Extensions are lowercase file extensions including the dot
	// (e.g. ".json"). Optional.
	Extensions []string

	// MagicBytes are exact byte prefixes that identify this format on
	// content sniffing (e.g. {0x00, 'a', 's', 'm'} for WebAssembly).
	// Optional.
	MagicBytes [][]byte

	// MatchPath is consulted by [Detect] after Filenames and Extensions
	// miss but before MagicBytes and Detect. Used for path-aware rules
	// such as vendor/modules.txt that need the parent directory name.
	// Optional.
	MatchPath func(path string) bool

	// Detect inspects up to ~512 bytes of stream content and returns true
	// when the bytes match this format. Runs after all MagicBytes checks
	// across every registered format. Optional.
	Detect func(peek []byte) bool

	// RendererFor returns the [Renderer] for a parsed content type.
	// params are the MIME parameters from contentType (e.g.
	// map[string]string{"lang": "go"} for "text/x-source-code; lang=go").
	// schemaURL is the optional schema reference passed to [Func]
	// (used by protobuf for descriptor lookup; ignored elsewhere).
	// Formats that ignore both arguments should wrap a static Renderer
	// with [Simple]. Required.
	RendererFor func(params map[string]string, schemaURL string) Renderer
}

// SuffixParam is the key under which [Func] records the +suffix stripped
// from an application/* MIME type (e.g. the "json" in
// "application/protobuf+json") into the params map passed to
// [Format.RendererFor]. It is reserved: a "+" prefix never appears in a
// valid RFC 7231 parameter name, so the key cannot collide with a real
// MIME parameter. Formats with multiple encodings (e.g. protobuf binary
// vs. protojson) inspect this key to pick between them.
const SuffixParam = "+suffix"

// Simple wraps a parameter-less Renderer as a RendererFor function. Use
// in the common case where a format's renderer ignores both MIME
// parameters and the schema URL:
//
//	stripes.Format{..., RendererFor: stripes.Simple(Render)}
func Simple(r Renderer) func(map[string]string, string) Renderer {
	return func(_ map[string]string, _ string) Renderer { return r }
}

// Register adds f to the global format registry. It panics on a missing
// Name, ContentType, or RendererFor; on a duplicate Name or ContentType;
// or on duplicate Filenames/Extensions. Safe to call from init().
func Register(f Format) {
	registryMu.Lock()
	defer registryMu.Unlock()

	switch {
	case f.Name == "":
		panic("stripes: Format.Name is required")
	case f.ContentType == "":
		panic("stripes: Format.ContentType is required for " + f.Name)
	case f.RendererFor == nil:
		panic("stripes: Format.RendererFor is required for " + f.Name)
	}
	if _, dup := registryByName[f.Name]; dup {
		panic(fmt.Sprintf("stripes: duplicate Format.Name %q", f.Name))
	}
	if _, dup := registryByCT[f.ContentType]; dup {
		panic(fmt.Sprintf("stripes: duplicate Format.ContentType %q (registered by %q)", f.ContentType, registryByCT[f.ContentType].Name))
	}
	for _, name := range f.Filenames {
		if prev, dup := registryByFilename[name]; dup {
			panic(fmt.Sprintf("stripes: filename %q registered by both %q and %q", name, prev.Name, f.Name))
		}
	}
	for _, ext := range f.Extensions {
		if prev, dup := registryByExt[ext]; dup {
			panic(fmt.Sprintf("stripes: extension %q registered by both %q and %q", ext, prev.Name, f.Name))
		}
	}

	stored := &f
	registry = append(registry, stored)
	registryByName[f.Name] = stored
	registryByCT[f.ContentType] = stored
	for _, name := range f.Filenames {
		registryByFilename[name] = stored
	}
	for _, ext := range f.Extensions {
		registryByExt[ext] = stored
	}
}

// RegisterFilenameFallback installs a name-based content-type resolver
// consulted by [Detect] after registered Filenames, Extensions, and
// MatchPath rules miss but before any MagicBytes or Detect callbacks
// fire. Used by stripes/code to plug chroma's filename-based language
// detection in without pulling chroma into the root package.
func RegisterFilenameFallback(fn func(name string) (contentType string, ok bool)) {
	registryMu.Lock()
	defer registryMu.Unlock()
	filenameFallbacks = append(filenameFallbacks, fn)
}

// Formats returns a snapshot of every registered format, in registration
// order. The returned values share storage with the registry; callers
// should not mutate them.
func Formats() []Format {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Format, len(registry))
	for i, f := range registry {
		out[i] = *f
	}
	return out
}

var (
	registryMu         sync.RWMutex
	registry           []*Format
	registryByName     = map[string]*Format{}
	registryByCT       = map[string]*Format{}
	registryByFilename = map[string]*Format{}
	registryByExt      = map[string]*Format{}
	filenameFallbacks  []func(name string) (string, bool)
)

// Func returns the [Renderer] matching contentType (a MIME media type),
// or nil if the content type is unsupported by the formats currently
// registered. For application/protobuf, schemaURL is interpreted as
// the full message name used to look up the descriptor in
// [protoregistry.GlobalTypes] / [protoregistry.GlobalFiles]; for other
// formats it is ignored.
//
// To populate the registry, import the per-format sub-packages whose
// renderers you need (e.g. `_ "github.com/firetiger-oss/stripes/json"`),
// or `_ "github.com/firetiger-oss/stripes/all"` for the full set.
func Func(contentType, schemaURL string) Renderer {
	mediaType, params, _ := mime.ParseMediaType(contentType)

	// For application/X+Y, prefer a handler registered for application/X
	// (the specific subtype) and record the stripped suffix in params so
	// the handler can pick its encoding. If no such handler exists, fall
	// back to application/Y (the structural-suffix convention — e.g.
	// application/ld+json → application/json).
	if strings.HasPrefix(mediaType, "application/") {
		if i := strings.LastIndexByte(mediaType, '+'); i >= 0 {
			suffix := mediaType[i+1:]
			registryMu.RLock()
			f := registryByCT[mediaType[:i]]
			registryMu.RUnlock()
			if f != nil {
				if params == nil {
					params = map[string]string{}
				}
				params[SuffixParam] = suffix
				return f.RendererFor(params, schemaURL)
			}
			mediaType = "application/" + suffix
		}
	}

	registryMu.RLock()
	f := registryByCT[mediaType]
	registryMu.RUnlock()
	if f != nil {
		return f.RendererFor(params, schemaURL)
	}

	if strings.HasPrefix(mediaType, "text/") {
		return Text
	}
	return nil
}

// lookupByFilename returns the registered Format for an exact basename
// match, or nil. Exported via Detect's filename lookup.
func lookupByFilename(base string) *Format {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registryByFilename[base]
}

// lookupByExtension returns the registered Format for a lowercase
// extension (with leading dot), or nil.
func lookupByExtension(ext string) *Format {
	registryMu.RLock()
	defer registryMu.RUnlock()
	return registryByExt[ext]
}

// matchPath walks every registered Format's MatchPath callback and
// returns the first matching Format, or nil.
func matchPath(path string) *Format {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, f := range registry {
		if f.MatchPath != nil && f.MatchPath(path) {
			return f
		}
	}
	return nil
}

// matchFilenameFallback invokes each registered filename fallback in
// registration order; the first that returns ok wins.
func matchFilenameFallback(name string) (string, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, fn := range filenameFallbacks {
		if ct, ok := fn(name); ok {
			return ct, true
		}
	}
	return "", false
}

// matchMagic walks every registered Format's MagicBytes and returns the
// first Format whose prefix matches peek, or nil.
func matchMagic(peek []byte) *Format {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, f := range registry {
		for _, m := range f.MagicBytes {
			if len(peek) >= len(m) && bytesEqualPrefix(peek, m) {
				return f
			}
		}
	}
	return nil
}

// matchDetect walks every registered Format's Detect callback in
// registration order and returns the first match, or nil.
func matchDetect(peek []byte) *Format {
	registryMu.RLock()
	defer registryMu.RUnlock()
	for _, f := range registry {
		if f.Detect != nil && f.Detect(peek) {
			return f
		}
	}
	return nil
}

func bytesEqualPrefix(b, prefix []byte) bool {
	for i, c := range prefix {
		if b[i] != c {
			return false
		}
	}
	return true
}

// dotExt returns the lowercase extension (including the leading dot) of
// name, or "" if name has no extension.
func dotExt(name string) string {
	return strings.ToLower(filepath.Ext(name))
}
