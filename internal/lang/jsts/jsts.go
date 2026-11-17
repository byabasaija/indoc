// Package jsts extracts symbols from JavaScript, TypeScript, JSX, and TSX
// using tree-sitter (see internal/treesitter).
//
// The symbol set matches the TypeScript compiler's view, checked by a
// differential test (testdata/ts_symbols.js):
//
//	function declarations with a body                        function
//	classes; class methods, constructors, accessors          class, method
//	(methods in object literals are not symbols)
//	class fields initialized with an arrow or function       method
//	variables initialized with an arrow, function, or class  function, class
//	interfaces, type aliases, enums, namespaces              type
//
// A declaration spans its whole statement, including export, const, and
// decorators. Exported declarations are public; everything else, and members
// marked private or #private, is private.
package jsts

import (
	"path"
	"strings"

	"github.com/byabasaija/indoc/internal/lang"
	"github.com/byabasaija/indoc/internal/treesitter"
)

// Grammar returns the tree-sitter grammar for a file name, or "".
func Grammar(file string) string {
	name := strings.ToLower(path.Base(file))
	switch {
	case strings.HasSuffix(name, ".tsx"):
		return "tsx"
	case strings.HasSuffix(name, ".ts"), strings.HasSuffix(name, ".mts"), strings.HasSuffix(name, ".cts"):
		return "typescript"
	case strings.HasSuffix(name, ".js"), strings.HasSuffix(name, ".jsx"), strings.HasSuffix(name, ".mjs"), strings.HasSuffix(name, ".cjs"):
		return "javascript"
	}
	return ""
}

// File is a parsed file: its symbols and a whole-file hash.
type File struct {
	Symbols []lang.Symbol
	Hash    string
}

// Parse extracts symbols from src.
func Parse(file string, src []byte) (File, error) {
	root, err := treesitter.Parse(Grammar(file), src)
	if err != nil {
		return File{}, err
	}
	p := &extractor{src: src, exported: exportedNames(root, src)}
	p.visit(root, nil)
	return File{Symbols: p.syms, Hash: lang.Hash(tokens(root, src, nil))}, nil
}

type scope struct {
	name    string
	isClass bool
}

type extractor struct {
	src      []byte
	syms     []lang.Symbol
	exported map[string]bool // names in export { ... } or module.exports
}

func (p *extractor) text(n *treesitter.Node) string { return n.Text(p.src) }

// visit walks n's children for declarations.
func (p *extractor) visit(n *treesitter.Node, scopes []scope) {
	for _, c := range n.Children {
		if !c.Extra {
			p.node(c, scopes)
		}
	}
}

func (p *extractor) node(n *treesitter.Node, scopes []scope) {
	if n.Type == "export_statement" {
		for _, c := range n.Children {
			if c.Field == "declaration" || c.Field == "value" {
				if p.declaration(c, n, scopes, true) {
					return
				}
			}
		}
		p.visit(n, scopes)
		return
	}
	if !p.declaration(n, n, scopes, false) {
		p.visit(n, scopes)
	}
}

// declaration records n if it is a symbol; span is the node whose rows the
// symbol covers (an enclosing export statement). It reports whether n was
// handled, including its children.
func (p *extractor) declaration(n, span *treesitter.Node, scopes []scope, exported bool) bool {
	switch n.Type {
	case "function_declaration", "generator_function_declaration":
		return p.symbol(n, span, n.Child("name"), n.Child("body"), lang.KindFunction, scopes, exported)
	case "class_declaration", "abstract_class_declaration", "class":
		if n.Type == "class" && n.Child("name") == nil {
			return false
		}
		return p.symbol(n, span, n.Child("name"), n.Child("body"), lang.KindClass, scopes, exported)
	case "interface_declaration", "type_alias_declaration", "enum_declaration":
		return p.symbol(n, span, n.Child("name"), n.Child("body"), lang.KindType, scopes, exported)
	case "internal_module", "module":
		name := n.Child("name")
		if name == nil || name.Type == "string" {
			return false
		}
		return p.symbol(n, span, name, n.Child("body"), lang.KindType, scopes, exported)
	case "method_definition":
		if n.Child("body") == nil || n.Parent == nil || n.Parent.Type != "class_body" {
			return false
		}
		return p.symbol(n, memberSpan(n), n.Child("name"), n.Child("body"), lang.KindMethod, scopes, exported)
	case "public_field_definition", "field_definition":
		value := n.Child("value")
		if value == nil || !isFunction(value) {
			return false
		}
		name := n.Child("name")
		if name == nil {
			name = n.Child("property")
		}
		return p.symbol(n, memberSpan(n), name, value, lang.KindMethod, scopes, exported)
	case "lexical_declaration", "variable_declaration":
		var decls []*treesitter.Node
		for _, c := range n.Children {
			if c.Type == "variable_declarator" {
				decls = append(decls, c)
			}
		}
		handled := false
		for _, d := range decls {
			name, value := d.Child("name"), d.Child("value")
			if value == nil {
				continue
			}
			kind := ""
			switch {
			case name == nil || name.Type != "identifier":
			case isFunction(value):
				kind = lang.KindFunction
			case value.Type == "class":
				kind = lang.KindClass
			}
			if kind == "" {
				p.node(value, scopes) // nested declarations inside other initializers
				handled = true
				continue
			}
			s := d
			if len(decls) == 1 {
				s = span
			}
			p.symbol(value, s, name, value, kind, scopes, exported)
			handled = true
		}
		if !handled {
			p.visit(n, scopes)
		}
		return true
	case "ambient_declaration":
		p.visit(n, scopes)
		return true
	}
	return false
}

