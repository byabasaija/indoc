package reference

import (
	"fmt"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/jsts"
)

var nestMethods = map[string]string{
	"Get": "GET", "Post": "POST", "Put": "PUT", "Patch": "PATCH", "Delete": "DELETE",
	"Options": "OPTIONS", "Head": "HEAD", "All": "ANY",
}

// extractNest lists NestJS routes: @Controller(path) classes and their
// @Get/@Post/... methods, under the app's setGlobalPrefix.
func extractNest(root string, files []string, js *jsProject) ([]doc.RefEntry, *Coverage) {
	var srcFiles []string
	for f := range js.files {
		if strings.Contains(f, "node_modules/") || isTest(f) {
			continue
		}
		if strings.Contains(readFile(root, f), "@nestjs/") {
			srcFiles = append(srcFiles, f)
		}
	}
	if len(srcFiles) == 0 {
		return nil, nil
	}
	sort.Strings(srcFiles)
	cov := &Coverage{Extractor: "nestjs", Evidence: []string{"@nestjs/common"}}

	prefix := ""
	for _, f := range srcFiles {
		for _, c := range js.load(f).Calls {
			name := c.CalleeName()
			switch {
			case strings.HasSuffix(name, ".setGlobalPrefix"):
				if s, ok := c.Arg(0).(*jsts.Str); ok {
					prefix = s.Value
					if len(c.Args) > 1 {
						cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s:%d: setGlobalPrefix exclusions are not applied", f, c.Line))
					}
				} else {
					cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s:%d: global prefix is not a literal", f, c.Line))
				}
			case strings.HasSuffix(name, ".enableVersioning"):
				cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s:%d: API versioning is enabled; version segments are not shown", f, c.Line))
			case name == "RouterModule.register":
				cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s:%d: RouterModule prefixes are not applied", f, c.Line))
			}
		}
	}

	var routes []doc.RefEntry
	for _, f := range srcFiles {
		m := js.load(f)
		nest := nestImports(m)
		for _, cls := range m.Classes {
			var base []string
			isController := false
			for _, d := range cls.Decorators {
				if nest[d.CalleeName()] != "Controller" {
					continue
				}
				isController = true
				var ok bool
				if base, ok = nestPaths(d.Arg(0)); !ok {
					cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s:%d: @Controller path %s is not a literal", f, d.Line, d.Arg(0).Text()))
					base = []string{"{" + d.Arg(0).Text() + "}"}
				}
			}
			if !isController {
				continue
			}
			cov.Files++
			for _, meth := range cls.Methods {
				for _, d := range meth.Decorators {
					method, ok := nestMethods[nest[d.CalleeName()]]
					if !ok {
						continue
					}
					sub, ok := nestPaths(d.Arg(0))
					if !ok {
						cov.Unresolved = append(cov.Unresolved, fmt.Sprintf("%s:%d: @%s path %s is not a literal", f, d.Line, d.CalleeName(), d.Arg(0).Text()))
						sub = []string{"{" + d.Arg(0).Text() + "}"}
					}
					for _, b := range base {
						for _, s := range sub {
							routes = append(routes, doc.RefEntry{
								Name: nestJoin(prefix, b, s), Detail: method,
								Handler: cls.Name + "." + meth.Name, Source: fmt.Sprintf("%s:%d", f, meth.Line),
							})
						}
					}
				}
			}
		}
	}
	cov.Entries = len(routes)
	if cov.Entries == 0 {
		cov.Warnings = append(cov.Warnings, "NestJS detected but no controller routes were extracted")
	}
	shadowWarnings(routes, cov)
	return routes, cov
}

// nestImports maps local names to the @nestjs/common export they import.
func nestImports(m *jsts.Module) map[string]string {
	out := map[string]string{}
	for _, imp := range m.Imports {
		if imp.Source == "@nestjs/common" && imp.Name != "default" && imp.Name != "*" {
			out[imp.Local] = imp.Name
		}
	}
	return out
}

// nestPaths reads a decorator path argument: none, a string, an array of
// strings, or { path: ... }.
func nestPaths(x jsts.Expr) ([]string, bool) {
	if x == nil {
		return []string{""}, true
	}
	if ps, ok := pathValues(x); ok {
		return ps, true
	}
	if o, ok := x.(*jsts.Opaque); ok && strings.HasPrefix(o.Src, "{") {
		// { path: "users", ... }: read the path property textually.
		if i := strings.Index(o.Src, "path:"); i >= 0 {
			rest := strings.TrimSpace(o.Src[i+5:])
			if len(rest) > 1 && (rest[0] == '"' || rest[0] == '\'') {
				if j := strings.IndexByte(rest[1:], rest[0]); j >= 0 {
					return []string{rest[1 : j+1]}, true
				}
			}
		}
	}
	return nil, false
}

// nestJoin joins prefix, controller, and method paths the way Nest does.
func nestJoin(parts ...string) string {
	var segs []string
	for _, p := range parts {
		p = strings.Trim(p, "/")
		if p != "" {
			segs = append(segs, p)
		}
	}
	return "/" + strings.Join(segs, "/")
}
