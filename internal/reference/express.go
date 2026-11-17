package reference

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/jsts"
)

// jsProject resolves JavaScript/TypeScript modules and bindings statically.
type jsProject struct {
	root  string
	files map[string]bool
	mods  map[string]*jsts.Module
}

func newJSProject(root string, files []string) *jsProject {
	p := &jsProject{root: root, files: map[string]bool{}, mods: map[string]*jsts.Module{}}
	for _, f := range files {
		if jsts.Grammar(f) != "" {
			p.files[f] = true
		}
	}
	return p
}

func (p *jsProject) load(file string) *jsts.Module {
	if m, ok := p.mods[file]; ok {
		return m
	}
	m := jsts.ParseModule(file, []byte(readFile(p.root, file)))
	p.mods[file] = m
	return m
}

var jsExtensions = []string{"", ".js", ".ts", ".mjs", ".cjs", ".jsx", ".tsx", "/index.js", "/index.ts", "/index.mjs", "/index.cjs"}

// resolveModule maps a relative import from file to a repository file.
func (p *jsProject) resolveModule(from, source string) (string, bool) {
	if !strings.HasPrefix(source, ".") {
		return "", false
	}
	base := path.Join(path.Dir(from), source)
	// TypeScript ESM imports name the compiled .js file.
	trimmed := strings.TrimSuffix(base, path.Ext(base))
	for _, cand := range []string{base, trimmed} {
		for _, ext := range jsExtensions {
			if p.files[cand+ext] {
				return cand + ext, true
			}
		}
	}
	return "", false
}

// jsRef is a binding: a top-level name in a file, or a file's default export
// expression (name "").
type jsRef struct {
	file, name string
	external   string // package import such as "express" or "express#Router"
}

// resolve follows a name used in file to the binding that defines it.
func (p *jsProject) resolve(file, name string, depth int) (jsRef, bool) {
	if depth > 8 {
		return jsRef{}, false
	}
	m := p.load(file)
	head, rest, _ := strings.Cut(name, ".")
	if rest == "" {
		if _, ok := m.Bindings[name]; ok {
			return jsRef{file: file, name: name}, true
		}
	}
	for _, imp := range m.Imports {
		if imp.Local != head {
			continue
		}
		target, ok := p.resolveModule(file, imp.Source)
		if !ok {
			ext := imp.Source
			if imp.Name != "default" && imp.Name != "*" {
				ext += "#" + imp.Name
			}
			if rest != "" {
				ext += "." + rest
			}
			return jsRef{external: ext}, true
		}
		exported := imp.Name
		if imp.Name == "*" {
			if rest == "" {
				return jsRef{}, false
			}
			exported, rest = rest, ""
		}
		if rest != "" {
			return jsRef{}, false
		}
		return p.export(target, exported, depth+1)
	}
	return jsRef{}, false
}

// export resolves an exported name of file.
func (p *jsProject) export(file, name string, depth int) (jsRef, bool) {
	m := p.load(file)
	if local, ok := m.Exports[name]; ok {
		if r, ok := p.resolve(file, local, depth+1); ok {
			return r, true
		}
	}
	if name == "default" && m.Default != nil {
		return jsRef{file: file, name: ""}, true
	}
	return jsRef{}, false
}

// value returns the expression a binding holds.
func (p *jsProject) value(r jsRef) jsts.Expr {
	m := p.load(r.file)
	if r.name == "" {
		return m.Default
	}
	return m.Bindings[r.name]
}

// express extracts routes from Express applications: app.METHOD and
// router.METHOD, router.route(path).METHOD chains, and app.use/router.use
// mounts of routers and sub-applications, across files.
type express struct {
	js     *jsProject
	cov    *Coverage
	routes []doc.RefEntry
}

type routerKind int

const (
	notRouter routerKind = iota
	expressApp
	expressRouter
)

type registration struct {
	method, path, handler, file string
	line                       int
}

type mount struct {
	child    jsRef
	prefixes []string
	file     string
	line     int
}

var expressMethods = map[string]string{
	"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH", "delete": "DELETE",
	"options": "OPTIONS", "head": "HEAD", "all": "ANY",
}

