package code

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
//     reusing the keyword red. User-defined types follow the same
//     color: PascalCase identifiers (uppercase first letter with at
//     least one lowercase letter) are tagged KeywordPseudo too,
//     including the type name following message/enum/service/extend/
//     group and the leaf of qualified references like
//     google.protobuf.Struct. SCREAMING_SNAKE_CASE identifiers (typical
//     proto enum values) stay plain so they don't get visually
//     conflated with types.
//   - Qualified type references like google.protobuf.Struct also split
//     into a NameDecorator path prefix (purple in github-dark). Package
//     names emit NameDecorator too, so package paths read in the same
//     purple as imported type prefixes.
//   - Option names following the "option" keyword (bare), parenthesised
//     option/extension references like `(google.api.http)` anywhere
//     they appear (including inside field-option `[...]` blocks), and
//     TextProto-style keys (the LHS of an "ident:" pair inside a {}
//     option value) are tagged NameTag — github-dark green — visually
//     distinct from the type and keyword colors.
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
			// Bare option name: `option idempotency_level`. Coloured as
			// NameTag (green) before the bare-keyword rule sees `option`.
			{Pattern: `(option)(\s+)([a-zA-Z_][\w.]*)`, Type: chroma.ByGroups(chroma.Keyword, chroma.Text, chroma.NameTag)},
			// TextProto-style key inside option {}: `key:` becomes a
			// NameTag. The colon only ever appears in proto IDL inside
			// {}-shaped option values, so a global rule is safe.
			{Pattern: `([a-zA-Z_]\w*)(\s*)(:)`, Type: chroma.ByGroups(chroma.NameTag, chroma.Text, chroma.Punctuation)},
			// Parenthesised option / extension name. Anywhere `(name)`
			// appears with a lowercase-first qualified identifier inside,
			// it's an option name — both `option (google.api.http)` and
			// field-option entries like `[(google.api.field_behavior) =
			// REQUIRED]`. The leading `[a-z_]` stops this from
			// swallowing CamelCase type references in `rpc Method(Request)`.
			{Pattern: `(\()([a-z_][\w.]*)(\))`, Type: chroma.ByGroups(chroma.Punctuation, chroma.NameTag, chroma.Punctuation)},
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
			// Qualified type reference: lowercase.path.CamelCase.
			// Splits into a purple namespace prefix and a leaf type
			// (KeywordPseudo, same colour as built-in types).
			{Pattern: `([a-z_]\w*(?:\.[a-z_]\w*)*\.)([A-Z]\w*)\b`, Type: chroma.ByGroups(chroma.NameDecorator, chroma.KeywordPseudo)},
			// PascalCase identifier — user-defined type reference.
			// Requires at least one lowercase letter so SCREAMING_SNAKE
			// (typical proto enum value) stays plain.
			{Pattern: `[A-Z]\w*[a-z]\w*\b`, Type: chroma.KeywordPseudo},
			{Pattern: `([a-zA-Z_][\w.]*)([ \t]*)(=)`, Type: chroma.ByGroups(chroma.Name, chroma.Text, chroma.Operator)},
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.Name},
			{Pattern: `\.`, Type: chroma.Punctuation},
		},
		"package": {
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.NameDecorator, Mutator: chroma.Pop(1)},
			chroma.Default(chroma.Pop(1)),
		},
		"typename": {
			{Pattern: `[a-zA-Z_]\w*`, Type: chroma.KeywordPseudo, Mutator: chroma.Pop(1)},
			chroma.Default(chroma.Pop(1)),
		},
	}
}
