package markdown

import (
	"io"
	"strings"
	"testing"

	"github.com/firetiger-oss/stripes"
)

// benchInput is a mid-size markdown document covering the common element
// mix: headings, paragraphs, lists, tables, fenced code, and a blockquote.
// Roughly 1.2 KB — representative of an LLM reply.
const benchInput = `# Streaming Markdown Benchmark

This is a paragraph with **bold**, *italic*, and ` + "`inline code`" + ` and a
[link](https://example.com/some/destination) embedded in normal prose. It is
long enough to exercise the soft-wrap path at the default 80-column width.

## Sub-heading

- a bullet point
- another one with **emphasis**
- a third with [a link](https://example.com)
- and a fourth that wraps if the width is small enough to force a break

1. ordered first
2. ordered second
3. ordered third

> a blockquote line
> that continues on a second line and wraps if the width is small

| name  | role     | description                              |
|-------|----------|------------------------------------------|
| alice | engineer | implements the streaming renderer        |
| bob   | reviewer | reviews the streaming renderer code path |
| carol | qa       | tests the streaming renderer end to end  |

` + "```go" + `
package main

import "fmt"

func main() {
    fmt.Println("hello, streaming markdown")
}
` + "```" + `

A closing paragraph after the code fence.
`

func BenchmarkRenderBatch(b *testing.B) {
	src := []byte(benchInput)
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 80
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(io.Discard, strings.NewReader(benchInput), styles)
	}
}

func BenchmarkRenderStreamSmallChunks(b *testing.B) {
	src := []byte(benchInput)
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 80
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(io.Discard, &chunkReader{src: src, size: 16}, styles)
	}
}

func BenchmarkRenderStreamByteByByte(b *testing.B) {
	src := []byte(benchInput)
	styles := stripes.DefaultStyles.Clone()
	styles.Width = 80
	b.ReportAllocs()
	b.SetBytes(int64(len(src)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Render(io.Discard, &chunkReader{src: src, size: 1}, styles)
	}
}