func extractExpress(root string, files []string) ([]doc.RefEntry, *Coverage) {
	js := newJSProject(root, files)
	var srcFiles []string
	usesExpress := false
	for f := range js.files {
		if strings.Contains(f, "node_modules/") || isTest(f) {
			continue
		}
		srcFiles = append(srcFiles, f)
	}
	sort.Strings(srcFiles)
	for _, f := range srcFiles {
		if !strings.Contains(readFile(root, f), "express") {
			continue
		}
		for _, imp := range js.load(f).Imports {
			if imp.Source == "express" {
				usesExpress = true
			}
		}
	}
	if !usesExpress {
		return nil, nil
	}
	e := &express{js: js, cov: &Coverage{Extractor: "express", Evidence: []string{"express"}}}

	kinds := map[jsRef]routerKind{}
	regs := map[jsRef][]registration{}
	mounts := map[jsRef][]mount{}
	mounted := map[jsRef]bool{}
	var order []jsRef
	kindOf := func(r jsRef) routerKind {
		if k, ok := kinds[r]; ok {
			return k
		}
		k := e.classify(r)
		kinds[r] = k
		if k != notRouter {
			order = append(order, r)
		}
		return k
	}

	for _, f := range srcFiles {
		m := js.load(f)
		for _, c := range m.Calls {
			mem, method, receiver, routePath := e.target(c)
			if mem == "" {
				continue
			}
			ref, ok := js.resolve(f, receiver, 0)
			if !ok || ref.external != "" || kindOf(ref) == notRouter {
				continue
			}
			switch {
			case mem == "use":
				prefixes := []string{"/"}
				args := c.Args
				if len(args) > 0 {
					if ps, ok := pathValues(args[0]); ok {
						prefixes, args = ps, args[1:]
					}
				}
				for _, a := range flatten(args) {
					child, ok := e.routerArg(f, a)
					if ok && kindOf(child) != notRouter {
						mounts[ref] = append(mounts[ref], mount{child: child, prefixes: prefixes, file: f, line: c.Line})
						mounted[child] = true
					}
				}
			default:
				paths := []string{routePath}
				if routePath == "\x00" { // app.get(path, ...)
					if c.Arg(0) == nil {
						continue
					}
					var ok bool
					if paths, ok = pathValues(c.Arg(0)); !ok {
						paths = []string{"{" + c.Arg(0).Text() + "}"}
						e.note(f, c.Line, "route path %s is not a literal", c.Arg(0).Text())
					}
					if len(c.Args) < 2 {
						continue // app.get("setting") reads a setting
					}
				}
				for _, p := range paths {
					regs[ref] = append(regs[ref], registration{method: method, path: p, handler: handlerName(c), file: f, line: c.Line})
				}
			}
		}
	}

	var roots []jsRef
	for _, r := range order {
		if kinds[r] == expressApp && !mounted[r] {
			roots = append(roots, r)
		}
	}
	if len(roots) == 0 && len(order) > 0 {
		e.cov.Warnings = append(e.cov.Warnings, "Express routers found but no application that mounts them")
	}
	visiting := map[jsRef]bool{}
	var walk func(r jsRef, prefix string)
	walk = func(r jsRef, prefix string) {
		if visiting[r] {
			return
		}
		visiting[r] = true
		defer delete(visiting, r)
		for _, reg := range regs[r] {
			e.routes = append(e.routes, doc.RefEntry{
				Name: joinRoute(prefix, reg.path), Detail: reg.method, Handler: reg.handler,
				Source: fmt.Sprintf("%s:%d", reg.file, reg.line),
			})
		}
		for _, mt := range mounts[r] {
			for _, p := range mt.prefixes {
				walk(mt.child, joinRoute(prefix, p))
			}
		}
	}
	for _, r := range roots {
		walk(r, "")
	}
	for _, r := range order {
		if kinds[r] == expressRouter && !mounted[r] && len(regs[r]) > 0 {
			e.cov.Unresolved = append(e.cov.Unresolved, fmt.Sprintf("router %s in %s has routes but is never mounted with use()", nameOf(r), r.file))
		}
	}
	touched := map[string]bool{}
	for _, r := range order {
		touched[r.file] = true
	}
	e.cov.Files = len(touched)
	e.cov.Entries = len(e.routes)
	if len(e.routes) == 0 {
		e.cov.Warnings = append(e.cov.Warnings, "Express is used but no routes were extracted")
	}
	shadowWarnings(e.routes, e.cov)
	return e.routes, e.cov
}

func nameOf(r jsRef) string {
	if r.name == "" {
		return "default export"
	}
	return r.name
}

func (e *express) note(file string, line int, format string, args ...any) {
	e.cov.Unresolved = append(e.cov.Unresolved, fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf(format, args...)))
}

