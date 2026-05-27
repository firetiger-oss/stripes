package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

// decompress wraps r in a decoder selected by encoding (the value of
// ObjectInfo.ContentEncoding). Empty and "identity" return a no-op
// closer over r. Unknown encodings return an error. The caller retains
// ownership of r; closing the returned ReadCloser closes only the
// decoder's resources.
func decompress(r io.Reader, encoding string) (io.ReadCloser, error) {
	switch strings.ToLower(strings.TrimSpace(encoding)) {
	case "", "identity":
		return io.NopCloser(r), nil
	case "gzip", "x-gzip":
		return gzip.NewReader(r)
	case "zstd":
		zr, err := zstd.NewReader(r)
		if err != nil {
			return nil, err
		}
		return zr.IOReadCloser(), nil
	case "zlib":
		return zlib.NewReader(r)
	case "flate", "deflate":
		return flate.NewReader(r), nil
	case "snappy":
		return io.NopCloser(snappy.NewReader(r)), nil
	case "s2":
		return io.NopCloser(s2.NewReader(r)), nil
	case "lz4":
		return io.NopCloser(lz4.NewReader(r)), nil
	case "br", "brotli":
		return io.NopCloser(brotli.NewReader(r)), nil
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", encoding)
	}
}

// encodingSuffixes maps the recognised compression file extensions
// to the wire encoding [decompress] understands. Kept ordered (and
// case-folded at use) so suffix matching is deterministic.
var encodingSuffixes = [...]struct{ ext, enc string }{
	{".gz", "gzip"},
	{".zst", "zstd"},
	{".zstd", "zstd"},
	{".br", "brotli"},
	{".lz4", "lz4"},
	{".sz", "snappy"},
	{".snappy", "snappy"},
}

// effectiveEncoding picks the wire compression for a payload. The
// explicit Content-Encoding from storage wins (S3 servers, HTTP
// hops, etc. that bother to set it); when absent, fall back to a
// recognised compression suffix on the filename so a vanilla
// .log.gz upload still decompresses cleanly.
func effectiveEncoding(name, contentEncoding string) string {
	e := strings.ToLower(strings.TrimSpace(contentEncoding))
	if e != "" && e != "identity" {
		return e
	}
	lower := strings.ToLower(name)
	for _, s := range encodingSuffixes {
		if strings.HasSuffix(lower, s.ext) {
			return s.enc
		}
	}
	return ""
}

// stripEncodingSuffix removes a recognised compression suffix from
// name so subsequent content-type detection sees the inner
// extension. "foo.log.gz" becomes "foo.log" — important because
// `.log` itself is unclaimed by any format and the per-format
// Detect callbacks fire on the decompressed bytes.
func stripEncodingSuffix(name string) string {
	lower := strings.ToLower(name)
	for _, s := range encodingSuffixes {
		if strings.HasSuffix(lower, s.ext) {
			return name[:len(name)-len(s.ext)]
		}
	}
	return name
}
