package code

import (
	"bytes"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/firetiger-oss/stripes"
	"github.com/muesli/termenv"
)

func TestCodeNoColor(t *testing.T) {
	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	plain := &stripes.Styles{Indent: "  "}
	var buf bytes.Buffer
	New("Go")(&buf, strings.NewReader(src), plain)
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
	New("Go")(&buf, strings.NewReader(src), stripes.DefaultStyles)

	if !bytes.Contains(buf.Bytes(), []byte{0x1b}) {
		t.Fatalf("expected ANSI escapes, got plain output: %q", buf.String())
	}
	if got := ansi.Strip(buf.String()); !strings.Contains(got, "package main") {
		t.Fatalf("stripped output missing source: %q", got)
	}
}

func TestCodeUnknownLangAnalyseFallback(t *testing.T) {
	src := "package main\n\nfunc main() { println(\"hi\") }\n"
	plain := &stripes.Styles{Indent: "  "}
	var buf bytes.Buffer
	New("")(&buf, strings.NewReader(src), plain)
	if got := buf.String(); !strings.Contains(got, "package main") {
		t.Fatalf("output missing input: %q", got)
	}
}

func TestCodeBogusLangFallsThrough(t *testing.T) {
	src := "totally not source code, no language\n"
	plain := &stripes.Styles{Indent: "  "}
	var buf bytes.Buffer
	New("not-a-real-lexer")(&buf, strings.NewReader(src), plain)
	if got := buf.String(); !strings.Contains(got, "totally not source") {
		t.Fatalf("output missing input: %q", got)
	}
}

// TestProtoLexerTokens locks down the enhanced Protocol Buffer lexer:
//   - the dispatch in resolveLexer routes any of the proto aliases to it
//   - keywords absent from chroma's embedded lexer (syntax, reserved) are
//     tagged as Keyword
//   - "required" — proto2-only — is no longer a keyword
//   - built-in scalar types and PascalCase user-defined types both emit
//     KeywordPseudo (light blue), including the type name following
//     message/enum/service/extend/group and the leaf of qualified
//     references like google.protobuf.Struct. SCREAMING_SNAKE_CASE
//     identifiers (typical proto enum values) stay plain.
//   - qualified type references split into a NameDecorator path prefix
//     and a KeywordPseudo leaf
//   - package paths emit NameDecorator
//   - the keyword regexes use a (?<!\.) lookbehind so dotted
//     continuations like ".repeated.min_items" inside option paths
//     don't accidentally hit a keyword match
//   - option names (bare or parenthesized) and TextProto-style keys
//     followed by ":" are tagged NameTag (green)
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
  ModelConfig model = 3;
  google.protobuf.Struct payload = 4;
}
enum Color {
  COLOR_UNSPECIFIED = 0;
}
service Greeter {
  rpc GetAgent(GetAgentRequest) returns (GetAgentResponse) {
    option idempotency_level = NO_SIDE_EFFECTS;
    option (google.api.http) = {
      post: "/v1/agents"
      body: "*"
    };
  }
}
message AllOfSchema {
  repeated Schema schemas = 1 [
    (google.api.field_behavior) = REQUIRED,
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
		"syntax":                    chroma.Keyword,
		"reserved":                  chroma.Keyword,
		"repeated":                  chroma.Keyword,
		"required":                  chroma.Name, // proto2-only, no longer special
		"message":                   chroma.KeywordDeclaration,
		"enum":                      chroma.KeywordDeclaration,
		"service":                   chroma.KeywordDeclaration,
		"Foo":                       chroma.KeywordPseudo,
		"Color":                     chroma.KeywordPseudo,
		"Greeter":                   chroma.KeywordPseudo,
		"ModelConfig":               chroma.KeywordPseudo, // user-defined PascalCase type
		"Schema":                    chroma.KeywordPseudo,
		"GetAgent":                  chroma.KeywordPseudo,
		"GetAgentRequest":           chroma.KeywordPseudo,
		"AllOfSchema":               chroma.KeywordPseudo,
		"string":                    chroma.KeywordPseudo,
		"int32":                     chroma.KeywordPseudo,
		"COLOR_UNSPECIFIED":         chroma.Name, // SCREAMING_SNAKE stays plain
		"NO_SIDE_EFFECTS":           chroma.Name,
		"chaotic.auth.v1":           chroma.NameDecorator,
		"google.protobuf.":          chroma.NameDecorator,
		"Struct":                    chroma.KeywordPseudo, // qualified-name leaf
		"repeated.min_items":        chroma.Name,          // .repeated continues a path; lookbehind blocks the keyword match
		"idempotency_level":         chroma.NameTag,       // bare option name
		"google.api.http":           chroma.NameTag,       // parenthesised option in `option (...)`
		"google.api.field_behavior": chroma.NameTag,       // parenthesised option in field-option `[...]`
		"buf.validate.field":        chroma.NameTag,       // parenthesised extension reference, even nested in `[...]`
		"post":                      chroma.NameTag,       // TextProto key
		"body":                      chroma.NameTag,
	}
	for v, wantType := range want {
		if got[v] != wantType {
			t.Errorf("token %q = %s, want %s", v, got[v], wantType)
		}
	}
}