func isFunction(n *treesitter.Node) bool {
	switch n.Type {
	case "arrow_function", "function_expression", "function", "generator_function":
		return true
	}
	return false
}

// memberSpan extends a class member to its decorators, which tree-sitter
// places as preceding siblings.
func memberSpan(n *treesitter.Node) *treesitter.Node {
	if n.Parent == nil {
		return n
	}
	first := n
	for i, c := range n.Parent.Children {
		if c != n {
			continue
		}
		for j := i - 1; j >= 0 && n.Parent.Children[j].Type == "decorator"; j-- {
			first = n.Parent.Children[j]
		}
	}
	if first == n {
		return n
	}
	return &treesitter.Node{Start: first.Start, StartRow: first.StartRow, End: n.End, EndRow: n.EndRow}
}

// symbol records a declaration and visits its body for nested declarations.
func (p *extractor) symbol(n, span, name, body *treesitter.Node, kind string, scopes []scope, exported bool) bool {
	if name == nil || name.Type == "computed_property_name" {
		return false // [Symbol.iterator]() has no citable name
	}
	ident := p.text(name)
	if name.Type == "string" {
		ident = strings.Trim(ident, `"'`)
	}
	if kind == lang.KindMethod && name.Type == "property_identifier" && ident == "constructor" {
		ident = "constructor"
	}
	inClass := len(scopes) > 0 && scopes[len(scopes)-1].isClass
	if inClass && kind == lang.KindFunction {
		kind = lang.KindMethod
	}
	quals := make([]string, 0, len(scopes)+1)
	for _, s := range scopes {
		quals = append(quals, s.name)
	}
	quals = append(quals, ident)

	private := !(exported || len(scopes) == 0 && p.exported[ident])
	if kind == lang.KindMethod {
		private = p.memberPrivate(n, name) || len(p.syms) > 0 && p.parentPrivate(scopes)
	} else if len(scopes) > 0 {
		private = true // nested functions and types are implementation detail
	}

	sym := lang.Symbol{
		Name: ident, Qual: strings.Join(quals, "."), Kind: kind,
		Line: span.StartRow + 1, DecoratorLine: span.StartRow + 1, EndLine: span.EndRow + 1,
		Private: private,
	}
	idx := len(p.syms)
	p.syms = append(p.syms, sym)

	inner := append(append([]scope{}, scopes...), scope{name: ident, isClass: kind == lang.KindClass})
	var exclude []*treesitter.Node
	if body != nil {
		if kind == lang.KindClass {
			exclude = memberBodies(body)
		}
		p.visit(body, inner)
	}
	var surfaceExclude []*treesitter.Node
	if body != nil {
		surfaceExclude = []*treesitter.Node{body}
	}
	p.syms[idx].Surface = lang.Hash(tokens(n, p.src, surfaceExclude))
	if body != nil {
		p.syms[idx].Body = lang.Hash(tokens(body, p.src, exclude))
	} else {
		p.syms[idx].Body = lang.Hash(nil)
	}
	return true
}

