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
