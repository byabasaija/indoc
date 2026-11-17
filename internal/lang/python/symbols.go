// Package python reads Python source through tree-sitter: symbols with
// surface and body hashes for staleness, and a static module view (imports,
// assignments, classes, calls) for framework extractors. Python itself is
// never needed.
package python

import (
	"strings"

	"github.com/byabasaija/indoc/internal/lang"
	"github.com/byabasaija/indoc/internal/treesitter"
)

// File is a parsed file: its symbols and a whole-file hash.
type File struct {
	Symbols []lang.Symbol
	Hash    string
}

// ParseFile extracts every def, async def, and class (nested too, in source
// order) and hashes the file. Qualified names follow def/class nesting only:
// a function inside an if-block at module level has no prefix.
func ParseFile(src []byte) (File, error) {
	root, err := treesitter.Parse("python", src)
	if err != nil {
		return File{}, err
	}
	x := &extractor{src: src}
	x.block(root, nil)
	return File{Symbols: x.syms, Hash: lang.Hash(tokens(root, src, nil))}, nil
}

// Parse returns the symbols in src, or none if it cannot be parsed.
func Parse(src string) []lang.Symbol {
	f, _ := ParseFile([]byte(src))
	return f.Symbols
}

// FileHash hashes the whole file's significant tokens, so module-level edits
// (settings, urlpatterns) are visible even though no symbol owns them.
func FileHash(src string) string {
	f, _ := ParseFile([]byte(src))
	return f.Hash
}

type scope struct {
	name    string
	isClass bool
}

type extractor struct {
	src  []byte
	syms []lang.Symbol
}

// block visits statements; definitions nest scopes, other compound
// statements (if, try, with, ...) do not.
func (x *extractor) block(n *treesitter.Node, scopes []scope) {
	for _, c := range n.Children {
		switch c.Type {
		case "function_definition", "class_definition":
			x.definition(c, c, scopes)
		case "decorated_definition":
			if def := c.Child("definition"); def != nil {
				x.definition(def, c, scopes)
			}
		default:
			if c.Named && !c.Extra {
				x.block(c, scopes)
			}
		}
	}
}

func (x *extractor) definition(def, outer *treesitter.Node, scopes []scope) {
	name := def.Child("name")
	if name == nil {
		return
	}
	isClass := def.Type == "class_definition"
	inner := append(append([]scope{}, scopes...), scope{name: name.Text(x.src), isClass: isClass})
	kind := lang.KindFunction
	switch {
	case isClass:
		kind = lang.KindClass
	case len(scopes) > 0 && scopes[len(scopes)-1].isClass:
		kind = lang.KindMethod
	}
	body := def.Child("body")

	// Surface: decorators and the header up to the body.
	var surface []string
	if outer != def {
		for _, c := range outer.Children {
			if c.Type == "decorator" {
				surface = append(surface, tokens(c, x.src, nil)...)
			}
		}
	}
	surface = append(surface, tokens(def, x.src, []*treesitter.Node{body})...)

	// A class's body is its interface: attributes and member signatures.
	// Member bodies are excluded; each member has its own symbol.
	var exclude []*treesitter.Node
	if isClass && body != nil {
		exclude = memberBodies(body)
	}
	bodyHash := lang.Hash(nil)
	if body != nil {
		bodyHash = lang.Hash(tokens(body, x.src, exclude))
	}

	x.syms = append(x.syms, lang.Symbol{
		Name:          name.Text(x.src),
		Qual:          qualOf(inner),
		Kind:          kind,
		Line:          keywordRow(def) + 1,
		DecoratorLine: outer.StartRow + 1,
		EndLine:       lastCodeRow(def) + 1,
		Surface:       lang.Hash(surface),
		Body:          bodyHash,
		Private:       privateQual(inner),
	})
	if body != nil {
		x.block(body, inner)
	}
}

