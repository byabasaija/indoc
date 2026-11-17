package python

import (
	"strings"

	"github.com/byabasaija/indoc/internal/lang"
)

// Parse returns every def, async def, and class in src, including nested ones,
// in source order. Qual paths follow def/class nesting only: a function defined
// inside an if-block at module level has no prefix.
func Parse(src string) []lang.Symbol {
	p := &parser{toks: Tokenize(src)}
	p.block(nil, false)
	return p.syms
}

type parser struct {
	toks []Token
	i    int
	syms []lang.Symbol
	// members collects, for each class being parsed, the token ranges of its
	// methods' and nested classes' bodies, which the class's own body hash
	// excludes.
	members [][]span
}

type span struct{ from, to int }

type scope struct {
	name    string
	isClass bool
}

func (p *parser) peek() Token {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return Token{Kind: EOF}
}

// block parses statements until the DEDENT closing this block (consumed when
// nested) or EOF.
func (p *parser) block(scopes []scope, nested bool) {
	for {
		tok := p.peek()
		switch tok.Kind {
		case EOF:
			return
		case Dedent:
			p.i++
			if nested {
				return
			}
		case NL, Newline, Comment, Indent:
			p.i++
		default:
			p.statement(scopes)
		}
	}
}

// statement parses one logical line and, if it opens an indented block, that
// block too.
func (p *parser) statement(scopes []scope) {
	decoratorStart := p.i
	firstDecoratorLine := 0
	for p.peek().Kind == Op && p.peek().Text == "@" {
		if firstDecoratorLine == 0 {
			firstDecoratorLine = p.peek().Line
		}
		p.skipLogicalLine()
		p.skipTrivia()
	}
	tok := p.peek()
	isAsyncDef := tok.Kind == Name && tok.Text == "async" && p.i+1 < len(p.toks) &&
		p.toks[p.i+1].Kind == Name && p.toks[p.i+1].Text == "def"
	if tok.Kind == Name && (tok.Text == "def" || tok.Text == "class" || isAsyncDef) {
		p.definition(scopes, decoratorStart, firstDecoratorLine)
		return
	}
	if firstDecoratorLine != 0 && (tok.Kind == EOF || tok.Kind == Dedent) {
		return // dangling decorators in malformed input
	}
	p.skipLogicalLine()
	p.nestedBlock(scopes)
}

func (p *parser) definition(scopes []scope, decoratorStart, firstDecoratorLine int) {
	keywordLine := p.peek().Line
	if p.peek().Text == "async" {
		p.i++
	}
	isClass := p.peek().Text == "class"
	p.i++ // def / class
	name := ""
	if p.peek().Kind == Name {
		name = p.peek().Text
	}

	// Header runs to the ':' at bracket depth 0.
	depth := 0
	for p.peek().Kind != EOF {
		tok := p.peek()
		if tok.Kind == Newline && depth == 0 {
			break // malformed header without ':'
		}
		p.i++
		if tok.Kind != Op {
			continue
		}
		switch tok.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		case ":":
			if depth == 0 {
				goto header
			}
		}
	}
header:
	surface := significant(p.toks[decoratorStart:p.i])

	bodyStart := p.i
	inner := append(append([]scope{}, scopes...), scope{name: name, isClass: isClass})
	idx := len(p.syms)
	p.syms = append(p.syms, lang.Symbol{}) // reserve slot to keep source order
	if isClass {
		p.members = append(p.members, nil)
	}
	if p.restOfLineIsEmpty() {
		p.skipLogicalLine()
		p.nestedBlock(inner)
	} else {
		p.skipLogicalLine() // simple-statement body: `def f(): return 1`
	}
	body := p.toks[bodyStart:p.i]

	// A class's body is its interface: attributes and member signatures.
	// Member bodies are excluded; each member has its own symbol.
	var exclude []span
	if isClass {
		exclude = p.members[len(p.members)-1]
		p.members = p.members[:len(p.members)-1]
	}
	if len(scopes) > 0 && scopes[len(scopes)-1].isClass && len(p.members) > 0 {
		top := len(p.members) - 1
		p.members[top] = append(p.members[top], span{bodyStart, p.i})
	}

	kind := lang.KindFunction
	switch {
	case isClass:
		kind = lang.KindClass
	case len(scopes) > 0 && scopes[len(scopes)-1].isClass:
		kind = lang.KindMethod
	}
	decoratorLine := keywordLine
	if firstDecoratorLine != 0 {
		decoratorLine = firstDecoratorLine
	}
	p.syms[idx] = lang.Symbol{
		Name:          name,
		Qual:          qual(inner),
		Kind:          kind,
		Line:          keywordLine,
		DecoratorLine: decoratorLine,
		EndLine:       lastCodeLine(body, keywordLine),
		Surface:       lang.Hash(surface),
		Body:          lang.Hash(significant(p.without(bodyStart, p.i, exclude))),
		Private:       privateQual(inner),
	}
}

// restOfLineIsEmpty reports whether only comments remain before NEWLINE,
// meaning the body is an indented block.
func (p *parser) restOfLineIsEmpty() bool {
	for j := p.i; j < len(p.toks); j++ {
		switch p.toks[j].Kind {
		case Comment, NL:
			continue
		case Newline, EOF:
			return true
		default:
			return false
		}
	}
	return true
}

