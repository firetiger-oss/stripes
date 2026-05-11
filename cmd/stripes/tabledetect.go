package main

import (
	"bytes"
	"path/filepath"
	"strings"

	"github.com/firetiger-oss/stripes"
)

// detectRowFlavor picks one of csvTable / tsvTable / jsonlTable based on
// the source filename's extension first, then a best-effort content sniff
// of peek. Defaults to csvTable when neither signal is conclusive.
func detectRowFlavor(name string, peek []byte) stripes.Renderer {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".csv":
		return csvTable
	case ".tsv", ".tab":
		return tsvTable
	case ".jsonl", ".ndjson":
		return jsonlTable
	}

	trimmed := bytes.TrimLeft(peek, " \t\r\n")
	if len(trimmed) == 0 {
		return csvTable
	}

	// JSONL: first non-blank char is `{` and there is at least one
	// `}` followed by a newline then another `{` — i.e. multiple objects
	// on separate lines. A single multi-line JSON object would fail this
	// pattern and fall through to CSV.
	if trimmed[0] == '{' && bytes.Contains(trimmed, []byte("}\n{")) {
		return jsonlTable
	}

	// TSV: first non-blank line contains a tab.
	firstLine := trimmed
	if nl := bytes.IndexByte(trimmed, '\n'); nl >= 0 {
		firstLine = trimmed[:nl]
	}
	if bytes.IndexByte(firstLine, '\t') >= 0 {
		return tsvTable
	}

	return csvTable
}
