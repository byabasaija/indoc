package python

import "strings"

// Expr is a parsed Python expression. Only the forms needed for static
// configuration (URL tables, routers, settings reads) are modeled; everything
// else is Opaque with its source text.
type Expr interface{ Text() string }

// Str is a string literal (adjacent literals concatenated). Literal is false
// for f-strings, whose value is not known statically.
type Str struct {
	Value   string
	Literal bool
	Src     string
}

// Dotted is a dotted name: `views`, `views.Foo`, `admin.site.urls`.
type Dotted struct{ Path string }

// Attr is attribute access on something other than a dotted name.
type Attr struct {
	X    Expr
	Attr string
}

// Call is a call expression.
type Call struct {
	Line   int // line of the opening parenthesis
	Func   Expr
	Args   []Expr
	Kwargs []Kwarg
}

// Kwarg is a keyword argument.
type Kwarg struct {
	Name  string
	Value Expr
}

// Seq is a list or tuple display.
type Seq struct {
	Elems []Expr
	Tuple bool
}

// Starred is `*x` or `**x`.
type Starred struct {
	X      Expr
	Double bool
}

// BinOp is a binary operation.
type BinOp struct {
	Op   string
	L, R Expr
}

// Cond is `a if test else b`.
type Cond struct{ Then, Test, Else Expr }

// Opaque is any expression not modeled above.
type Opaque struct{ Src string }

