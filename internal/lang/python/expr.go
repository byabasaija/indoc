package python

import (
	"strings"

	"github.com/byabasaija/indoc/internal/treesitter"
)

// Expr is a Python expression in the forms static configuration uses (URL
// tables, routers, settings reads). Everything else is Opaque with its source
// text.
type Expr interface{ Text() string }

// Str is a string literal (adjacent literals concatenated). Literal is false
// for f-strings and t-strings, whose value is not known statically.
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
	Line   int // 1-based line of the opening parenthesis
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

// expr converts a syntax tree node.
func expr(n *treesitter.Node, src []byte) Expr {
	if n == nil {
		return &Opaque{}
	}
	switch n.Type {
	case "identifier":
		return &Dotted{Path: n.Text(src)}
	case "true", "false", "none":
		return &Dotted{Path: map[string]string{"true": "True", "false": "False", "none": "None"}[n.Type]}
	case "attribute":
		obj := expr(n.Child("object"), src)
		attr := ""
		if a := n.Child("attribute"); a != nil {
			attr = a.Text(src)
		}
		if d, ok := obj.(*Dotted); ok {
			return &Dotted{Path: d.Path + "." + attr}
		}
		return &Attr{X: obj, Attr: attr}
	case "call":
		return call(n, src)
	case "string":
		return str(n, src)
	case "concatenated_string":
		s := &Str{Literal: true, Src: oneLine(n.Text(src))}
		var b strings.Builder
		for _, c := range named(n) {
			part := str(c, src)
			b.WriteString(part.Value)
			s.Literal = s.Literal && part.Literal
		}
		s.Value = b.String()
		return s
	case "list", "tuple", "expression_list":
		seq := &Seq{Tuple: n.Type != "list"}
		for _, c := range named(n) {
			seq.Elems = append(seq.Elems, expr(c, src))
		}
		return seq
	case "parenthesized_expression":
		if inner := named(n); len(inner) == 1 {
			return expr(inner[0], src)
		}
	case "list_splat", "dictionary_splat":
		if inner := named(n); len(inner) == 1 {
			return &Starred{X: expr(inner[0], src), Double: n.Type == "dictionary_splat"}
		}
	case "binary_operator":
		op := ""
		if o := n.Child("operator"); o != nil {
			op = o.Text(src)
		}
		return &BinOp{Op: op, L: expr(n.Child("left"), src), R: expr(n.Child("right"), src)}
	case "conditional_expression":
		if parts := named(n); len(parts) == 3 {
			return &Cond{Then: expr(parts[0], src), Test: expr(parts[1], src), Else: expr(parts[2], src)}
		}
	}
	return &Opaque{Src: oneLine(n.Text(src))}
}

func call(n *treesitter.Node, src []byte) *Call {
	c := &Call{Func: expr(n.Child("function"), src), Line: n.StartRow + 1}
	args := n.Child("arguments")
	if args == nil {
		return c
	}
	c.Line = args.StartRow + 1
	for _, a := range named(args) {
		if a.Type == "keyword_argument" {
			name := ""
			if nm := a.Child("name"); nm != nil {
				name = nm.Text(src)
			}
			c.Kwargs = append(c.Kwargs, Kwarg{Name: name, Value: expr(a.Child("value"), src)})
			continue
		}
		c.Args = append(c.Args, expr(a, src))
	}
	return c
}

// str decodes a string node. f-strings and t-strings are not literal.
func str(n *treesitter.Node, src []byte) *Str {
	s := &Str{Src: oneLine(n.Text(src)), Literal: true}
	var start, end *treesitter.Node
	for _, c := range n.Children {
		switch c.Type {
		case "string_start":
			start = c
		case "string_end":
			end = c
		case "interpolation":
			s.Literal = false
		}
	}
	if start == nil || end == nil {
		s.Literal = false
		return s
	}
	prefix := strings.ToLower(strings.TrimRight(start.Text(src), `"'`))
	if strings.ContainsAny(prefix, "ft") {
		s.Literal = false
	}
	body := string(src[start.End:end.Start])
	if strings.Contains(prefix, "r") || !strings.Contains(body, `\`) {
		s.Value = body
	} else {
		s.Value = unescape(body)
	}
	return s
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

func named(n *treesitter.Node) []*treesitter.Node {
	var out []*treesitter.Node
	for _, c := range n.Children {
		if c.Named && !c.Extra {
			out = append(out, c)
		}
	}
	return out
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }
