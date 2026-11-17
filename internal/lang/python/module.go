package python

import (
	"strings"

	"github.com/byabasaija/indoc/internal/treesitter"
)

// Module is the statically visible structure of a Python file: imports,
// module- and class-level assignments, module-level call statements, classes,
// and functions. Function bodies are not walked for those; AllCalls and
// Subscripts cover the whole file, bodies included.
type Module struct {
	Imports    []Import
	Assigns    []Assign
	Calls      []CallStmt
	Classes    map[string]*Class
	Funcs      map[string]*Func
	AllCalls   []*Call     // every call in the file
	Subscripts []Subscript // every x[key] in the file
}

// Import is one imported binding.
//
//	import a.b         -> Local "a",  Module "a",    Name ""
//	import a.b as c    -> Local "c",  Module "a.b",  Name ""
//	from .x import y   -> Local "y",  Module "x",    Name "y", Level 1
type Import struct {
	Local  string
	Module string
	Name   string
	Level  int
	Line   int
}

// Assign is `target = value`, `target += value`, or an annotated
// declaration `target: T [= value]`. Value is nil for a bare annotation.
type Assign struct {
	Target     string
	Op         string // "=", "+=", or ":" (annotation without a value)
	Value      Expr
	Annotation Expr // nil when not annotated
	Line       int
	Cond       bool // inside if/for/while/match at module level
}

// CallStmt is a bare call statement such as `router.register(...)`.
type CallStmt struct {
	Call *Call
	Line int
	Cond bool
}

// Class is a class definition.
type Class struct {
	Name    string
	Bases   []Expr
	Line    int
	Methods map[string]*Func
	Assigns []Assign
}

// Func is a function or method definition.
type Func struct {
	Name       string
	Decorators []Expr
	Line       int
}

// Subscript is `value[key]`.
type Subscript struct {
	Value Expr
	Key   Expr
	Line  int
}

// ParseModule reads the structure of src. It never fails: unparseable input
// yields an empty module.
func ParseModule(src string) *Module {
	m := &Module{Classes: map[string]*Class{}, Funcs: map[string]*Func{}}
	b := []byte(src)
	root, err := treesitter.Parse("python", b)
	if err != nil {
		return m
	}
	mb := &moduleBuilder{m: m, src: b}
	mb.statements(root, nil, false)
	root.Walk(func(n *treesitter.Node) bool {
		switch n.Type {
		case "call":
			m.AllCalls = append(m.AllCalls, call(n, b))
		case "subscript":
			m.Subscripts = append(m.Subscripts, Subscript{
				Value: expr(n.Child("value"), b), Key: expr(n.Child("subscript"), b), Line: n.StartRow + 1,
			})
		}
		return true
	})
	return m
}

type moduleBuilder struct {
	m   *Module
	src []byte
}

// statements reads the statements of a module, class body, or nested block.
// cls is the enclosing class, if any; cond is true inside if/for/while/match.
func (mb *moduleBuilder) statements(n *treesitter.Node, cls *Class, cond bool) {
	for _, c := range named(n) {
		switch c.Type {
		case "import_statement", "import_from_statement":
			if cls == nil {
				mb.m.Imports = append(mb.m.Imports, mb.imports(c)...)
			}
		case "expression_statement":
			mb.expressionStatement(c, cls, cond)
		case "class_definition":
			mb.class(c, cls)
		case "function_definition":
			mb.function(c, nil, cls)
		case "decorated_definition":
			var decorators []Expr
			for _, d := range c.Children {
				if d.Type == "decorator" {
					if inner := named(d); len(inner) == 1 {
						decorators = append(decorators, expr(inner[0], mb.src))
					}
				}
			}
			def := c.Child("definition")
			switch {
			case def == nil:
			case def.Type == "class_definition":
				mb.class(def, cls)
			case def.Type == "function_definition":
				mb.function(def, decorators, cls)
			}
		case "if_statement", "for_statement", "while_statement", "match_statement":
			mb.nested(c, cls, true)
		case "try_statement", "with_statement":
			mb.nested(c, cls, cond)
		}
	}
}

// nested reads the blocks of a compound statement (including elif, else,
// except, finally, and case clauses).
func (mb *moduleBuilder) nested(n *treesitter.Node, cls *Class, cond bool) {
	for _, c := range named(n) {
		switch c.Type {
		case "block":
			mb.statements(c, cls, cond)
		case "elif_clause", "else_clause", "except_clause", "except_group_clause", "finally_clause", "case_clause", "block_body":
			mb.nested(c, cls, cond)
		}
	}
}

