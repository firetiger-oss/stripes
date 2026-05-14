package stripes

import (
	"net/http"
	"path/filepath"
	"strings"
)

// Detect resolves a content type for a stream.
//
// The lookup order is:
//  1. Exact filename match against registered [Format.Filenames].
//  2. Registered [Format.MatchPath] callbacks (path-aware rules); these
//     run before the extension match so e.g. vendor/modules.txt is not
//     swallowed by a ".txt" registration.
//  3. File-extension match against registered [Format.Extensions].
//  4. Registered filename fallbacks (see [RegisterFilenameFallback]).
//  5. Magic-byte prefixes from registered [Format.MagicBytes].
//  6. Registered [Format.Detect] content-shape heuristics.
//  7. net/http.DetectContentType fallback.
//  8. "text/plain" if nothing else matched.
//
// The returned string is a MIME media type compatible with [Func]. Only
// formats whose sub-packages have been imported participate; an empty
// registry resolves everything to "text/plain" (or the http fallback).
// Empty name and nil peek both return "text/plain".
func Detect(name string, peek []byte) string {
	if base := filepath.Base(name); base != "" && base != "." && base != "/" {
		if f := lookupByFilename(base); f != nil {
			return f.ContentType
		}
	}
	if name != "" {
		if f := matchPath(name); f != nil {
			return f.ContentType
		}
	}
	if ext := dotExt(name); ext != "" {
		if f := lookupByExtension(ext); f != nil {
			return f.ContentType
		}
	}
	if name != "" {
		if ct, ok := matchFilenameFallback(name); ok {
			return ct
		}
	}
	if len(peek) > 0 {
		if f := matchMagic(peek); f != nil {
			return f.ContentType
		}
		if f := matchDetect(peek); f != nil {
			return f.ContentType
		}
		ct := http.DetectContentType(peek)
		if i := strings.IndexByte(ct, ';'); i >= 0 {
			ct = ct[:i]
		}
		ct = strings.TrimSpace(ct)
		switch ct {
		case "text/xml":
			return "application/xml"
		case "application/json":
			return "application/json"
		case "text/html":
			return "text/html"
		}
		if strings.HasPrefix(ct, "text/") {
			return "text/plain"
		}
	}
	return "text/plain"
}
