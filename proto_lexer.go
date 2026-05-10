package stripes

import "github.com/alecthomas/chroma/v2"

// protoLexer is an enhanced Protocol Buffer lexer used in place of
// chroma's embedded lexers/embedded/protocol_buffer.xml. It adds
// recognition of `syntax`, `edition`, `reserved`, `weak`, `public`,
// and `stream` as keywords, and treats `inf`/`nan` as KeywordConstant.
//
// The type names following message/enum/service/extend/group are
// emitted as Name (not NameClass) so the keyword alone carries the
// visual weight — same convention as VS Code and GitHub's renderer.
//
// The lexer is constructed at package load. RegexLexer compiles its
// rules lazily on first use and is safe for concurrent callers.
var protoLexer = chroma.MustNewLexer(
	&chroma.Config{
		Name:      "Protocol Buffer",
		Aliases:   []string{"protobuf", "proto"},
		Filenames: []string{"*.proto"},
	},
	protoLexerRules,
)

func protoLexerRules() chroma.Rules {
	return chroma.Rules{
		"root": {
			{Pattern: `[ \t\r\n]+`, Type: chroma.Text},
			{Pattern: `//[^\n]*`, Type: chroma.CommentSingle},
			{Pattern: `/\*(.|\n)*?\*/`, Type: chroma.CommentMultiline},
			{Pattern: `[,;{}\[\]()<>]`, Type: chroma.Punctuation},
			{Pattern: `\b(syntax|edition|import|weak|public|option|extensions|reserved|to|max|stream|returns|rpc|oneof|optional|required|repeated|default|packed|ctype)\b`, Type: chroma.Keyword},
			{Pattern: `\b(int32|int64|uint32|uint64|sint32|sint64|fixed32|fixed64|sfixed32|sfixed64|float|double|bool|string|bytes|map)\b`, Type: chroma.KeywordType},
			{Pattern: `\b(true|false|inf|nan)\b`, Type: chroma.KeywordConstant},
			{Pattern: `(package)(\s+)`, Type: chroma.ByGroups(chroma.KeywordNamespace, chroma.Text), Mutator: chroma.Push("package")},
			{Pattern: `(message|enum|service|extend|group)(\s+)`, Type: chroma.ByGroups(chroma.KeywordDeclaration, chroma.Text), Mutator: chroma.Push("typename")},
			{Pattern: `"(\\.|[^"\\])*"`, Type: chroma.LiteralString},
			{Pattern: `'(\\.|[^'\\])*'`, Type: chroma.LiteralString},
			{Pattern: `(\d+\.\d*|\.\d+|\d+)[eE][+-]?\d+[LlUu]*`, Type: chroma.LiteralNumberFloat},
			{Pattern: `(\d+\.\d*|\.\d+|\d+[fF])[fF]?`, Type: chroma.LiteralNumberFloat},
			{Pattern: `0x[0-9a-fA-F]+[LlUu]*`, Type: chroma.LiteralNumberHex},
			{Pattern: `0[0-7]+[LlUu]*`, Type: chroma.LiteralNumberOct},
			{Pattern: `\d+[LlUu]*`, Type: chroma.LiteralNumberInteger},
			{Pattern: `[+\-=]`, Type: chroma.Operator},
			{Pattern: `([a-zA-Z_][\w.]*)([ \t]*)(=)`, Type: chroma.ByGroups(chroma.Name, chroma.Text, chroma.Operator)},
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.Name},
		},
		"package": {
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.NameNamespace, Mutator: chroma.Pop(1)},
			chroma.Default(chroma.Pop(1)),
		},
		"typename": {
			{Pattern: `[a-zA-Z_]\w*`, Type: chroma.Name, Mutator: chroma.Pop(1)},
			chroma.Default(chroma.Pop(1)),
		},
	}
}
