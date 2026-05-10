package stripes

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

func TestCodeNoColor(t *testing.T) {
	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	plain := &Styles{Indent: "  "}
	var buf bytes.Buffer
	Code("Go")(&buf, strings.NewReader(src), plain)
	got := buf.String()
	want := strings.TrimRight(src, "\n")
	if got != want {
		t.Fatalf("color-off output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestCodeColorEmitsANSI(t *testing.T) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	t.Cleanup(func() { lipgloss.SetColorProfile(termenv.Ascii) })

	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	var buf bytes.Buffer
	Code("Go")(&buf, strings.NewReader(src), DefaultStyles)

	if !bytes.Contains(buf.Bytes(), []byte{0x1b}) {
		t.Fatalf("expected ANSI escapes, got plain output: %q", buf.String())
	}
	if got := ansi.Strip(buf.String()); !strings.Contains(got, "package main") {
		t.Fatalf("stripped output missing source: %q", got)
	}
}

func TestCodeUnknownLangAnalyseFallback(t *testing.T) {
	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	plain := &Styles{Indent: "  "}
	var buf bytes.Buffer
	Code("")(&buf, strings.NewReader(src), plain)
	if got := buf.String(); !strings.Contains(got, "package main") {
		t.Fatalf("output missing input: %q", got)
	}
}

func TestCodeBogusLangFallsThrough(t *testing.T) {
	src := "totally not source code, no language\n"
	plain := &Styles{Indent: "  "}
	var buf bytes.Buffer
	Code("not-a-real-lexer")(&buf, strings.NewReader(src), plain)
	if got := buf.String(); !strings.Contains(got, "totally not source") {
		t.Fatalf("output missing input: %q", got)
	}
}

// TestProtoLexerTokens locks down the enhanced Protocol Buffer lexer:
//   - the dispatch in resolveLexer routes any of the proto aliases to it
//   - keywords absent from chroma's embedded lexer (syntax, reserved) are
//     tagged as Keyword
//   - "required" — proto2-only — is no longer a keyword
//   - built-in scalar types emit KeywordPseudo (light blue) instead of
//     KeywordType (which falls back to keyword red in github-dark)
//   - the type name following message/enum/service is plain Name so the
//     keyword carries the visual weight alone
//   - qualified type references like google.protobuf.Struct split into
//     a NameDecorator path prefix and a plain Name leaf
//   - package paths emit NameDecorator so they read in the same purple
//     as imported type prefixes
//   - the keyword regexes use a (?<!\.) lookbehind so dotted
//     continuations like ".repeated.min_items" inside option paths
//     don't accidentally hit a keyword match
func TestProtoLexerTokens(t *testing.T) {
	for _, alias := range []string{"Protocol Buffer", "protobuf", "proto"} {
		if got := resolveLexer(alias, nil); got != protoLexer {
			t.Fatalf("resolveLexer(%q) returned chroma's default lexer instead of protoLexer", alias)
		}
	}

	src := `syntax = "proto3";
package chaotic.auth.v1;
message Foo {
  reserved 7;
  string name = 1;
  required int32 legacy = 2;
  google.protobuf.Struct payload = 3;
}
enum Color {
  COLOR_UNSPECIFIED = 0;
}
service Greeter {}
message AllOfSchema {
  repeated Schema schemas = 1 [
    (buf.validate.field).repeated.min_items = 1
  ];
}
`
	iter, err := protoLexer.Tokenise(nil, src)
	if err != nil {
		t.Fatalf("tokenise: %v", err)
	}
	got := map[string]chroma.TokenType{}
	for tok := iter(); tok != chroma.EOF; tok = iter() {
		v := strings.TrimSpace(tok.Value)
		if v == "" {
			continue
		}
		if _, seen := got[v]; !seen {
			got[v] = tok.Type
		}
	}

	want := map[string]chroma.TokenType{
		"syntax":             chroma.Keyword,
		"reserved":           chroma.Keyword,
		"repeated":           chroma.Keyword,
		"required":           chroma.Name, // proto2-only, no longer special
		"message":            chroma.KeywordDeclaration,
		"enum":               chroma.KeywordDeclaration,
		"service":            chroma.KeywordDeclaration,
		"Foo":                chroma.Name,
		"Color":              chroma.Name,
		"Greeter":            chroma.Name,
		"string":             chroma.KeywordPseudo,
		"int32":              chroma.KeywordPseudo,
		"chaotic.auth.v1":    chroma.NameDecorator,
		"google.protobuf.":   chroma.NameDecorator,
		"Struct":             chroma.Name,
		"buf.validate.field": chroma.Name, // no CamelCase leaf — stays plain
		"repeated.min_items": chroma.Name, // .repeated continues a path; lookbehind blocks the keyword match
	}
	for v, wantType := range want {
		if got[v] != wantType {
			t.Errorf("token %q = %s, want %s", v, got[v], wantType)
		}
	}
}