func (mb *moduleBuilder) expressionStatement(n *treesitter.Node, cls *Class, cond bool) {
	parts := named(n)
	if len(parts) != 1 {
		return
	}
	e := parts[0]
	line := n.StartRow + 1
	switch e.Type {
	case "assignment", "augmented_assignment":
		left := e.Child("left")
		if left == nil || left.Type != "identifier" {
			return
		}
		a := Assign{Target: left.Text(mb.src), Line: line, Cond: cond}
		if e.Type == "augmented_assignment" {
			op := e.Child("operator")
			if op == nil || op.Text(mb.src) != "+=" {
				return
			}
			a.Op = "+="
			a.Value = expr(e.Child("right"), mb.src)
		} else {
			if t := e.Child("type"); t != nil {
				a.Annotation = expr(firstNamed(t), mb.src)
			}
			if right := e.Child("right"); right != nil {
				a.Op, a.Value = "=", expr(right, mb.src)
			} else {
				a.Op = ":"
			}
		}
		if cls != nil {
			cls.Assigns = append(cls.Assigns, a)
		} else {
			mb.m.Assigns = append(mb.m.Assigns, a)
		}
	case "call":
		if cls == nil {
			mb.m.Calls = append(mb.m.Calls, CallStmt{Call: call(e, mb.src), Line: line, Cond: cond})
		}
	}
}

func firstNamed(n *treesitter.Node) *treesitter.Node {
	if parts := named(n); len(parts) > 0 {
		return parts[0]
	}
	return n
}

func (mb *moduleBuilder) class(n *treesitter.Node, outer *Class) {
	name := n.Child("name")
	if name == nil {
		return
	}
	c := &Class{Name: name.Text(mb.src), Line: keywordRow(n) + 1, Methods: map[string]*Func{}}
	if sup := n.Child("superclasses"); sup != nil {
		for _, b := range named(sup) {
			if b.Type != "keyword_argument" {
				c.Bases = append(c.Bases, expr(b, mb.src))
			}
		}
	}
	if outer == nil {
		mb.m.Classes[c.Name] = c
	}
	if body := n.Child("body"); body != nil {
		mb.statements(body, c, false)
	}
}

func (mb *moduleBuilder) function(n *treesitter.Node, decorators []Expr, cls *Class) {
	name := n.Child("name")
	if name == nil {
		return
	}
	f := &Func{Name: name.Text(mb.src), Decorators: decorators, Line: keywordRow(n) + 1}
	if cls != nil {
		cls.Methods[f.Name] = f
	} else {
		mb.m.Funcs[f.Name] = f
	}
}

func (mb *moduleBuilder) imports(n *treesitter.Node) []Import {
	line := n.StartRow + 1
	var out []Import
	if n.Type == "import_statement" {
		for _, c := range n.Children {
			if c.Field != "name" {
				continue
			}
			switch c.Type {
			case "dotted_name":
				mod := c.Text(mb.src)
				first, _, _ := strings.Cut(mod, ".")
				out = append(out, Import{Local: first, Module: first, Line: line})
			case "aliased_import":
				if nm, alias := c.Child("name"), c.Child("alias"); nm != nil && alias != nil {
					out = append(out, Import{Local: alias.Text(mb.src), Module: nm.Text(mb.src), Line: line})
				}
			}
		}
		return out
	}
	module, level := "", 0
	if mn := n.Child("module_name"); mn != nil {
		if mn.Type == "relative_import" {
			for _, c := range mn.Children {
				switch c.Type {
				case "import_prefix":
					level = strings.Count(c.Text(mb.src), ".")
				case "dotted_name":
					module = c.Text(mb.src)
				}
			}
		} else {
			module = mn.Text(mb.src)
		}
	}
	for _, c := range n.Children {
		if c.Field != "name" {
			continue
		}
		switch c.Type {
		case "dotted_name":
			nm := c.Text(mb.src)
			out = append(out, Import{Local: nm, Module: module, Name: nm, Level: level, Line: line})
		case "aliased_import":
			if nm, alias := c.Child("name"), c.Child("alias"); nm != nil && alias != nil {
				out = append(out, Import{Local: alias.Text(mb.src), Module: module, Name: nm.Text(mb.src), Level: level, Line: line})
			}
		}
	}
	return out
}