// memberPrivate reports private, protected, or #private class members.
func (p *extractor) memberPrivate(n, name *treesitter.Node) bool {
	if name.Type == "private_property_identifier" {
		return true
	}
	for _, c := range n.Children {
		if c.Type == "accessibility_modifier" {
			t := p.text(c)
			return t == "private" || t == "protected"
		}
	}
	return false
}

// parentPrivate reports whether the enclosing class is private.
func (p *extractor) parentPrivate(scopes []scope) bool {
	if len(scopes) == 0 {
		return false
	}
	q := make([]string, len(scopes))
	for i, s := range scopes {
		q[i] = s.name
	}
	qual := strings.Join(q, ".")
	for i := len(p.syms) - 1; i >= 0; i-- {
		if p.syms[i].Qual == qual {
			return p.syms[i].Private
		}
	}
	return false
}

// memberBodies returns the bodies of a class's members; a class's own body
// hash covers its interface, not its methods' implementations.
func memberBodies(classBody *treesitter.Node) []*treesitter.Node {
	var out []*treesitter.Node
	for _, m := range classBody.Children {
		if b := m.Child("body"); b != nil {
			out = append(out, b)
		} else if v := m.Child("value"); v != nil && isFunction(v) {
			if b := v.Child("body"); b != nil {
				out = append(out, b)
			}
		}
	}
	return out
}

// exportedNames collects names exported by `export { a, b as c }` and
// CommonJS `module.exports = { a, b }` / `exports.a = ...`.
func exportedNames(root *treesitter.Node, src []byte) map[string]bool {
	names := map[string]bool{}
	root.Walk(func(n *treesitter.Node) bool {
		switch n.Type {
		case "export_specifier":
			if name := n.Child("name"); name != nil {
				names[name.Text(src)] = true
			}
			return false
		case "assignment_expression":
			left, right := n.Child("left"), n.Child("right")
			if left == nil || right == nil {
				return true
			}
			l := left.Text(src)
			switch {
			case l == "module.exports" && right.Type == "object":
				for _, prop := range right.Children {
					switch prop.Type {
					case "shorthand_property_identifier":
						names[prop.Text(src)] = true
					case "pair":
						if v := prop.Child("value"); v != nil && v.Type == "identifier" {
							names[v.Text(src)] = true
						}
					}
				}
			case l == "module.exports" && right.Type == "identifier":
				names[right.Text(src)] = true
			case strings.HasPrefix(l, "exports.") || strings.HasPrefix(l, "module.exports."):
				if right.Type == "identifier" {
					names[right.Text(src)] = true
				}
			}
		}
		return true
	})
	return names
}

// tokens returns the leaf texts under n with comments, logging statements,
// and trailing commas removed, skipping the subtrees in exclude.
func tokens(n *treesitter.Node, src []byte, exclude []*treesitter.Node) []string {
	skip := map[*treesitter.Node]bool{}
	for _, e := range exclude {
		skip[e] = true
	}
	var out []string
	var walk func(*treesitter.Node)
	walk = func(n *treesitter.Node) {
		if skip[n] || n.Extra || n.Type == "comment" || isLogging(n, src) {
			return
		}
		if len(n.Children) == 0 {
			t := n.Text(src)
			if t == "" {
				return
			}
			if (t == ")" || t == "]" || t == "}") && len(out) > 0 && out[len(out)-1] == "," {
				out = out[:len(out)-1] // trailing comma is formatting
			}
			if t == ";" {
				return // optional in JS; formatters add or remove them
			}
			out = append(out, t)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(n)
	return out
}

var loggers = map[string]bool{"console": true, "logger": true, "log": true}

// isLogging reports a statement that is only a logging call: console.log(...),
// logger.info(...), this.logger.warn(...).
func isLogging(n *treesitter.Node, src []byte) bool {
	if n.Type != "expression_statement" {
		return false
	}
	var call *treesitter.Node
	for _, c := range n.Children {
		if c.Named && !c.Extra {
			call = c
			break
		}
	}
	if call == nil || call.Type != "call_expression" {
		return false
	}
	fn := call.Child("function")
	if fn == nil || fn.Type != "member_expression" {
		return false
	}
	obj := fn.Child("object")
	if obj == nil {
		return false
	}
	if obj.Type == "member_expression" { // this.logger.x
		if prop := obj.Child("property"); prop != nil {
			return loggers[prop.Text(src)]
		}
	}
	return obj.Type == "identifier" && loggers[obj.Text(src)]
}
