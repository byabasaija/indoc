package reference

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/python"
)

// fastapi resolves FastAPI applications statically: route decorators,
// add_api_route, APIRouter prefixes, include_router (also nested), mounts,
// and the built-in documentation routes.
type fastapi struct {
	py       *pyProject
	cov      *Coverage
	routes   []doc.RefEntry
	visiting map[string]bool
	read     map[string]bool
}

var fastapiMethods = map[string]string{
	"get": "GET", "post": "POST", "put": "PUT", "patch": "PATCH", "delete": "DELETE",
	"options": "OPTIONS", "head": "HEAD", "trace": "TRACE",
}

func extractFastAPI(root string, files []string) ([]doc.RefEntry, *Coverage) {
	var pyFiles []string
	for _, f := range files {
		if strings.HasSuffix(f, ".py") {
			pyFiles = append(pyFiles, f)
		}
	}
	py := newPyProject(root, pyFiles)
	f := &fastapi{py: py, cov: &Coverage{Extractor: "fastapi"}, visiting: map[string]bool{}, read: map[string]bool{}}

	type app struct {
		file, name string
		ctor       *python.Call
	}
	var apps []app
	for _, file := range pyFiles {
		if !strings.Contains(readFile(root, file), "FastAPI") {
			continue
		}
		for _, a := range py.load(file).Assigns {
			if c, ok := a.Value.(*python.Call); ok && lastSeg(c.FuncName()) == "FastAPI" {
				if r := py.lookup(file, c.FuncName()); r.kind == "external" && strings.HasPrefix(r.external, "fastapi") || c.FuncName() == "FastAPI" || c.FuncName() == "fastapi.FastAPI" {
					apps = append(apps, app{file: file, name: a.Target, ctor: c})
					f.cov.Evidence = append(f.cov.Evidence, fmt.Sprintf("%s = FastAPI(...) (%s:%d)", a.Target, file, a.Line))
				}
			}
		}
	}
	if len(apps) == 0 {
		return nil, nil
	}
	mounted := map[string]bool{}
	for _, a := range apps {
		for _, c := range py.load(a.file).Calls {
			if c.Call.FuncName() == a.name+".mount" {
				if n, ok := c.Call.Arg(1, "app").(*python.Dotted); ok {
					if r := py.lookup(a.file, n.Path); r.kind == "assign" {
						mounted[r.file+":"+r.name] = true
					}
				}
			}
		}
	}
	for _, a := range apps {
		if mounted[a.file+":"+a.name] {
			continue // documented under the app that mounts it
		}
		f.app(a.file, a.name, a.ctor, "")
	}
	f.cov.Files = len(f.read)
	f.cov.Entries = len(f.routes)
	if len(f.routes) == 0 {
		f.cov.Warnings = append(f.cov.Warnings, "FastAPI app found but no routes were extracted")
	}
	shadowWarnings(f.routes, f.cov)
	return f.routes, f.cov
}

func readFile(root, file string) string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil || len(data) > maxFileBytes {
		return ""
	}
	return string(data)
}

// app emits an application's own routes, its routers, and built-in docs routes.
func (f *fastapi) app(file, name string, ctor *python.Call, prefix string) {
	f.router(file, name, prefix)
	docs := builtinDocs(ctor)
	for _, d := range docs {
		f.routes = append(f.routes, doc.RefEntry{
			Name: prefix + d.path, Detail: "GET", Handler: "FastAPI " + d.what,
			Source: fmt.Sprintf("%s:%d", file, ctor.Line),
		})
	}
}

type builtinRoute struct{ path, what string }