// memberBodies returns the bodies of definitions directly in a class body,
// including those inside if/try blocks, but not inside methods.
func memberBodies(n *treesitter.Node) []*treesitter.Node {
	var out []*treesitter.Node
	for _, c := range n.Children {
		switch c.Type {
		case "function_definition", "class_definition":
			if b := c.Child("body"); b != nil {
				out = append(out, b)
			}
		case "decorated_definition":
			if def := c.Child("definition"); def != nil {
				if b := def.Child("body"); b != nil {
					out = append(out, b)
				}
			}
		default:
			if c.Named && !c.Extra {
				out = append(out, memberBodies(c)...)
			}
		}
	}
	return out
}

func qualOf(scopes []scope) string {
	parts := make([]string, len(scopes))
	for i, s := range scopes {
		parts[i] = s.name
	}
	return strings.Join(parts, ".")
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

// keywordRow is the row of `def`, `async`, or `class`.
func keywordRow(def *treesitter.Node) int {
	for _, c := range def.Children {
		if !c.Extra {
			return c.StartRow
		}
	}
	return def.StartRow
}

// lastCodeRow is the end row of the last non-comment leaf, matching
// CPython's end_lineno, which excludes trailing comments.
func lastCodeRow(n *treesitter.Node) int {
	for i := len(n.Children) - 1; i >= 0; i-- {
		c := n.Children[i]
		if c.Extra || c.Type == "comment" {
			continue
		}
		if len(c.Children) == 0 {
			if c.End == c.Start {
				continue // zero-width nodes from error recovery
			}
			return c.EndRow
		}
		if r := lastCodeRow(c); r >= 0 {
			return r
		}
	}
	if len(n.Children) == 0 && n.End > n.Start && !n.Extra {
		return n.EndRow
	}
	return n.StartRow
}

var loggerNames = map[string]bool{
	"logger": true, "logging": true, "log": true, "LOGGER": true, "LOG": true,
	"_logger": true, "_log": true, "_LOGGER": true,
}

// tokens returns the leaf texts under n with comments, layout, trailing
// commas, and statements that cannot affect documented behavior removed:
// bare string statements (docstrings) and single logging calls. Block
// structure is kept as markers, so moving a statement in or out of a block
// counts as a change. Subtrees in exclude are skipped.
func tokens(n *treesitter.Node, src []byte, exclude []*treesitter.Node) []string {
	skip := map[*treesitter.Node]bool{}
	for _, e := range exclude {
		if e != nil {
			skip[e] = true
		}
	}
	var out []string
	var walk func(*treesitter.Node)
	walk = func(n *treesitter.Node) {
		if skip[n] || n.Extra || n.Type == "comment" || ignorable(n, src) {
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
			out = append(out, t)
			return
		}
		if n.Type == "block" {
			out = append(out, "{")
		}
		statements := n.Type == "block" || n.Type == "module"
		for _, c := range n.Children {
			walk(c)
			if statements && c.Named && !c.Extra && !skip[c] && !ignorable(c, src) {
				out = append(out, ";")
			}
		}
		if n.Type == "block" {
			out = append(out, "}")
		}
	}
	walk(n)
	return out
}

// ignorable reports a statement that is only a string (a docstring) or only
// a logging call such as logger.info(...) or self.log.warning(...).
func ignorable(n *treesitter.Node, src []byte) bool {
	if n.Type != "expression_statement" {
		return false
	}
	parts := named(n)
	if len(parts) != 1 {
		return false
	}
	e := parts[0]
	switch e.Type {
	case "string", "concatenated_string":
		return true
	case "call":
		fn := e.Child("function")
		if fn == nil || fn.Type != "attribute" {
			return false
		}
		obj := fn.Child("object")
		if obj != nil && obj.Type == "attribute" { // self.logger.x(...)
			if o := obj.Child("object"); o != nil && o.Text(src) == "self" {
				obj = obj.Child("attribute")
			}
		}
		return obj != nil && obj.Type == "identifier" && loggerNames[obj.Text(src)]
	}
	return false
}
