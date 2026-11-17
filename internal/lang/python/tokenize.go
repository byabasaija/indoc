// Package python parses Python source into symbols without running Python.
//
// The tokenizer is complete: strings of every form (including PEP 701
// f-strings with nested quotes), comments, bracket nesting, line
// continuations, and INDENT/DEDENT. The parser built on it is deliberately
// partial: it understands def, async def, class, and decorators, and treats
// every other statement as opaque tokens.
package python

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Kind is a token kind.
type Kind int

// Token kinds, mirroring CPython's tokenize module.
const (
	Name Kind = iota
	Number
	String
	Op
	Newline // end of a logical line
	NL      // non-logical line break (blank line, inside brackets, after a comment-only line)
	Comment
	Indent
	Dedent
	EOF
)

// Token is one lexical token. Line and EndLine are 1-based.
type Token struct {
	Kind    Kind
	Text    string
	Line    int
	EndLine int
}

var threeCharOps = []string{"**=", "//=", ">>=", "<<=", "..."}

var twoCharOps = []string{
	"!=", "%=", "&=", "**", "*=", "+=", "-=", "->", "//", "/=", ":=",
	"<<", "<=", "==", ">=", ">>", "@=", "^=", "|=",
}

// Tokenize splits Python source into tokens. It never fails: malformed input
// produces best-effort tokens so callers can still find symbols.
func Tokenize(src string) []Token {
	src = strings.TrimPrefix(src, "\uFEFF")
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	t := &tokenizer{src: src, line: 1, indents: []int{0}, atLineStart: true}
	t.run()
	return t.out
}

type tokenizer struct {
	src         string
	pos         int
	line        int
	depth       int // bracket nesting
	indents     []int
	atLineStart bool
	lineHasCode bool // a non-comment token was emitted since the last NEWLINE
	out         []Token
}

func (t *tokenizer) emit(kind Kind, text string, line int) {
	t.out = append(t.out, Token{Kind: kind, Text: text, Line: line, EndLine: line + strings.Count(text, "\n")})
	if kind != Comment && kind != NL && kind != Newline && kind != Indent && kind != Dedent {
		t.lineHasCode = true
	}
}

func (t *tokenizer) run() {
	n := len(t.src)
	for t.pos < n {
		if t.atLineStart && t.depth == 0 {
			t.indentation()
			if t.pos >= n {
				break
			}
		}
		c := t.src[t.pos]
		switch {
		case c == ' ' || c == '\t' || c == '\f':
			t.pos++
		case c == '#':
			end := strings.IndexByte(t.src[t.pos:], '\n')
			if end < 0 {
				end = n - t.pos
			}
			t.emit(Comment, t.src[t.pos:t.pos+end], t.line)
			t.pos += end
		case c == '\n':
			if t.depth == 0 && t.lineHasCode {
				t.emit(Newline, "\n", t.line)
				t.lineHasCode = false
			} else {
				t.emit(NL, "\n", t.line)
			}
			t.pos++
			t.line++
			t.atLineStart = t.depth == 0
		case c == '\\' && t.pos+1 < n && t.src[t.pos+1] == '\n':
			t.pos += 2
			t.line++
		case c == '"' || c == '\'':
			t.str(t.pos, t.pos)
		case c >= '0' && c <= '9', c == '.' && t.pos+1 < n && isDigit(t.src[t.pos+1]):
			t.number()
		default:
			r, size := utf8.DecodeRuneInString(t.src[t.pos:])
			if isIdentStart(r) {
				t.nameOrString()
				continue
			}
			t.op(size)
		}
	}
	if t.lineHasCode {
		t.emit(Newline, "", t.line)
	}
	for len(t.indents) > 1 {
		t.indents = t.indents[:len(t.indents)-1]
		t.emit(Dedent, "", t.line)
	}
	t.emit(EOF, "", t.line)
}

// indentation measures leading whitespace and emits INDENT/DEDENT for lines
// that contain code. Blank and comment-only lines do not affect indentation.
func (t *tokenizer) indentation() {
	col := 0
	i := t.pos
	for i < len(t.src) {
		switch t.src[i] {
		case ' ':
			col++
		case '\t':
			col = (col/8 + 1) * 8
		case '\f':
			col = 0
		default:
			goto done
		}
		i++
	}
done:
	t.pos = i
	t.atLineStart = false
	if i >= len(t.src) || t.src[i] == '\n' || t.src[i] == '#' ||
		(t.src[i] == '\\' && i+1 < len(t.src) && t.src[i+1] == '\n') {
		return
	}
	top := t.indents[len(t.indents)-1]
	if col > top {
		t.indents = append(t.indents, col)
		t.emit(Indent, "", t.line)
		return
	}
	for col < t.indents[len(t.indents)-1] {
		t.indents = t.indents[:len(t.indents)-1]
		t.emit(Dedent, "", t.line)
	}
	if col > t.indents[len(t.indents)-1] { // inconsistent dedent; recover
		t.indents = append(t.indents, col)
	}
}

func (t *tokenizer) nameOrString() {
	start := t.pos
	end := scanIdent(t.src, start)
	word := t.src[start:end]
	if end < len(t.src) && (t.src[end] == '"' || t.src[end] == '\'') && isStringPrefix(word) {
		t.str(start, end)
		return
	}
	t.pos = end
	t.emit(Name, word, t.line)
}

