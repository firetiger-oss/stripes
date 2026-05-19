package main

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/flate"
	"github.com/klauspost/compress/gzip"
	"github.com/klauspost/compress/s2"
	"github.com/klauspost/compress/snappy"
	"github.com/klauspost/compress/zlib"
	"github.com/klauspost/compress/zstd"
	"github.com/pierrec/lz4/v4"
)

type encoder func(t testing.TB, payload []byte) []byte

var codecs = []struct {
	name     string
	encoding string
	encode   encoder
}{
	{"Gzip", "gzip", encodeGzip},
	{"GzipAliasXGzip", "x-gzip", encodeGzip},
	{"GzipUppercase", "GZIP", encodeGzip},
	{"Zstd", "zstd", encodeZstd},
	{"Zlib", "zlib", encodeZlib},
	{"Flate", "flate", encodeFlate},
	{"FlateAliasDeflate", "deflate", encodeFlate},
	{"Snappy", "snappy", encodeSnappy},
	{"S2", "s2", encodeS2},
	{"LZ4", "lz4", encodeLZ4},
	{"Brotli", "br", encodeBrotli},
	{"BrotliAlias", "brotli", encodeBrotli},
}

func TestDecompress(t *testing.T) {
	payload := []byte("hello world\n")

	for _, c := range codecs {
		t.Run(c.name, func(t *testing.T) {
			compressed := c.encode(t, payload)
			rc, err := decompress(bytes.NewReader(compressed), c.encoding)
			if err != nil {
				t.Fatalf("decompress(%q): %v", c.encoding, err)
			}
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("decoded %q, want %q", got, payload)
			}
		})
	}
}

func TestDecompressIdentity(t *testing.T) {
	payload := []byte("hello world\n")
	for _, enc := range []string{"", "identity", " IDENTITY ", "Identity"} {
		t.Run(enc, func(t *testing.T) {
			rc, err := decompress(bytes.NewReader(payload), enc)
			if err != nil {
				t.Fatalf("decompress(%q): %v", enc, err)
			}
			got, err := io.ReadAll(rc)
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			if err := rc.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			if !bytes.Equal(got, payload) {
				t.Fatalf("got %q, want %q", got, payload)
			}
		})
	}
}

func TestDecompressUnsupported(t *testing.T) {
	rc, err := decompress(strings.NewReader("anything"), "xz")
	if err == nil {
		t.Fatalf("expected error, got reader %v", rc)
	}
	if rc != nil {
		t.Fatalf("expected nil reader, got %v", rc)
	}
	if !strings.Contains(err.Error(), `unsupported content encoding "xz"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func encodeGzip(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func encodeZstd(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := zstd.NewWriter(&buf)
	if err != nil {
		t.Fatalf("zstd new: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zstd write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zstd close: %v", err)
	}
	return buf.Bytes()
}

func encodeZlib(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("zlib write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("zlib close: %v", err)
	}
	return buf.Bytes()
}

func encodeFlate(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w, err := flate.NewWriter(&buf, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate new: %v", err)
	}
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}
	return buf.Bytes()
}

func encodeSnappy(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := snappy.NewBufferedWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("snappy write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("snappy close: %v", err)
	}
	return buf.Bytes()
}

func encodeS2(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := s2.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("s2 write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("s2 close: %v", err)
	}
	return buf.Bytes()
}

func encodeLZ4(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := lz4.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("lz4 write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("lz4 close: %v", err)
	}
	return buf.Bytes()
}

func encodeBrotli(t testing.TB, payload []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := brotli.NewWriter(&buf)
	if _, err := w.Write(payload); err != nil {
		t.Fatalf("brotli write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("brotli close: %v", err)
	}
	return buf.Bytes()
}

var benchPayload = bytes.Repeat([]byte("the quick brown fox jumps over the lazy dog\n"), 96)

func benchmarkDecompress(b *testing.B, encoding string, enc encoder) {
	b.Helper()
	compressed := enc(b, benchPayload)
	b.ReportAllocs()
	b.SetBytes(int64(len(benchPayload)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rc, err := decompress(bytes.NewReader(compressed), encoding)
		if err != nil {
			b.Fatalf("decompress: %v", err)
		}
		if _, err := io.Copy(io.Discard, rc); err != nil {
			b.Fatalf("copy: %v", err)
		}
		if err := rc.Close(); err != nil {
			b.Fatalf("close: %v", err)
		}
	}
}

func BenchmarkDecompressIdentity(b *testing.B) {
	benchmarkDecompress(b, "", func(_ testing.TB, payload []byte) []byte { return payload })
}
func BenchmarkDecompressGzip(b *testing.B)   { benchmarkDecompress(b, "gzip", encodeGzip) }
func BenchmarkDecompressZstd(b *testing.B)   { benchmarkDecompress(b, "zstd", encodeZstd) }
func BenchmarkDecompressZlib(b *testing.B)   { benchmarkDecompress(b, "zlib", encodeZlib) }
func BenchmarkDecompressFlate(b *testing.B)  { benchmarkDecompress(b, "flate", encodeFlate) }
func BenchmarkDecompressSnappy(b *testing.B) { benchmarkDecompress(b, "snappy", encodeSnappy) }
func BenchmarkDecompressS2(b *testing.B)     { benchmarkDecompress(b, "s2", encodeS2) }
func BenchmarkDecompressLZ4(b *testing.B)    { benchmarkDecompress(b, "lz4", encodeLZ4) }
func BenchmarkDecompressBrotli(b *testing.B) { benchmarkDecompress(b, "br", encodeBrotli) }
