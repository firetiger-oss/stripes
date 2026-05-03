package stripes

import (
	"bytes"
	"net/http"
	"path/filepath"
	"strings"
)

// Detect resolves a content type for a stream.
//
// The lookup order is:
//  1. Filename extension (when name has a recognized extension).
//  2. Magic-byte sniffing of peek (first ~512 bytes of the stream).
//  3. net/http.DetectContentType fallback.
//  4. "text/plain" if nothing else matched.
//
// The returned string is a MIME media type compatible with [Func].
// Empty name and nil peek both return "text/plain".
func Detect(name string, peek []byte) string {
	if ct := detectByExtension(name); ct != "" {
		return ct
	}
	if ct := detectByContent(peek); ct != "" {
		return ct
	}
	if len(peek) > 0 {
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

func detectByExtension(name string) string {
	switch filepath.Base(name) {
	case "Dockerfile", "Containerfile":
		return "text/x-dockerfile"
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		return "application/json"
	case ".yaml", ".yml":
		return "application/yaml"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".csv":
		return "text/csv"
	case ".dockerfile":
		return "text/x-dockerfile"
	case ".txt":
		return "text/plain"
	}
	return ""
}

func detectByContent(peek []byte) string {
	trimmed := bytes.TrimLeft(peek, " \t\r\n")
	if len(trimmed) == 0 {
		return ""
	}

	switch trimmed[0] {
	case '{', '[':
		return "application/json"
	case '<':
		lower := bytes.ToLower(trimmed)
		switch {
		case bytes.HasPrefix(lower, []byte("<?xml")):
			return "application/xml"
		case bytes.HasPrefix(lower, []byte("<!doctype html")),
			bytes.HasPrefix(lower, []byte("<html")):
			return "text/html"
		}
		return "application/xml"
	case '-':
		if bytes.HasPrefix(trimmed, []byte("---")) {
			return "application/yaml"
		}
	}

	if looksLikeDockerfile(trimmed) {
		return "text/x-dockerfile"
	}
	if looksLikeYAML(trimmed) {
		return "application/yaml"
	}
	return ""
}

func looksLikeYAML(b []byte) bool {
	const maxScan = 4
	for i, line := 0, []byte(nil); i < maxScan; i++ {
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			line, b = b, nil
		} else {
			line, b = b[:nl], b[nl+1:]
		}
		line = bytes.TrimRight(line, " \t\r")
		if len(line) == 0 || line[0] == '#' {
			if b == nil {
				break
			}
			continue
		}
		if !isASCIIIdentStart(line[0]) {
			return false
		}
		colon := bytes.IndexByte(line, ':')
		if colon <= 0 {
			return false
		}
		for j := 0; j < colon; j++ {
			c := line[j]
			if !isASCIIIdent(c) {
				return false
			}
		}
		after := line[colon+1:]
		if len(after) == 0 || after[0] == ' ' || after[0] == '\t' {
			return true
		}
		return false
	}
	return false
}

func isASCIIIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isASCIIIdent(c byte) bool {
	return isASCIIIdentStart(c) || (c >= '0' && c <= '9') || c == '-' || c == '.'
}

// looksLikeDockerfile inspects the first few non-blank lines of peek
// for Dockerfile-shaped content: a `# syntax=` / `# escape=` parser
// directive, or a leading FROM/ARG instruction.
func looksLikeDockerfile(b []byte) bool {
	const maxScan = 4
	for i, line := 0, []byte(nil); i < maxScan; i++ {
		nl := bytes.IndexByte(b, '\n')
		if nl < 0 {
			line, b = b, nil
		} else {
			line, b = b[:nl], b[nl+1:]
		}
		line = bytes.TrimRight(line, " \t\r")
		if len(line) == 0 {
			if b == nil {
				break
			}
			continue
		}
		if line[0] == '#' {
			rest := bytes.TrimLeft(line[1:], " \t")
			lower := bytes.ToLower(rest)
			if bytes.HasPrefix(lower, []byte("syntax=")) ||
				bytes.HasPrefix(lower, []byte("escape=")) ||
				bytes.HasPrefix(lower, []byte("check=")) {
				return true
			}
			if b == nil {
				break
			}
			continue
		}
		fields := bytes.Fields(line)
		if len(fields) == 0 {
			if b == nil {
				break
			}
			continue
		}
		head := bytes.ToUpper(fields[0])
		switch string(head) {
		case "FROM", "ARG":
			return true
		}
		return false
	}
	return false
}
