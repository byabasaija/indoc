package python

import "strings"

// Module is the statically visible structure of a Python file: imports,
// module- and class-level assignments, module-level call statements, classes,
// and functions. Function bodies are not analyzed.
type Module struct {
	Imports []Import
	Assigns []Assign
	Calls   []CallStmt
	Classes map[string]*Class
	Funcs   map[string]*Func
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

// Assign is `target = value` or `target += value`.
type Assign struct {
	Target string
	Op     string // "=" or "+="
	Value  Expr
	Line   int
	Cond   bool // inside if/for/while/match at module level
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

type frame struct {
	kind string // "class", "def", "cond", "block"
	cls  *Class
}

// ParseModule reads the structure of src. It never fails.
func ParseModule(src string) *Module {
	m := &Module{Classes: map[string]*Class{}, Funcs: map[string]*Func{}}
	toks := Tokenize(src)
	var (
		stack      []frame
		stmt       []Token
		pending    *frame // frame to push if the next token is INDENT
		decorators []Expr
	)
	inDef := func() bool {
		for _, f := range stack {
			if f.kind == "def" {
				return true
			}
		}
		return false
	}
	cond := func() bool {
		for _, f := range stack {
			if f.kind == "cond" {
				return true
			}
		}
		return false
	}
	class := func() *Class { // innermost class, when directly in its body
		for i := len(stack) - 1; i >= 0; i-- {
			switch stack[i].kind {
			case "class":
				return stack[i].cls
			case "def":
				return nil
			}
		}
		return nil
	}

	handle := func(s []Token) {
		pending = nil
		if len(s) == 0 || inDef() {
			if len(s) > 0 && endsWithColon(s) {
				pending = &frame{kind: "block"}
			}
			return
		}
		first := s[0]
		line := first.Line
		switch {
		case first.Kind == Op && first.Text == "@":
			decorators = append(decorators, ParseExpr(s[1:]))
			return
		case first.Text == "class" && first.Kind == Name:
			c := &Class{Line: line, Methods: map[string]*Func{}}
			if len(s) > 1 {
				c.Name = s[1].Text
			}
			if len(s) > 3 && s[2].Text == "(" {
				if call, ok := ParseExpr(s[1 : len(s)-1]).(*Call); ok {
					c.Bases = call.Args
				}
			}
			if outer := class(); outer == nil && len(stack) == countKind(stack, "cond", "block") {
				m.Classes[c.Name] = c
			}
			decorators = nil
			pending = &frame{kind: "class", cls: c}
			return
		case first.Kind == Name && (first.Text == "def" || first.Text == "async" && len(s) > 1 && s[1].Text == "def"):
			nameAt := 1
			if first.Text == "async" {
				nameAt = 2
			}
			f := &Func{Decorators: decorators, Line: line}
			if len(s) > nameAt {
				f.Name = s[nameAt].Text
			}
			decorators = nil
			if c := class(); c != nil {
				c.Methods[f.Name] = f
			} else if len(stack) == countKind(stack, "cond", "block") {
				m.Funcs[f.Name] = f
			}
			pending = &frame{kind: "def"}
			return
		}
		decorators = nil
		if endsWithColon(s) {
			kind := "block"
			switch first.Text {
			case "if", "elif", "else", "for", "while", "match", "case":
				kind = "cond"
			}
			pending = &frame{kind: kind}
			return
		}
		c := class()
		if c == nil && len(stack) != countKind(stack, "cond", "block") {
			return
		}
		if c == nil {
			if imp, ok := parseImport(s); ok {
				for i := range imp {
					imp[i].Line = line
				}
				m.Imports = append(m.Imports, imp...)
				return
			}
		}
		if a, ok := parseAssign(s); ok {
			a.Line = line
			a.Cond = cond()
			if c != nil {
				c.Assigns = append(c.Assigns, a)
			} else {
				m.Assigns = append(m.Assigns, a)
			}
			return
		}
		if c == nil {
			if call, ok := ParseExpr(s).(*Call); ok {
				m.Calls = append(m.Calls, CallStmt{Call: call, Line: line, Cond: cond()})
			}
		}
	}

	for _, t := range toks {
		switch t.Kind {
		case Comment, NL:
		case Newline:
			handle(stmt)
			stmt = stmt[:0]
		case Indent:
			if pending != nil {
				stack = append(stack, *pending)
			} else {
				stack = append(stack, frame{kind: "block"})
			}
			pending = nil
		case Dedent:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		case EOF:
			handle(stmt)
		default:
			stmt = append(stmt, t)
		}
	}
	return m
}

func countKind(stack []frame, kinds ...string) int {
	n := 0
	for _, f := range stack {
		for _, k := range kinds {
			if f.kind == k {
				n++
				break
			}
		}
	}
	return n
}

func endsWithColon(s []Token) bool {
	last := s[len(s)-1]
	return last.Kind == Op && last.Text == ":"
}

func parseImport(s []Token) ([]Import, bool) {
	switch s[0].Text {
	case "import":
		var out []Import
		for _, part := range splitTop(s[1:], ",") {
			mod, alias := dotted(part)
			if mod == "" {
				continue
			}
			local := alias
			target := mod
			if local == "" {
				local, _, _ = strings.Cut(mod, ".")
				target = local
			}
			out = append(out, Import{Local: local, Module: target})
		}
		return out, true
	case "from":
		i := 1
		level := 0
		for i < len(s) && (s[i].Text == "." || s[i].Text == "...") {
			level += len(s[i].Text)
			i++
		}
		var mod []string
		for i < len(s) && s[i].Text != "import" {
			if s[i].Kind == Name {
				mod = append(mod, s[i].Text)
			}
			i++
		}
		if i >= len(s) {
			return nil, false
		}
		names := s[i+1:]
		if len(names) > 0 && names[0].Text == "(" {
			names = names[1:]
			if n := len(names); n > 0 && names[n-1].Text == ")" {
				names = names[:n-1]
			}
		}
		var out []Import
		for _, part := range splitTop(names, ",") {
			name, alias := dotted(part)
			if name == "" || name == "*" {
				continue
			}
			local := name
			if alias != "" {
				local = alias
			}
			out = append(out, Import{Local: local, Module: strings.Join(mod, "."), Name: name, Level: level})
		}
		return out, true
	}
	return nil, false
}

// dotted reads `a.b.c [as d]`.
func dotted(toks []Token) (name, alias string) {
	var parts []string
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if t.Kind == Name && t.Text == "as" && i+1 < len(toks) {
			return strings.Join(parts, ""), toks[i+1].Text
		}
		parts = append(parts, t.Text)
	}
	return strings.Join(parts, ""), ""
}

// splitTop splits tokens on sep at bracket depth 0.
func splitTop(toks []Token, sep string) [][]Token {
	var out [][]Token
	depth, start := 0, 0
	for i, t := range toks {
		if t.Kind != Op {
			continue
		}
		switch t.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case sep:
			if depth == 0 {
				out = append(out, toks[start:i])
				start = i + 1
			}
		}
	}
	if start < len(toks) {
		out = append(out, toks[start:])
	}
	return out
}

// parseAssign recognizes `name = value`, `name: T = value`, and `name += value`.
func parseAssign(s []Token) (Assign, bool) {
	if len(s) < 3 || s[0].Kind != Name || isKeyword(s[0].Text) {
		return Assign{}, false
	}
	depth := 0
	for i := 1; i < len(s); i++ {
		t := s[i]
		if t.Kind != Op {
			continue
		}
		switch t.Text {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case "=", "+=":
			if depth != 0 {
				continue
			}
			// Only a plain name (optionally annotated) may precede the operator.
			if i != 1 && !(s[1].Text == ":" && t.Text == "=") {
				return Assign{}, false
			}
			return Assign{Target: s[0].Text, Op: t.Text, Value: ParseExpr(s[i+1:])}, true
		}
	}
	return Assign{}, false
}