// builtinDocs lists FastAPI's automatic routes, honoring openapi_url,
// docs_url, redoc_url, and swagger_ui_oauth2_redirect_url (None disables).
func builtinDocs(ctor *python.Call) []builtinRoute {
	value := func(kw, def string) (string, bool) {
		switch v := ctor.Kw(kw).(type) {
		case nil:
			return def, true
		case *python.Str:
			return v.Value, v.Literal
		case *python.Dotted:
			if v.Path == "None" {
				return "", false
			}
		}
		return "", false
	}
	openapi, ok := value("openapi_url", "/openapi.json")
	if !ok || openapi == "" {
		return nil
	}
	out := []builtinRoute{{openapi, "OpenAPI schema"}}
	if docs, ok := value("docs_url", "/docs"); ok && docs != "" {
		out = append(out, builtinRoute{docs, "Swagger UI"})
		if redirect, ok := value("swagger_ui_oauth2_redirect_url", "/docs/oauth2-redirect"); ok && redirect != "" {
			out = append(out, builtinRoute{redirect, "Swagger UI OAuth2 redirect"})
		}
	}
	if redoc, ok := value("redoc_url", "/redoc"); ok && redoc != "" {
		out = append(out, builtinRoute{redoc, "ReDoc"})
	}
	return out
}

// router emits the routes registered on variable `name` in file (an app or
// an APIRouter), prefixed by prefix and the router's own prefix.
func (f *fastapi) router(file, name, prefix string) {
	key := file + ":" + name
	if f.visiting[key] {
		return
	}
	f.visiting[key] = true
	defer delete(f.visiting, key)
	f.read[file] = true

	m := f.py.load(file)
	own := ""
	for _, a := range m.Assigns {
		if c, ok := a.Value.(*python.Call); ok && a.Target == name && lastSeg(c.FuncName()) == "APIRouter" {
			if s, ok := c.Kw("prefix").(*python.Str); ok {
				own = s.Value
			}
		}
	}
	base := prefix + own

	// Decorated functions, in source order.
	type item struct {
		line int
		emit func()
	}
	var items []item
	for _, fn := range m.Funcs {
		fn := fn
		for _, dec := range fn.Decorators {
			c, ok := dec.(*python.Call)
			if !ok {
				continue
			}
			callee := c.FuncName()
			if !strings.HasPrefix(callee, name+".") {
				continue
			}
			method := strings.TrimPrefix(callee, name+".")
			var methods []string
			switch {
			case fastapiMethods[method] != "":
				methods = []string{fastapiMethods[method]}
			case method == "api_route":
				methods = methodList(c.Kw("methods"))
			case method == "websocket":
				methods = []string{"WS"}
			default:
				continue
			}
			path, ok := literalPath(c.Arg(0, "path"))
			if !ok {
				f.note(file, c.Line, "route path %s is not a literal", argText(c.Arg(0, "path")))
			}
			items = append(items, item{line: fn.Line, emit: func() {
				f.add(base+path, methods, fn.Name, file, c.Line)
			}})
		}
	}
	for _, cs := range m.Calls {
		c := cs.Call
		switch c.FuncName() {
		case name + ".add_api_route", name + ".add_api_websocket_route":
			path, ok := literalPath(c.Arg(0, "path"))
			if !ok {
				f.note(file, c.Line, "route path %s is not a literal", argText(c.Arg(0, "path")))
			}
			methods := methodList(c.Kw("methods"))
			if strings.HasSuffix(c.FuncName(), "websocket_route") {
				methods = []string{"WS"}
			}
			handler := argText(c.Arg(1, "endpoint"))
			items = append(items, item{line: cs.Line, emit: func() { f.add(base+path, methods, handler, file, c.Line) }})
		case name + ".include_router":
			inc := ""
			if s, ok := c.Kw("prefix").(*python.Str); ok {
				inc = s.Value
			}
			target, ok := c.Arg(0, "router").(*python.Dotted)
			if !ok {
				f.note(file, c.Line, "include_router(%s) not resolved", argText(c.Arg(0, "router")))
				continue
			}
			r := f.py.lookup(file, target.Path)
			line := cs.Line
			switch {
			case r.kind == "assign":
				items = append(items, item{line: line, emit: func() { f.router(r.file, r.name, base+inc) }})
			case r.kind == "module" || strings.HasSuffix(target.Path, ".router"):
				// include_router(api.router) where api is a module.
				mod := f.py.lookup(file, strings.TrimSuffix(target.Path, ".router"))
				if mod.kind == "module" {
					items = append(items, item{line: line, emit: func() { f.router(mod.file, "router", base+inc) }})
					continue
				}
				fallthrough
			default:
				f.note(file, line, "include_router(%s) not resolved", target.Path)
			}
		case name + ".mount":
			path, _ := literalPath(c.Arg(0, "path"))
			sub, _ := c.Arg(1, "app").(*python.Dotted)
			line := cs.Line
			if sub != nil {
				if r := f.py.lookup(file, sub.Path); r.kind == "assign" {
					if ctor := f.fastapiCtor(r.file, r.name); ctor != nil {
						items = append(items, item{line: line, emit: func() { f.app(r.file, r.name, ctor, base+path) }})
						continue
					}
				}
			}
			what := argText(c.Arg(1, "app"))
			items = append(items, item{line: line, emit: func() {
				f.routes = append(f.routes, doc.RefEntry{
					Name: base + path + "/*", Detail: "MOUNT", Handler: what,
					Source:      fmt.Sprintf("%s:%d", file, c.Line),
					Description: "Mounted application; its routes are not listed.",
				})
			}})
		}
	}
	sortItems := func() {
		for i := 1; i < len(items); i++ {
			for j := i; j > 0 && items[j].line < items[j-1].line; j-- {
				items[j], items[j-1] = items[j-1], items[j]
			}
		}
	}
	sortItems()
	for _, it := range items {
		it.emit()
	}
}

