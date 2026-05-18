// Package code registers the source-code renderer with the stripes
// registry. Import for side effects to enable text/x-source-code
// support and chroma's filename-based language detection in
// [stripes.Detect]:
//
//	import _ "github.com/firetiger-oss/stripes/code"
package code

import (
	"bytes"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	chromalexers "github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
	"github.com/firetiger-oss/stripes"
)

func init() {
	stripes.Register(stripes.Format{
		Name:        "code",
		ContentType: "text/x-source-code",
		RendererFor: func(params map[string]string, _ string) stripes.Renderer {
			return New(params["lang"])
		},
	})
	// Plug chroma's filename-based language detection into stripes.Detect
	// so a recognised source file resolves to
	// text/x-source-code; lang=<chroma lexer name>. Extensions are not
	// registered on the Format above because each needs a distinct lang
	// parameter, which only the fallback can supply.
	stripes.RegisterFilenameFallback(func(name string) (string, bool) {
		base := filepath.Base(name)
		switch strings.ToLower(filepath.Ext(base)) {
		case ".wat", ".wast":
			return mime.FormatMediaType("text/x-source-code", map[string]string{"lang": "wat"}), true
		case ".tfvars":
			return mime.FormatMediaType("text/x-source-code", map[string]string{"lang": "terraform"}), true
		}
		if lex := chromalexers.Match(base); lex != nil {
			return mime.FormatMediaType("text/x-source-code", map[string]string{"lang": lex.Config().Name}), true
		}
		return "", false
	})
}

// New returns a [stripes.Renderer] that highlights source code using
// the chroma lexer named lang. When lang is empty the renderer falls
// back to chroma's content-based language detection; if no lexer can be
// resolved the input is written verbatim.
func New(lang string) stripes.Renderer {
	return func(w io.Writer, r io.Reader, styles *stripes.Styles) {
		src, err := io.ReadAll(r)
		if err != nil {
			fmt.Fprintf(w, "ERROR: %s\n", err)
			return
		}
		highlightCode(w, src, lang, styles)
	}
}

// highlightCode resolves a chroma lexer for src (preferring lang, falling
// back to content-based detection) and writes a styled rendering to w. If
// no lexer can be resolved or color output is disabled, src is written
// verbatim with the trailing newline trimmed.
func highlightCode(w io.Writer, src []byte, lang string, styles *stripes.Styles) {
	lex := resolveLexer(lang, src)
	color := stripes.IsANSIEnabled(styles)

	if lex == nil || !color {
		w.Write(bytes.TrimRight(src, "\n"))
		return
	}

	iter, err := lex.Tokenise(nil, string(src))
	if err != nil {
		io.WriteString(w, strings.TrimRight(string(src), "\n"))
		return
	}
	style := chromastyles.Get(chromaStyleName(styles))
	if style == nil {
		style = chromastyles.Fallback
	}
	fmtr := formatters.Get("terminal256")
	if fmtr == nil {
		fmtr = chroma.Formatter(formatters.Fallback)
	}
	var buf bytes.Buffer
	if err := fmtr.Format(&buf, style, iter); err != nil {
		io.WriteString(w, strings.TrimRight(string(src), "\n"))
		return
	}
	io.WriteString(w, strings.TrimRight(buf.String(), "\n"))
}

func resolveLexer(lang string, src []byte) chroma.Lexer {
	if lang != "" {
		switch strings.ToLower(lang) {
		case "protocol buffer", "protobuf", "proto":
			return protoLexer
		}
		if lex := chromalexers.Get(lang); lex != nil {
			return lex
		}
	}
	if lex := chromalexers.Analyse(string(src)); lex != nil {
		return lex
	}
	return nil
}

// chromaStyleName returns the chroma style name configured on styles, or
// the package default ("github-dark") when unset.
func chromaStyleName(styles *stripes.Styles) string {
	if styles != nil && styles.CodeStyle != "" {
		return styles.CodeStyle
	}
	return "github-dark"
}