func (s *Str) Text() string    { return s.Src }
func (n *Dotted) Text() string { return n.Path }
func (a *Attr) Text() string   { return a.X.Text() + "." + a.Attr }
func (o *Opaque) Text() string { return o.Src }
func (s *Starred) Text() string {
	return map[bool]string{false: "*", true: "**"}[s.Double] + s.X.Text()
}
func (b *BinOp) Text() string { return b.L.Text() + " " + b.Op + " " + b.R.Text() }
func (c *Cond) Text() string {
	return c.Then.Text() + " if " + c.Test.Text() + " else " + c.Else.Text()
}
func (s *Seq) Text() string {
	parts := make([]string, len(s.Elems))
	for i, e := range s.Elems {
		parts[i] = e.Text()
	}
	if s.Tuple {
		return "(" + strings.Join(parts, ", ") + ")"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (c *Call) Text() string {
	var parts []string
	for _, a := range c.Args {
		parts = append(parts, a.Text())
	}
	for _, k := range c.Kwargs {
		parts = append(parts, k.Name+"="+k.Value.Text())
	}
	return c.Func.Text() + "(" + strings.Join(parts, ", ") + ")"
}

// Kw returns a keyword argument's value, or nil.
func (c *Call) Kw(name string) Expr {
	for _, k := range c.Kwargs {
		if k.Name == name {
			return k.Value
		}
	}
	return nil
}

// Arg returns positional argument i, or the keyword argument name, or nil.
func (c *Call) Arg(i int, name string) Expr {
	if i < len(c.Args) {
		if _, star := c.Args[i].(*Starred); !star {
			return c.Args[i]
		}
	}
	if name != "" {
		return c.Kw(name)
	}
	return nil
}

// FuncName is the callee's dotted name ("path", "router.register"), or "".
func (c *Call) FuncName() string {
	if n, ok := c.Func.(*Dotted); ok {
		return n.Path
	}
	return ""
}

// ParseExpr parses tokens (no NEWLINE) as one expression. It never fails;
// unrecognized input becomes Opaque.
func ParseExpr(toks []Token) Expr {
	p := &exprParser{toks: significantOnly(toks)}
	e := p.expr()
	if p.i < len(p.toks) {
		return &Opaque{Src: joinText(p.toks)}
	}
	return e
}

func significantOnly(toks []Token) []Token {
	out := make([]Token, 0, len(toks))
	for _, t := range toks {
		switch t.Kind {
		case Comment, NL, Newline, Indent, Dedent, EOF:
			continue
		}
		out = append(out, t)
	}
	return out
}

type exprParser struct {
	toks []Token
	i    int
}

func (p *exprParser) peek() Token {
	if p.i < len(p.toks) {
		return p.toks[p.i]
	}
	return Token{Kind: EOF}
}

func (p *exprParser) is(text string) bool {
	t := p.peek()
	return (t.Kind == Op || t.Kind == Name) && t.Text == text
}

func (p *exprParser) expr() Expr {
	start := p.i
	x := p.binary()
	if p.is("if") {
		p.i++
		test := p.binary()
		if !p.is("else") {
			return p.opaqueFrom(start)
		}
		p.i++
		return &Cond{Then: x, Test: test, Else: p.expr()}
	}
	if p.is("for") || p.is("lambda") || p.is(":=") {
		return p.opaqueFrom(start)
	}
	return x
}

var binaryOps = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "//": true, "%": true, "|": true, "&": true, "^": true,
	"or": true, "and": true, "==": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
	"in": true, "is": true, "@": true, "**": true, "<<": true, ">>": true,
}

func (p *exprParser) binary() Expr {
	x := p.unary()
	for {
		t := p.peek()
		if !(t.Kind == Op || t.Kind == Name) || !binaryOps[t.Text] {
			return x
		}
		if t.Text == "is" || t.Text == "in" || t.Text == "not" {
			p.i++
			if p.is("not") {
				p.i++
			}
		} else {
			p.i++
		}
		x = &BinOp{Op: t.Text, L: x, R: p.unary()}
	}
}

func (p *exprParser) unary() Expr {
	switch {
	case p.is("*"), p.is("**"):
		double := p.peek().Text == "**"
		p.i++
		return &Starred{X: p.unary(), Double: double}
	case p.is("not"), p.is("-"), p.is("+"), p.is("~"), p.is("await"):
		start := p.i
		p.i++
		p.unary()
		return p.opaqueFrom(start)
	}
	return p.postfix(p.atom())
}

func (p *exprParser) postfix(x Expr) Expr {
	for {
		switch {
		case p.is("."):
			p.i++
			t := p.peek()
			if t.Kind != Name {
				return x
			}
			p.i++
			if n, ok := x.(*Dotted); ok {
				x = &Dotted{Path: n.Path + "." + t.Text}
			} else {
				x = &Attr{X: x, Attr: t.Text}
			}
		case p.is("("):
			line := p.peek().Line
			p.i++
			x = p.callArgs(x)
			x.(*Call).Line = line
		case p.is("["):
			start := p.i
			p.skipBalanced()
			x = &Opaque{Src: x.Text() + joinText(p.toks[start:p.i])}
		default:
			return x
		}
	}
}

func (p *exprParser) callArgs(fn Expr) Expr {
	c := &Call{Func: fn}
	for !p.is(")") && p.peek().Kind != EOF {
		t := p.peek()
		if t.Kind == Name && p.i+1 < len(p.toks) && p.toks[p.i+1].Kind == Op && p.toks[p.i+1].Text == "=" {
			p.i += 2
			c.Kwargs = append(c.Kwargs, Kwarg{Name: t.Text, Value: p.expr()})
		} else {
			c.Args = append(c.Args, p.expr())
		}
		if !p.is(",") {
			break
		}
		p.i++
	}
	if !p.is(")") {
		p.skipToClose(")")
	}
	p.i++
	return c
}

func (p *exprParser) atom() Expr {
	t := p.peek()
	switch {
	case t.Kind == String:
		var raw, value []string
		literal := true
		for p.peek().Kind == String {
			s := p.peek().Text
			v, ok := decodeString(s)
			literal = literal && ok
			raw = append(raw, s)
			value = append(value, v)
			p.i++
		}
		return &Str{Value: strings.Join(value, ""), Literal: literal, Src: strings.Join(raw, " ")}
	case t.Kind == Name && !isKeyword(t.Text):
		p.i++
		return &Dotted{Path: t.Text}
	case t.Kind == Name && (t.Text == "True" || t.Text == "False" || t.Text == "None"):
		p.i++
		return &Dotted{Path: t.Text}
	case t.Kind == Number:
		p.i++
		return &Opaque{Src: t.Text}
	case p.is("("), p.is("["):
		closer := map[string]string{"(": ")", "[": "]"}[t.Text]
		start := p.i
		p.i++
		seq := &Seq{Tuple: t.Text == "("}
		trailingComma := false
		for !p.is(closer) && p.peek().Kind != EOF {
			e := p.expr()
			if p.is("for") || p.is("async") { // comprehension
				p.i = start
				p.skipBalanced()
				return &Opaque{Src: joinText(p.toks[start:p.i])}
			}
			seq.Elems = append(seq.Elems, e)
			trailingComma = false
			if !p.is(",") {
				break
			}
			p.i++
			trailingComma = true
		}
		if !p.is(closer) {
			p.i = start
			p.skipBalanced()
			return &Opaque{Src: joinText(p.toks[start:p.i])}
		}
		p.i++
		if seq.Tuple && len(seq.Elems) == 1 && !trailingComma {
			return seq.Elems[0] // parenthesized expression
		}
		return seq
	case p.is("{"):
		start := p.i
		p.skipBalanced()
		return &Opaque{Src: joinText(p.toks[start:p.i])}
	}
	// Unknown: consume one token so callers always make progress.
	p.i++
	return &Opaque{Src: t.Text}
}

// skipBalanced consumes a bracketed group starting at the cursor.
func (p *exprParser) skipBalanced() {
	depth := 0
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		if t.Kind != Op {
			continue
		}
		switch t.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
			if depth <= 0 {
				return
			}
		}
	}
}

