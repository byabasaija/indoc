package reference

import (
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/jsts"
)

// File-system routers: Next.js (App Router and Pages Router) and Astro.
// Routes come from file paths; HTTP methods of API routes and endpoints come
// from their exported handlers.

var routeHandlers = []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"}

// exportedMethods lists the HTTP method handlers a module exports
// (export function GET, export const POST = ..., export { handler as GET }).
func exportedMethods(m *jsts.Module, extra ...string) []string {
	var out []string
	for _, h := range append(append([]string{}, routeHandlers...), extra...) {
		if _, ok := m.Exports[h]; ok {
			out = append(out, h)
		}
	}
	return out
}

func extractNext(root string, files []string, js *jsProject) ([]doc.RefEntry, *Coverage) {
	usesNext := false
	for _, f := range files {
		if strings.HasPrefix(path.Base(f), "next.config.") {
			usesNext = true
		}
	}
	if !usesNext && !packageDepends(root, "next") {
		return nil, nil
	}
	cov := &Coverage{Extractor: "next.js", Evidence: []string{"next"}}
	base := nextBasePath(root, files)
	var routes []doc.RefEntry
	for _, f := range files {
		if strings.Contains(f, "node_modules/") || jsts.Grammar(f) == "" {
			continue
		}
		dir, file := path.Split(f)
		stem := strings.TrimSuffix(file, path.Ext(file))
		switch {
		case hasRoot(dir, "app/") || hasRoot(dir, "src/app/"):
			segs, ok := appSegments(strings.TrimPrefix(strings.TrimPrefix(dir, "src/"), "app/"))
			if !ok {
				continue
			}
			route := base + "/" + strings.Join(segs, "/")
			switch stem {
			case "page":
				routes = append(routes, doc.RefEntry{Name: cleanRoute(route), Detail: "GET", Handler: "page", Source: f + ":1"})
				cov.Files++
			case "route":
				methods := exportedMethods(js.load(f))
				if len(methods) == 0 {
					cov.Unresolved = append(cov.Unresolved, f+": route handler exports no HTTP method functions")
					continue
				}
				routes = append(routes, doc.RefEntry{Name: cleanRoute(route), Detail: strings.Join(methods, ", "), Handler: "route handler", Source: f + ":1"})
				cov.Files++
			}
		case hasRoot(dir, "pages/") || hasRoot(dir, "src/pages/"):
			rel := strings.TrimPrefix(strings.TrimPrefix(f, "src/"), "pages/")
			rel = strings.TrimSuffix(rel, path.Ext(rel))
			if strings.HasPrefix(path.Base(rel), "_") || strings.HasSuffix(rel, ".d") {
				continue // _app, _document, _error
			}
			parts := strings.Split(rel, "/")
			if parts[len(parts)-1] == "index" {
				parts = parts[:len(parts)-1]
			}
			route := cleanRoute(base + "/" + strings.Join(parts, "/"))
			e := doc.RefEntry{Name: route, Detail: "GET", Handler: "page", Source: f + ":1"}
			if parts[0] == "api" {
				e.Detail, e.Handler = "ANY", "API route"
			}
			routes = append(routes, e)
			cov.Files++
		}
	}
	cov.Entries = len(routes)
	if cov.Entries == 0 {
		cov.Warnings = append(cov.Warnings, "Next.js detected but no app/ or pages/ routes found")
	}
	return routes, cov
}

func hasRoot(dir, root string) bool { return dir == root || strings.HasPrefix(dir, root) }

// appSegments converts an App Router directory into URL segments. ok is
// false for directories that are not routable (_private, intercepting).
func appSegments(dir string) ([]string, bool) {
	var segs []string
	for _, s := range strings.Split(strings.Trim(dir, "/"), "/") {
		switch {
		case s == "":
		case strings.HasPrefix(s, "_"):
			return nil, false // private folder
		case strings.HasPrefix(s, "(.") || strings.HasPrefix(s, "(..)"):
			return nil, false // intercepting route
		case strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")"):
			// route group: not part of the URL
		case strings.HasPrefix(s, "@"):
			// parallel route slot: not part of the URL
		default:
			segs = append(segs, s)
		}
	}
	return segs, true
}

func cleanRoute(r string) string {
	r = path.Clean("/" + r)
	return r
}

var basePathRe = regexp.MustCompile(`basePath\s*:\s*["'\x60]([^"'\x60]+)["'\x60]`)

func nextBasePath(root string, files []string) string {
	for _, f := range files {
		if strings.HasPrefix(f, "next.config.") {
			if m := basePathRe.FindStringSubmatch(readFile(root, f)); m != nil {
				return strings.TrimSuffix(m[1], "/")
			}
		}
	}
	return ""
}

var astroPages = map[string]bool{".astro": true, ".md": true, ".mdx": true, ".html": true, ".markdown": true}

func extractAstro(root string, files []string, js *jsProject) ([]doc.RefEntry, *Coverage) {
	usesAstro := false
	for _, f := range files {
		if strings.HasPrefix(path.Base(f), "astro.config.") {
			usesAstro = true
		}
	}
	if !usesAstro {
		return nil, nil
	}
	cov := &Coverage{Extractor: "astro", Evidence: []string{"astro.config"}}
	var routes []doc.RefEntry
	for _, f := range files {
		if !strings.HasPrefix(f, "src/pages/") {
			continue
		}
		rel := strings.TrimPrefix(f, "src/pages/")
		if hidden(rel) {
			continue
		}
		ext := path.Ext(rel)
		trimmed := strings.TrimSuffix(rel, ext)
		parts := strings.Split(trimmed, "/")
		if parts[len(parts)-1] == "index" {
			parts = parts[:len(parts)-1]
		}
		route := cleanRoute(strings.Join(parts, "/"))
		switch {
		case astroPages[ext]:
			routes = append(routes, doc.RefEntry{Name: route, Detail: "GET", Handler: "page", Source: f + ":1"})
		case jsts.Grammar(f) != "":
			m := js.load(f)
			methods := exportedMethods(m, "ALL")
			if len(methods) == 0 {
				cov.Unresolved = append(cov.Unresolved, f+": endpoint exports no HTTP method handlers")
				continue
			}
			for i, mth := range methods {
				if mth == "ALL" {
					methods[i] = "ANY"
				}
			}
			routes = append(routes, doc.RefEntry{Name: route, Detail: strings.Join(methods, ", "), Handler: "endpoint", Source: f + ":1"})
		default:
			continue
		}
		cov.Files++
	}
	sort.SliceStable(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
	cov.Entries = len(routes)
	if cov.Entries == 0 {
		cov.Warnings = append(cov.Warnings, "Astro detected but no routes found in src/pages")
	}
	return routes, cov
}

// hidden reports Astro's excluded files: a leading _ on any segment.
func hidden(rel string) bool {
	for _, s := range strings.Split(rel, "/") {
		if strings.HasPrefix(s, "_") {
			return true
		}
	}
	return false
}

// packageDepends reports a dependency in the root package.json.
func packageDepends(root, dep string) bool {
	return strings.Contains(readFile(root, "package.json"), `"`+dep+`"`)
}
