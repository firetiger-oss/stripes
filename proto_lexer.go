package stripes

import "github.com/alecthomas/chroma/v2"

// protoLexer is an enhanced Protocol Buffer lexer used in place of
// chroma's embedded lexers/embedded/protocol_buffer.xml.
//
// Differences from chroma's stock lexer:
//   - Adds syntax, edition, reserved, weak, public, stream as keywords;
//     drops required (proto3 removed it). All keyword/type/constant
//     rules use a (?<!\.) lookbehind so that dotted continuations like
//     ".repeated.min_items" inside option paths don't accidentally hit
//     a keyword match.
//   - Built-in scalar types (int32, string, bool, map, ...) emit
//     KeywordPseudo so github-dark colors them light blue instead of
//     reusing the keyword red.
//   - Qualified type references like google.protobuf.Struct split into
//     NameDecorator (the lowercase path prefix) + Name (the CamelCase
//     leaf). Package names emit NameDecorator too, so package paths
//     read as the same purple as imported type prefixes.
//   - The type names following message/enum/service/extend/group are
//     emitted as Name (not NameClass) so the keyword alone carries the
//     visual weight — same convention as VS Code and GitHub's renderer.
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
			{Pattern: `(?<!\.)\b(syntax|edition|import|weak|public|option|extensions|reserved|to|max|stream|returns|rpc|oneof|optional|repeated|default|packed|ctype)\b`, Type: chroma.Keyword},
			{Pattern: `(?<!\.)\b(int32|int64|uint32|uint64|sint32|sint64|fixed32|fixed64|sfixed32|sfixed64|float|double|bool|string|bytes|map)\b`, Type: chroma.KeywordPseudo},
			{Pattern: `(?<!\.)\b(true|false|inf|nan)\b`, Type: chroma.KeywordConstant},
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
			// Qualified type reference: lowercase.path.CamelCase. Splits
			// into a purple namespace prefix and a plain leaf type.
			{Pattern: `([a-z_]\w*(?:\.[a-z_]\w*)*\.)([A-Z]\w*)\b`, Type: chroma.ByGroups(chroma.NameDecorator, chroma.Name)},
			{Pattern: `([a-zA-Z_][\w.]*)([ \t]*)(=)`, Type: chroma.ByGroups(chroma.Name, chroma.Text, chroma.Operator)},
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.Name},
			{Pattern: `\.`, Type: chroma.Punctuation},
		},
		"package": {
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.NameDecorator, Mutator: chroma.Pop(1)},
			chroma.Default(chroma.Pop(1)),
		},
		"typename": {
			{Pattern: `[a-zA-Z_]\w*`, Type: chroma.Name, Mutator: chroma.Pop(1)},
			chroma.Default(chroma.Pop(1)),
		},
	}
}
