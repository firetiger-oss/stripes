package stripes

import (
	"bytes"
	"fmt"
	"io"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/formatters"
	chromalexers "github.com/alecthomas/chroma/v2/lexers"
	chromastyles "github.com/alecthomas/chroma/v2/styles"
)

// Code returns a [Renderer] that highlights source code using the chroma
// lexer named lang. When lang is empty the renderer falls back to chroma's
// content-based language detection; if no lexer can be resolved the input
// is written verbatim.
func Code(lang string) Renderer {
	return func(w io.Writer, r io.Reader, styles *Styles) {
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
func highlightCode(w io.Writer, src []byte, lang string, styles *Styles) {
	lex := resolveLexer(lang, src)
	color := stylesEmitANSI(styles)

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
func chromaStyleName(styles *Styles) string {
	if styles != nil && styles.CodeStyle != "" {
		return styles.CodeStyle
	}
	return "github-dark"
}