// nestedBlock parses an indented block if one starts at the cursor.
func (p *parser) nestedBlock(scopes []scope) {
	j := p.i
	for j < len(p.toks) && (p.toks[j].Kind == NL || p.toks[j].Kind == Comment) {
		j++
	}
	if j < len(p.toks) && p.toks[j].Kind == Indent {
		p.i = j + 1
		p.block(scopes, true)
	}
}

// skipLogicalLine advances past the next NEWLINE (or to EOF/DEDENT).
func (p *parser) skipLogicalLine() {
	for p.i < len(p.toks) {
		tok := p.toks[p.i]
		if tok.Kind == EOF || tok.Kind == Dedent || tok.Kind == Indent {
			return
		}
		p.i++
		if tok.Kind == Newline {
			return
		}
	}
}

func (p *parser) skipTrivia() {
	for p.i < len(p.toks) {
		switch p.toks[p.i].Kind {
		case NL, Comment:
			p.i++
		default:
			return
		}
	}
}

func qual(scopes []scope) string {
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = s.name
	}
	return strings.Join(parts, ".")
}

// lastCodeLine is the end line of the last code token in toks: the symbol's
// end as CPython's ast reports it (trailing comments excluded).
func lastCodeLine(toks []Token, fallback int) int {
	for j := len(toks) - 1; j >= 0; j-- {
		switch toks[j].Kind {
		case Comment, NL, Newline, Indent, Dedent, EOF:
			continue
		}
		return toks[j].EndLine
	}
	return fallback
}

// without returns toks[from:to] minus the excluded spans.
func (p *parser) without(from, to int, exclude []span) []Token {
	if len(exclude) == 0 {
		return p.toks[from:to]
	}
	out := make([]Token, 0, to-from)
	j := 0
	for i := from; i < to; i++ {
		for j < len(exclude) && exclude[j].to <= i {
			j++
		}
		if j < len(exclude) && i >= exclude[j].from && i < exclude[j].to {
			continue
		}
		out = append(out, p.toks[i])
	}
	return out
}

// significant returns token texts with comments, layout, trailing commas, and
// statements that cannot affect documented behavior removed:
//
//   - bare string statements (docstrings), which have no runtime effect
//   - logging calls such as logger.info(...) or self.log.warning(...)
//
// Block structure is kept as markers so moving a statement in or out of a
// block counts as a change.
func significant(toks []Token) []string {
	out := make([]string, 0, len(toks))
	var stmt []Token
	flush := func(end string) {
		if len(stmt) > 0 && !ignorable(stmt) {
			for _, t := range stmt {
				out = append(out, t.Text)
			}
			if end != "" {
				out = append(out, end)
			}
		}
		stmt = stmt[:0]
	}
	for _, t := range toks {
		switch t.Kind {
		case Comment, NL, EOF:
		case Newline:
			flush(";")
		case Indent:
			flush("")
			out = append(out, "{")
		case Dedent:
			flush("")
			out = append(out, "}")
		default:
			// A trailing comma before a closing bracket is formatting
			// (formatters add "magic" trailing commas). This also equates
			// (x,) with (x), an accepted blind spot.
			if t.Kind == Op && (t.Text == ")" || t.Text == "]" || t.Text == "}") &&
				len(stmt) > 0 && stmt[len(stmt)-1].Text == "," {
				stmt = stmt[:len(stmt)-1]
			}
			stmt = append(stmt, t)
		}
	}
	flush("")
	return out
}

var loggerNames = map[string]bool{
	"logger": true, "logging": true, "log": true, "LOGGER": true, "LOG": true,
	"_logger": true, "_log": true, "_LOGGER": true,
}

// ignorable reports whether a simple statement is a docstring-like bare
// string or a single logging call.
func ignorable(stmt []Token) bool {
	allStrings := true
	for _, t := range stmt {
		if t.Kind != String {
			allStrings = false
			break
		}
	}
	if allStrings {
		return true
	}
	// [self .] logger . method ( ... )  as the whole statement
	i := 0
	if len(stmt) > 2 && stmt[0].Text == "self" && stmt[1].Text == "." {
		i = 2
	}
	if len(stmt) < i+5 || stmt[i].Kind != Name || !loggerNames[stmt[i].Text] ||
		stmt[i+1].Text != "." || stmt[i+2].Kind != Name || stmt[i+3].Text != "(" ||
		stmt[len(stmt)-1].Text != ")" {
		return false
	}
	depth := 0
	for k := i + 3; k < len(stmt); k++ {
		switch stmt[k].Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
			if depth == 0 && k != len(stmt)-1 {
				return false // e.g. logger.info(x) or fallback()
			}
		}
	}
	return depth == 0
}

// FileHash hashes the whole file's significant tokens, so module-level edits
// (settings, urlpatterns) are visible even though no symbol owns them.
// Docstrings and logging calls are excluded, as in symbol hashes.
func FileHash(src string) string {
	return lang.Hash(significant(Tokenize(src)))
}

// privateQual applies Python's convention: any _name segment (dunder methods
// excepted) makes the symbol private.
func privateQual(scopes []scope) bool {
	for _, sc := range scopes {
		n := sc.name
		if strings.HasPrefix(n, "_") && !(strings.HasPrefix(n, "__") && strings.HasSuffix(n, "__")) {
			return true
		}
	}
	return false
}