func (f *fastapi) fastapiCtor(file, name string) *python.Call {
	for _, a := range f.py.load(file).Assigns {
		if c, ok := a.Value.(*python.Call); ok && a.Target == name && lastSeg(c.FuncName()) == "FastAPI" {
			return c
		}
	}
	return nil
}

func (f *fastapi) add(path string, methods []string, handler, file string, line int) {
	detail := strings.Join(methods, ", ")
	if detail == "" {
		detail = "GET"
	}
	f.routes = append(f.routes, doc.RefEntry{Name: path, Detail: detail, Handler: handler, Source: fmt.Sprintf("%s:%d", file, line)})
}

func (f *fastapi) note(file string, line int, format string, args ...any) {
	f.cov.Unresolved = append(f.cov.Unresolved, fmt.Sprintf("%s:%d: %s", file, line, fmt.Sprintf(format, args...)))
}

func literalPath(x python.Expr) (string, bool) {
	if s, ok := x.(*python.Str); ok && s.Literal {
		return s.Value, true
	}
	if x == nil {
		return "", false
	}
	return "{" + x.Text() + "}", false
}

func argText(x python.Expr) string {
	if x == nil {
		return "?"
	}
	return x.Text()
}

// methodList reads methods=["GET", "POST"]; FastAPI defaults to GET.
func methodList(x python.Expr) []string {
	seq, ok := x.(*python.Seq)
	if !ok {
		return []string{"GET"}
	}
	set := map[string]bool{}
	for _, e := range seq.Elems {
		if s, ok := e.(*python.Str); ok {
			set[strings.ToUpper(s.Value)] = true
		}
	}
	out := ordered(set)
	for _, m := range []string{"OPTIONS", "HEAD", "TRACE"} {
		if set[m] {
			out = append(out, m)
		}
	}
	return out
}

// shadowWarnings reports routes an earlier route with the same path and an
// overlapping method makes unreachable.
func shadowWarnings(routes []doc.RefEntry, cov *Coverage) {
	type seen struct {
		methods map[string]bool
		entry   doc.RefEntry
	}
	first := map[string][]seen{}
	for _, r := range routes {
		if r.Detail == "MOUNT" || r.Detail == "INCLUDE" {
			continue
		}
		ms := map[string]bool{}
		for _, m := range strings.Split(r.Detail, ", ") {
			ms[m] = true
		}
		for _, prev := range first[r.Name] {
			for m := range ms {
				if prev.methods[m] {
					cov.Warnings = append(cov.Warnings, fmt.Sprintf("%s %s (%s, %s) is unreachable: %s (%s) matches it first",
						m, r.Name, r.Handler, r.Source, prev.entry.Handler, prev.entry.Source))
				}
			}
		}
		first[r.Name] = append(first[r.Name], seen{ms, r})
	}
}