func (p *exprParser) skipToClose(closer string) {
	depth := 0
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		if t.Kind == Op {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth == 0 && t.Text == closer {
					return
				}
				depth--
			}
		}
		p.i++
	}
}

func (p *exprParser) opaqueFrom(start int) Expr {
	depth := 0
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		if t.Kind == Op {
			switch t.Text {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth == 0 {
					return &Opaque{Src: joinText(p.toks[start:p.i])}
				}
				depth--
			case ",":
				if depth == 0 {
					return &Opaque{Src: joinText(p.toks[start:p.i])}
				}
			}
		}
		p.i++
	}
	return &Opaque{Src: joinText(p.toks[start:p.i])}
}

var keywords = map[string]bool{
	"and": true, "as": true, "assert": true, "async": true, "await": true, "break": true,
	"class": true, "continue": true, "def": true, "del": true, "elif": true, "else": true,
	"except": true, "finally": true, "for": true, "from": true, "global": true, "if": true,
	"import": true, "in": true, "is": true, "lambda": true, "nonlocal": true, "not": true,
	"or": true, "pass": true, "raise": true, "return": true, "try": true, "while": true,
	"with": true, "yield": true, "True": true, "False": true, "None": true,
}

func isKeyword(s string) bool { return keywords[s] }

func joinText(toks []Token) string {
	var b strings.Builder
	for i, t := range toks {
		if i > 0 && needsSpace(toks[i-1], t) {
			b.WriteByte(' ')
		}
		b.WriteString(t.Text)
	}
	return b.String()
}

func needsSpace(prev, cur Token) bool {
	if prev.Kind == Op || cur.Kind == Op {
		return prev.Text == ","
	}
	return true
}

// decodeString returns a string token's value. ok is false for f-strings and
// t-strings, whose value depends on runtime data.
func decodeString(tok string) (string, bool) {
	i := 0
	for i < len(tok) && tok[i] != '"' && tok[i] != '\'' {
		i++
	}
	prefix := strings.ToLower(tok[:i])
	body := tok[i:]
	q := 1
	if len(body) >= 6 && (strings.HasPrefix(body, `"""`) || strings.HasPrefix(body, `'''`)) {
		q = 3
	}
	if len(body) < 2*q {
		return "", false
	}
	body = body[q : len(body)-q]
	if strings.ContainsAny(prefix, "ft") {
		return body, false
	}
	if strings.Contains(prefix, "r") || !strings.Contains(body, `\`) {
		return body, true
	}
	return unescape(body), true
}

// unescape handles the common escapes; anything else is kept verbatim.
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' || i+1 == len(s) {
			b.WriteByte(s[i])
			continue
		}
		i++
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 't':
			b.WriteByte('\t')
		case '\\', '\'', '"':
			b.WriteByte(s[i])
		case '\n': // line continuation inside the literal
		default:
			b.WriteByte('\\')
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