// str scans a string literal whose prefix starts at start and quote at quote.
func (t *tokenizer) str(start, quote int) {
	end := scanString(t.src, quote, t.src[start:quote])
	text := t.src[start:end]
	line := t.line
	t.pos = end
	t.emit(String, text, line)
	t.line += strings.Count(text, "\n")
}

func (t *tokenizer) number() {
	start := t.pos
	i := start
	hex := strings.HasPrefix(strings.ToLower(t.src[start:]), "0x")
	for i < len(t.src) {
		c := t.src[i]
		if isDigit(c) || isASCIILetter(c) || c == '_' || c == '.' {
			i++
			continue
		}
		if (c == '+' || c == '-') && !hex && i > start && (t.src[i-1] == 'e' || t.src[i-1] == 'E') {
			i++
			continue
		}
		break
	}
	t.pos = i
	t.emit(Number, t.src[start:i], t.line)
}

func (t *tokenizer) op(size int) {
	rest := t.src[t.pos:]
	for _, group := range [][]string{threeCharOps, twoCharOps} {
		for _, o := range group {
			if strings.HasPrefix(rest, o) {
				t.pos += len(o)
				t.emit(Op, o, t.line)
				return
			}
		}
	}
	c := t.src[t.pos]
	switch c {
	case '(', '[', '{':
		t.depth++
	case ')', ']', '}':
		if t.depth > 0 {
			t.depth--
		}
	}
	t.pos += size
	t.emit(Op, rest[:size], t.line)
}

// scanString returns the index just past the string literal whose opening
// quote is at q. Formatted strings (f/t prefixes) may contain replacement
// fields with arbitrary nested expressions, including strings that reuse the
// enclosing quote character (PEP 701).
func scanString(src string, q int, prefix string) int {
	n := len(src)
	prefix = strings.ToLower(prefix)
	formatted := strings.ContainsAny(prefix, "ft")
	raw := strings.Contains(prefix, "r")
	quote := src[q]
	triple := q+2 < n && src[q+1] == quote && src[q+2] == quote
	i := q + 1
	if triple {
		i = q + 3
	}
	for i < n {
		c := src[i]
		switch {
		case c == '\\':
			switch {
			case formatted && !raw && i+2 < n && src[i+1] == 'N' && src[i+2] == '{':
				// \N{NAME}: the braces belong to the escape, not a field.
				if end := strings.IndexByte(src[i:], '}'); end >= 0 {
					i += end + 1
				} else {
					i = n
				}
			case formatted && i+1 < n && (src[i+1] == '{' || src[i+1] == '}'):
				i++ // a backslash does not escape braces in f-strings
			default:
				i += 2
			}
		case c == quote && (!triple || (i+2 < n && src[i+1] == quote && src[i+2] == quote)):
			if triple {
				return i + 3
			}
			return i + 1
		case c == '\n' && !triple:
			return i // unterminated; stop at end of line
		case formatted && c == '{':
			if i+1 < n && src[i+1] == '{' {
				i += 2
				continue
			}
			i = scanField(src, i+1)
		case formatted && c == '}' && i+1 < n && src[i+1] == '}':
			i += 2
		default:
			i++
		}
	}
	return n
}

// scanField scans an f-string replacement field starting just after '{' and
// returns the index just past its closing '}'.
func scanField(src string, i int) int {
	n := len(src)
	depth := 0
	for i < n {
		c := src[i]
		switch {
		case c == '"' || c == '\'':
			i = scanString(src, i, "")
		case isASCIILetter(c) || c == '_':
			end := scanIdent(src, i)
			if end < n && (src[end] == '"' || src[end] == '\'') && isStringPrefix(src[i:end]) {
				i = scanString(src, end, src[i:end])
			} else {
				i = end
			}
		case c == '(' || c == '[' || c == '{':
			depth++
			i++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
			i++
		case c == '}':
			if depth == 0 {
				return i + 1
			}
			depth--
			i++
		case c == ':' && depth == 0:
			return scanFormatSpec(src, i+1)
		case c == '!' && depth == 0 && i+1 < n && src[i+1] != '=':
			i++ // conversion (!r, !s, !a); the spec or '}' follows
		default:
			i++
		}
	}
	return n
}

// scanFormatSpec scans literal format-spec text, which may contain nested
// replacement fields, and returns the index just past the closing '}'.
func scanFormatSpec(src string, i int) int {
	for i < len(src) {
		switch src[i] {
		case '{':
			i = scanField(src, i+1)
		case '}':
			return i + 1
		case '\\':
			i += 2
		default:
			i++
		}
	}
	return len(src)
}

func scanIdent(src string, i int) int {
	for i < len(src) {
		r, size := utf8.DecodeRuneInString(src[i:])
		if !isIdentContinue(r) {
			break
		}
		i += size
	}
	return i
}

func isStringPrefix(word string) bool {
	switch strings.ToLower(word) {
	case "r", "u", "b", "f", "t", "br", "rb", "fr", "rf", "tr", "rt":
		return true
	}
	return false
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.Is(unicode.Nl, r)
}

func isIdentContinue(r rune) bool {
	return isIdentStart(r) || unicode.IsDigit(r) ||
		unicode.In(r, unicode.Mn, unicode.Mc, unicode.Nd, unicode.Pc)
}

func isDigit(c byte) bool       { return c >= '0' && c <= '9' }
func isASCIILetter(c byte) bool { return (c|0x20) >= 'a' && (c|0x20) <= 'z' }
