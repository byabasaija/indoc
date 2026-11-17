package jsts

import (
	"strings"

	"github.com/byabasaija/indoc/internal/treesitter"
)

// Module is the statically visible structure of a JavaScript or TypeScript
// file, for framework extractors: imports (ESM and require), top-level
// bindings, exports (ESM and CommonJS), every call, and decorated classes.
type Module struct {
	Imports  []Import
	Bindings map[string]Expr   // top-level const/let/var name -> initializer
	Exports  map[string]string // exported name -> local name ("default" included)
	Default  Expr              // export default <expr> / module.exports = <expr>
	Calls    []*Call           // every call and new expression, in source order
	Classes  []*Class          // classes with decorators (NestJS)
}

// Import is one imported binding: Name is "default", "*", or the exported
// name.
type Import struct {
	Local, Source, Name string
	Line                int
}

// Class is a class with its decorators and its methods' decorators.
type Class struct {
	Name       string
	Line       int
	Decorators []*Call
	Methods    []*Method
	Exported   bool
}

// Method is a class method with its decorators.
type Method struct {
	Name       string
	Line       int
	Decorators []*Call
}

// Expr is a JavaScript expression in the forms routing code uses.
type Expr interface{ Text() string }

// Str is a string literal or a template literal without substitutions.
type Str struct{ Value, Src string }

// Name is an identifier or a member chain of identifiers: `app`,
// `express.Router`, `this.router`.
type Name struct{ Path string }

// Member is property access on something other than a Name, such as a call:
// router.route("/x").get.
type Member struct {
	X    Expr
	Prop string
}

// Call is a call or `new` expression.
type Call struct {
	Callee Expr
	Args   []Expr
	New    bool
	Line   int
}

// Array is an array literal.
type Array struct{ Elems []Expr }

// Func is an inline function or arrow function.
type Func struct{ Src string }

// Opaque is anything else.
type Opaque struct{ Src string }

