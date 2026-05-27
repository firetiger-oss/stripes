package code

import "github.com/alecthomas/chroma/v2"

// rulesLexer highlights the Go compiler's SSA rewrite-rule DSL, the
// `(op <type> [auxint] {aux} arg0 arg1 ...) [&& cond] => (...)` syntax
// used in cmd/compile/internal/ssa/_gen/*.rules. Chroma has no built-in
// lexer for the format.
//
// Token mapping (github-dark colors in parens for orientation):
//   - `//` and `/* */` comments  → Comment (grey)
//   - `=>` and `&&`              → Operator (red)
//   - `<...>` type annotation    → KeywordType inside angle-bracket state
//   - `[...]` auxint             → bracket state; inner numbers, bools,
//     and identifiers tokenise normally, brackets are Punctuation
//   - `{...}` aux symbol         → brace state; inner ident is NameTag
//   - `___`, `...`, bare `_`     → NameBuiltinPseudo
//   - `ident:` binding label     → NameLabel + Punctuation
//   - PascalCase identifier      → NameFunction (op names like I64Add,
//     SignExt32to64, MakeTuple). Matches before lowercase identifier so
//     a single capital letter is enough.
//   - lowercase ident with dots  → Name (captures, helpers like
//     bits.Len64, config.PtrSize)
//   - numbers, strings           → LiteralNumber* / LiteralString
//   - other punctuation/operators → Punctuation / Operator
//
// The `|` inside op-name alternation like `Add(64|32|16|8)` falls into
// the Operator rule; the surrounding `(` `)` are Punctuation, which reads
// fine in github-dark.
var rulesLexer = chroma.MustNewLexer(
	&chroma.Config{
		Name:      "Go SSA Rules",
		Aliases:   []string{"go-ssa-rules", "rules"},
		Filenames: []string{"*.rules"},
	},
	rulesLexerRules,
)

func rulesLexerRules() chroma.Rules {
	return chroma.Rules{
		"root": {
			{Pattern: `[ \t\r\n]+`, Type: chroma.Text},
			{Pattern: `//[^\n]*`, Type: chroma.CommentSingle},
			{Pattern: `/\*(.|\n)*?\*/`, Type: chroma.CommentMultiline},
			{Pattern: `=>`, Type: chroma.Operator},
			{Pattern: `<<|>>|<=|>=|==|!=|&&|\|\|`, Type: chroma.Operator},
			// An angle-bracket type annotation always opens with `<`
			// followed by an identifier-start character (e.g. `<t>`,
			// `<typ.Int64>`, `<*Type>`). Other `<` are comparison
			// operators and fall through to the operator rule below.
			{Pattern: `<(?=[a-zA-Z_*])`, Type: chroma.Punctuation, Mutator: chroma.Push("type")},
			{Pattern: `\[`, Type: chroma.Punctuation, Mutator: chroma.Push("auxint")},
			{Pattern: `\{`, Type: chroma.Punctuation, Mutator: chroma.Push("aux")},
			{Pattern: `___|\.\.\.|\b_\b`, Type: chroma.NameBuiltinPseudo},
			{Pattern: `([a-zA-Z_]\w*)(\s*)(:)(?!=)`, Type: chroma.ByGroups(chroma.NameLabel, chroma.Text, chroma.Punctuation)},
			{Pattern: `"(\\.|[^"\\])*"`, Type: chroma.LiteralString},
			{Pattern: `'(\\.|[^'\\])*'`, Type: chroma.LiteralString},
			{Pattern: `0x[0-9a-fA-F]+`, Type: chroma.LiteralNumberHex},
			{Pattern: `\d+\.\d*|\.\d+|\d+[eE][+-]?\d+`, Type: chroma.LiteralNumberFloat},
			{Pattern: `\d+`, Type: chroma.LiteralNumberInteger},
			{Pattern: `\b(true|false|nil)\b`, Type: chroma.KeywordConstant},
			{Pattern: `[A-Z]\w*`, Type: chroma.NameFunction},
			{Pattern: `[a-z_]\w*(?:\.[a-zA-Z_]\w*)*`, Type: chroma.Name},
			{Pattern: `[(),;.]`, Type: chroma.Punctuation},
			{Pattern: `[+\-*/%&|^=!<>]`, Type: chroma.Operator},
		},
		"type": {
			{Pattern: `>`, Type: chroma.Punctuation, Mutator: chroma.Pop(1)},
			{Pattern: `[ \t\r\n]+`, Type: chroma.Text},
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.KeywordType},
			{Pattern: `[(),*&]`, Type: chroma.Punctuation},
			{Pattern: `\d+`, Type: chroma.LiteralNumberInteger},
		},
		"auxint": {
			{Pattern: `\]`, Type: chroma.Punctuation, Mutator: chroma.Pop(1)},
			{Pattern: `[ \t\r\n]+`, Type: chroma.Text},
			{Pattern: `\b(true|false)\b`, Type: chroma.KeywordConstant},
			{Pattern: `0x[0-9a-fA-F]+`, Type: chroma.LiteralNumberHex},
			{Pattern: `\d+\.\d*|\.\d+|\d+[eE][+-]?\d+`, Type: chroma.LiteralNumberFloat},
			{Pattern: `\d+`, Type: chroma.LiteralNumberInteger},
			{Pattern: `[A-Z]\w*`, Type: chroma.NameFunction},
			{Pattern: `[a-z_]\w*(?:\.[a-zA-Z_]\w*)*`, Type: chroma.Name},
			{Pattern: `[(),]`, Type: chroma.Punctuation},
			{Pattern: `<<|>>|[+\-*/%&|^]`, Type: chroma.Operator},
		},
		"aux": {
			{Pattern: `\}`, Type: chroma.Punctuation, Mutator: chroma.Pop(1)},
			{Pattern: `[ \t\r\n]+`, Type: chroma.Text},
			{Pattern: `[a-zA-Z_][\w.]*`, Type: chroma.NameTag},
			{Pattern: `[(),]`, Type: chroma.Punctuation},
		},
	}
}
