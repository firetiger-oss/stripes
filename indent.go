package stripes

import (
	"bytes"
	"io"
	"strings"
)

func NewPrefixWriter(writer io.Writer, prefix string) io.Writer {
	return &prefixWriter{
		writer:  writer,
		prefix:  []byte(prefix),
		newLine: true,
	}
}

type prefixWriter struct {
	writer  io.Writer
	prefix  []byte
	newLine bool
}

func (pw *prefixWriter) Write(data []byte) (n int, err error) {
	for n < len(data) {
		if pw.newLine {
			pw.newLine = false
			pw.writer.Write(pw.prefix)
		}

		i := bytes.IndexByte(data[n:], '\n')
		if i < 0 {
			pw.writer.Write(data[n:])
			n = len(data)
			break
		}

		pw.writer.Write(data[n : n+i+1])
		pw.newLine = true
		n += i + 1
	}
	return n, nil
}

func (pw *prefixWriter) WriteString(data string) (n int, err error) {
	for n < len(data) {
		if pw.newLine {
			pw.newLine = false
			pw.writer.Write(pw.prefix)
		}

		i := strings.IndexByte(data[n:], '\n')
		if i < 0 {
			io.WriteString(pw.writer, data[n:])
			n = len(data)
			break
		}

		io.WriteString(pw.writer, data[n:n+i+1])
		pw.newLine = true
		n += i + 1
	}
	return n, nil
}

var _ io.StringWriter = (*prefixWriter)(nil)