func (s *Str) Text() string    { return s.Src }
func (n *Name) Text() string   { return n.Path }
func (m *Member) Text() string { return m.X.Text() + "." + m.Prop }
func (f *Func) Text() string   { return f.Src }
func (o *Opaque) Text() string { return o.Src }
func (a *Array) Text() string {
	parts := make([]string, len(a.Elems))
	for i, e := range a.Elems {
		parts[i] = e.Text()
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
func (c *Call) Text() string {
	parts := make([]string, len(c.Args))
	for i, a := range c.Args {
		parts[i] = a.Text()
	}
	prefix := ""
	if c.New {
		prefix = "new "
	}
	return prefix + c.Callee.Text() + "(" + strings.Join(parts, ", ") + ")"
}

// CalleeName returns the callee's dotted name, or "".
func (c *Call) CalleeName() string {
	if n, ok := c.Callee.(*Name); ok {
		return n.Path
	}
	return ""
}

// Arg returns argument i or nil.
func (c *Call) Arg(i int) Expr {
	if i < len(c.Args) {
		return c.Args[i]
	}
	return nil
}

// ParseModule reads the structure of a JS/TS file. It never fails:
// unparseable input yields an empty module.
func ParseModule(file string, src []byte) *Module {
	m := &Module{Bindings: map[string]Expr{}, Exports: map[string]string{}}
	root, err := treesitter.Parse(Grammar(file), src)
	if err != nil {
		return m
	}
	b := &moduleBuilder{m: m, src: src}
	for _, st := range root.Children {
		b.statement(st)
	}
	root.Walk(func(n *treesitter.Node) bool {
		if n.Type == "call_expression" || n.Type == "new_expression" {
			if c, ok := b.expr(n).(*Call); ok {
				m.Calls = append(m.Calls, c)
			}
		}
		return true
	})
	return m
}

type moduleBuilder struct {
	m   *Module
	src []byte
}

func (b *moduleBuilder) text(n *treesitter.Node) string { return n.Text(b.src) }

func (b *moduleBuilder) statement(n *treesitter.Node) {
	switch n.Type {
	case "import_statement":
		b.importStatement(n)
	case "lexical_declaration", "variable_declaration":
		b.declaration(n)
	case "expression_statement":
		for _, c := range n.Children {
			if c.Type == "assignment_expression" {
				b.assignment(c)
			}
		}
	case "export_statement":
		b.export(n)
	case "class_declaration", "abstract_class_declaration":
		b.class(n, nil, false)
	}
}

func (b *moduleBuilder) importStatement(n *treesitter.Node) {
	source := stringValue(n.Child("source"), b.src)
	line := n.StartRow + 1
	for _, c := range n.Children {
		if c.Type != "import_clause" {
			continue
		}
		for _, part := range c.Children {
			switch part.Type {
			case "identifier":
				b.m.Imports = append(b.m.Imports, Import{Local: b.text(part), Source: source, Name: "default", Line: line})
			case "namespace_import":
				for _, id := range part.Children {
					if id.Type == "identifier" {
						b.m.Imports = append(b.m.Imports, Import{Local: b.text(id), Source: source, Name: "*", Line: line})
					}
				}
			case "named_imports":
				for _, spec := range part.Children {
					if spec.Type != "import_specifier" {
						continue
					}
					name := b.text(spec.Child("name"))
					local := name
					if alias := spec.Child("alias"); alias != nil {
						local = b.text(alias)
					}
					b.m.Imports = append(b.m.Imports, Import{Local: local, Source: source, Name: name, Line: line})
				}
			}
		}
	}
}

func (b *moduleBuilder) declaration(n *treesitter.Node) {
	for _, d := range n.Children {
		if d.Type != "variable_declarator" {
			continue
		}
		name, value := d.Child("name"), d.Child("value")
		if name == nil || value == nil {
			continue
		}
		v := b.expr(value)
		// require("x"): an import.
		if c, ok := v.(*Call); ok && c.CalleeName() == "require" {
			if s, ok := c.Arg(0).(*Str); ok {
				b.requireBinding(name, s.Value, d.StartRow+1)
				continue
			}
		}
		if name.Type == "identifier" {
			b.m.Bindings[b.text(name)] = v
		}
	}
}

func (b *moduleBuilder) requireBinding(pattern *treesitter.Node, source string, line int) {
	switch pattern.Type {
	case "identifier":
		b.m.Imports = append(b.m.Imports, Import{Local: b.text(pattern), Source: source, Name: "default", Line: line})
	case "object_pattern":
		for _, p := range pattern.Children {
			switch p.Type {
			case "shorthand_property_identifier_pattern":
				name := b.text(p)
				b.m.Imports = append(b.m.Imports, Import{Local: name, Source: source, Name: name, Line: line})
			case "pair_pattern":
				key, value := p.Child("key"), p.Child("value")
				if key != nil && value != nil && value.Type == "identifier" {
					b.m.Imports = append(b.m.Imports, Import{Local: b.text(value), Source: source, Name: b.text(key), Line: line})
				}
			}
		}
	}
}

// assignment records CommonJS exports.
func (b *moduleBuilder) assignment(n *treesitter.Node) {
	left := b.text(n.Child("left"))
	right := n.Child("right")
	if right == nil {
		return
	}
	switch {
	case left == "module.exports":
		v := b.expr(right)
		b.m.Default = v
		if name, ok := v.(*Name); ok {
			b.m.Exports["default"] = name.Path
		}
		if right.Type == "object" {
			for _, p := range right.Children {
				switch p.Type {
				case "shorthand_property_identifier":
					b.m.Exports[b.text(p)] = b.text(p)
				case "pair":
					if k, v := p.Child("key"), p.Child("value"); k != nil && v != nil && v.Type == "identifier" {
						b.m.Exports[strings.Trim(b.text(k), `"'`)] = b.text(v)
					}
				}
			}
		}
	case strings.HasPrefix(left, "exports.") || strings.HasPrefix(left, "module.exports."):
		name := left[strings.LastIndex(left, ".")+1:]
		if right.Type == "identifier" {
			b.m.Exports[name] = b.text(right)
		} else {
			b.m.Bindings["exports."+name] = b.expr(right)
			b.m.Exports[name] = "exports." + name
		}
	}
}

func (b *moduleBuilder) export(n *treesitter.Node) {
	var decorators []*Call
	for _, c := range n.Children {
		switch {
		case c.Type == "decorator":
			decorators = append(decorators, b.decorator(c))
		case c.Field == "value":
			v := b.expr(c)
			b.m.Default = v
			if name, ok := v.(*Name); ok {
				b.m.Exports["default"] = name.Path
			}
		case c.Field == "declaration":
			def := isDefault(n)
			switch c.Type {
			case "lexical_declaration", "variable_declaration":
				b.declaration(c)
				for _, d := range c.Children {
					if name := d.Child("name"); d.Type == "variable_declarator" && name != nil && name.Type == "identifier" {
						b.m.Exports[b.text(name)] = b.text(name)
					}
				}
			case "class_declaration", "abstract_class_declaration":
				b.class(c, decorators, true)
				if name := c.Child("name"); name != nil {
					b.m.Exports[b.text(name)] = b.text(name)
					if def {
						b.m.Exports["default"] = b.text(name)
					}
				}
			case "function_declaration":
				if name := c.Child("name"); name != nil {
					b.m.Exports[b.text(name)] = b.text(name)
					if def {
						b.m.Exports["default"] = b.text(name)
					}
				}
			}
		case c.Type == "export_clause":
			for _, spec := range c.Children {
				if spec.Type != "export_specifier" {
					continue
				}
				local := b.text(spec.Child("name"))
				exported := local
				if alias := spec.Child("alias"); alias != nil {
					exported = b.text(alias)
				}
				b.m.Exports[exported] = local
			}
		}
	}
}

func isDefault(n *treesitter.Node) bool {
	for _, c := range n.Children {
		if !c.Named && c.Type == "default" {
			return true
		}
	}
	return false
}

func (b *moduleBuilder) decorator(n *treesitter.Node) *Call {
	for _, c := range n.Children {
		switch c.Type {
		case "call_expression":
			if call, ok := b.expr(c).(*Call); ok {
				return call
			}
		case "identifier", "member_expression":
			return &Call{Callee: b.expr(c), Line: c.StartRow + 1}
		}
	}
	return &Call{Callee: &Opaque{}, Line: n.StartRow + 1}
}

func (b *moduleBuilder) class(n *treesitter.Node, decorators []*Call, exported bool) {
	name := n.Child("name")
	if name == nil {
		return
	}
	cls := &Class{Name: b.text(name), Line: n.StartRow + 1, Decorators: decorators, Exported: exported}
	for _, c := range n.Children {
		if c.Type == "decorator" {
			cls.Decorators = append(cls.Decorators, b.decorator(c))
		}
	}
	body := n.Child("body")
	if body == nil {
		return
	}
	var pending []*Call
	for _, member := range body.Children {
		switch member.Type {
		case "decorator":
			pending = append(pending, b.decorator(member))
		case "method_definition":
			mname := member.Child("name")
			if mname == nil {
				pending = nil
				continue
			}
			for _, c := range member.Children {
				if c.Type == "decorator" {
					pending = append(pending, b.decorator(c))
				}
			}
			cls.Methods = append(cls.Methods, &Method{Name: b.text(mname), Line: member.StartRow + 1, Decorators: pending})
			pending = nil
		default:
			if member.Named {
				pending = nil
			}
		}
	}
	b.m.Classes = append(b.m.Classes, cls)
}

// expr converts an expression node.
func (b *moduleBuilder) expr(n *treesitter.Node) Expr {
	if n == nil {
		return &Opaque{}
	}
	switch n.Type {
	case "identifier", "this", "property_identifier":
		return &Name{Path: b.text(n)}
	case "member_expression":
		obj := b.expr(n.Child("object"))
		prop := ""
		if p := n.Child("property"); p != nil {
			prop = b.text(p)
		}
		if name, ok := obj.(*Name); ok {
			return &Name{Path: name.Path + "." + prop}
		}
		return &Member{X: obj, Prop: prop}
	case "call_expression", "new_expression":
		fn := n.Child("function")
		if n.Type == "new_expression" {
			fn = n.Child("constructor")
		}
		c := &Call{Callee: b.expr(fn), New: n.Type == "new_expression", Line: n.StartRow + 1}
		if args := n.Child("arguments"); args != nil {
			c.Line = args.StartRow + 1
			for _, a := range args.Children {
				if a.Named && !a.Extra {
					c.Args = append(c.Args, b.expr(a))
				}
			}
		}
		return c
	case "string":
		return &Str{Value: stringValue(n, b.src), Src: b.text(n)}
	case "template_string":
		for _, c := range n.Children {
			if c.Type == "template_substitution" {
				return &Opaque{Src: b.text(n)}
			}
		}
		return &Str{Value: stringValue(n, b.src), Src: b.text(n)}
	case "array":
		a := &Array{}
		for _, c := range n.Children {
			if c.Named && !c.Extra {
				a.Elems = append(a.Elems, b.expr(c))
			}
		}
		return a
	case "arrow_function", "function_expression", "function":
		return &Func{Src: "(inline)"}
	case "parenthesized_expression", "as_expression", "satisfies_expression", "non_null_expression":
		for _, c := range n.Children {
			if c.Named && !c.Extra {
				return b.expr(c)
			}
		}
	case "await_expression":
		for _, c := range n.Children {
			if c.Named {
				return b.expr(c)
			}
		}
	}
	return &Opaque{Src: strings.Join(strings.Fields(b.text(n)), " ")}
}

// stringValue reads a string or template literal's characters.
func stringValue(n *treesitter.Node, src []byte) string {
	if n == nil {
		return ""
	}
	var sb strings.Builder
	for _, c := range n.Children {
		switch c.Type {
		case "string_fragment":
			sb.WriteString(c.Text(src))
		case "escape_sequence":
			e := c.Text(src)
			if len(e) == 2 {
				switch e[1] {
				case 'n':
					sb.WriteByte('\n')
				case 't':
					sb.WriteByte('\t')
				default:
					sb.WriteByte(e[1])
				}
			} else {
				sb.WriteString(e)
			}
		}
	}
	return sb.String()
}