// classify reports whether a binding holds an Express app or router.
func (e *express) classify(r jsRef) routerKind {
	call, ok := e.js.value(r).(*jsts.Call)
	if !ok {
		return notRouter
	}
	// require("express")() and require("express").Router()
	if mem, ok := call.Callee.(*jsts.Member); ok && mem.Prop == "Router" && isRequireOf(mem.X, "express") {
		return expressRouter
	}
	if isRequireOf(call.Callee, "express") {
		return expressApp
	}
	callee := call.CalleeName()
	if callee == "" {
		return notRouter
	}
	ref, ok := e.js.resolve(r.file, callee, 0)
	if !ok {
		return notRouter
	}
	switch ref.external {
	case "express":
		return expressApp
	case "express.Router", "express#Router":
		return expressRouter
	}
	return notRouter
}

// target decodes a registration call: router.METHOD(...), router.use(...),
// or router.route(path).METHOD(...).METHOD(...). member is "use" or the
// method; routePath is "\x00" when the path is the call's first argument.
func (e *express) target(c *jsts.Call) (member, method, receiver, routePath string) {
	var obj jsts.Expr
	var prop string
	switch callee := c.Callee.(type) {
	case *jsts.Name:
		i := strings.LastIndex(callee.Path, ".")
		if i < 0 {
			return "", "", "", ""
		}
		receiver, prop = callee.Path[:i], callee.Path[i+1:]
	case *jsts.Member:
		obj, prop = callee.X, callee.Prop
	default:
		return "", "", "", ""
	}
	if obj != nil {
		// A chain: walk down to router.route(path).
		for {
			inner, ok := obj.(*jsts.Call)
			if !ok {
				return "", "", "", ""
			}
			if n, ok := inner.Callee.(*jsts.Name); ok && strings.HasSuffix(n.Path, ".route") {
				receiver = strings.TrimSuffix(n.Path, ".route")
				p, ok := pathValues(inner.Arg(0))
				if !ok || len(p) == 0 {
					return "", "", "", ""
				}
				routePath = p[0]
				break
			}
			mem, ok := inner.Callee.(*jsts.Member)
			if !ok {
				return "", "", "", ""
			}
			obj = mem.X
		}
	} else {
		routePath = "\x00"
	}
	if prop == "use" && obj == nil {
		return "use", "", receiver, ""
	}
	if m, ok := expressMethods[prop]; ok {
		return prop, m, receiver, routePath
	}
	return "", "", "", ""
}

// routerArg resolves a use() argument to a binding: a name, or an inline
// require("./routes/x").
func (e *express) routerArg(file string, a jsts.Expr) (jsRef, bool) {
	switch v := a.(type) {
	case *jsts.Name:
		return e.js.resolve(file, v.Path, 0)
	case *jsts.Call:
		if v.CalleeName() == "require" {
			if s, ok := v.Arg(0).(*jsts.Str); ok {
				if target, ok := e.js.resolveModule(file, s.Value); ok {
					return e.js.export(target, "default", 0)
				}
			}
		}
	}
	return jsRef{}, false
}

func flatten(args []jsts.Expr) []jsts.Expr {
	var out []jsts.Expr
	for _, a := range args {
		if arr, ok := a.(*jsts.Array); ok {
			out = append(out, flatten(arr.Elems)...)
			continue
		}
		out = append(out, a)
	}
	return out
}

// pathValues reads a path argument: a string or an array of strings.
func pathValues(x jsts.Expr) ([]string, bool) {
	switch v := x.(type) {
	case *jsts.Str:
		return []string{v.Value}, true
	case *jsts.Array:
		var out []string
		for _, el := range v.Elems {
			s, ok := el.(*jsts.Str)
			if !ok {
				return nil, false
			}
			out = append(out, s.Value)
		}
		return out, len(out) > 0
	}
	return nil, false
}

func handlerName(c *jsts.Call) string {
	if len(c.Args) == 0 {
		return ""
	}
	last := c.Args[len(c.Args)-1]
	switch v := last.(type) {
	case *jsts.Name:
		return v.Path
	case *jsts.Func:
		return "(inline)"
	case *jsts.Call:
		return v.Text()
	}
	return last.Text()
}

// joinRoute joins a mount prefix and a route path as Express matches them.
func joinRoute(prefix, p string) string {
	if prefix == "" || prefix == "/" {
		if p == "" {
			return "/"
		}
		return p
	}
	prefix = strings.TrimSuffix(prefix, "/")
	if p == "/" || p == "" {
		return prefix
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return prefix + p
}

func isRequireOf(x jsts.Expr, module string) bool {
	c, ok := x.(*jsts.Call)
	if !ok || c.CalleeName() != "require" {
		return false
	}
	s, ok := c.Arg(0).(*jsts.Str)
	return ok && s.Value == module
}
