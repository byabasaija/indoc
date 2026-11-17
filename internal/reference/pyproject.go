package reference

import (
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/lang/python"
)

// pyProject resolves Python modules and names within a repository, statically.
type pyProject struct {
	root  string
	files map[string]bool
	roots []string // source roots, relative ("" is the repository root)
	mods  map[string]*python.Module
}

func newPyProject(root string, files []string) *pyProject {
	p := &pyProject{root: root, files: map[string]bool{}, mods: map[string]*python.Module{}}
	rootSet := map[string]bool{"": true}
	for _, f := range files {
		p.files[f] = true
		if path.Base(f) == "manage.py" {
			rootSet[dirOf(f)] = true
		}
		if strings.HasPrefix(f, "src/") {
			rootSet["src"] = true
		}
	}
	for r := range rootSet {
		p.roots = append(p.roots, r)
	}
	// Deeper roots first: a manage.py directory is more specific than the repo root.
	sort.Slice(p.roots, func(i, j int) bool { return len(p.roots[i]) > len(p.roots[j]) })
	return p
}

func dirOf(f string) string {
	d := path.Dir(f)
	if d == "." {
		return ""
	}
	return d
}

func join(root, rel string) string {
	if root == "" {
		return rel
	}
	return root + "/" + rel
}

// moduleFile returns the repository file for a dotted module name.
func (p *pyProject) moduleFile(dotted string) (string, bool) {
	if dotted == "" {
		return "", false
	}
	rel := strings.ReplaceAll(dotted, ".", "/")
	for _, r := range p.roots {
		for _, cand := range []string{join(r, rel+".py"), join(r, rel+"/__init__.py")} {
			if p.files[cand] {
				return cand, true
			}
		}
	}
	return "", false
}

// moduleName returns the dotted module name of a repository file.
func (p *pyProject) moduleName(file string) string {
	for _, r := range p.roots {
		if r != "" && !strings.HasPrefix(file, r+"/") {
			continue
		}
		rel := strings.TrimPrefix(file, r+"/")
		if r == "" {
			rel = file
		}
		rel = strings.TrimSuffix(rel, ".py")
		rel = strings.TrimSuffix(rel, "/__init__")
		return strings.ReplaceAll(rel, "/", ".")
	}
	return ""
}

func (p *pyProject) load(file string) *python.Module {
	if m, ok := p.mods[file]; ok {
		return m
	}
	var m *python.Module
	if src, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(file))); err == nil && len(src) <= maxFileBytes {
		m = python.ParseModule(string(src))
	} else {
		m = python.ParseModule("")
	}
	p.mods[file] = m
	return m
}

// importTarget returns the absolute dotted target of an import in file.
func (p *pyProject) importTarget(file string, imp python.Import) string {
	base := imp.Module
	if imp.Level > 0 {
		pkg := strings.Split(p.moduleName(file), ".")
		if !strings.HasSuffix(file, "/__init__.py") && len(pkg) > 0 {
			pkg = pkg[:len(pkg)-1]
		}
		up := imp.Level - 1
		if up > len(pkg) {
			up = len(pkg)
		}
		pkg = pkg[:len(pkg)-up]
		base = strings.Join(append(pkg, nonEmpty(imp.Module)...), ".")
	}
	if imp.Name != "" {
		return strings.Trim(base+"."+imp.Name, ".")
	}
	return base
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}

// ref is what a name resolves to.
type ref struct {
	kind     string // "module", "class", "func", "assign", "external", "unknown"
	file     string
	name     string // class, function, or variable name within file
	external string // dotted name outside the repository
}

// lookup resolves a dotted name as seen from file.
func (p *pyProject) lookup(file, dotted string) ref {
	return p.lookupDepth(file, dotted, 0)
}

func (p *pyProject) lookupDepth(file, dotted string, depth int) ref {
	if depth > 8 || dotted == "" {
		return ref{kind: "unknown"}
	}
	parts := strings.Split(dotted, ".")
	m := p.load(file)
	head := parts[0]
	if len(parts) == 1 {
		if _, ok := m.Classes[head]; ok {
			return ref{kind: "class", file: file, name: head}
		}
		if _, ok := m.Funcs[head]; ok {
			return ref{kind: "func", file: file, name: head}
		}
	}
	for i := len(m.Imports) - 1; i >= 0; i-- {
		imp := m.Imports[i]
		if imp.Local != head {
			continue
		}
		abs := p.importTarget(file, imp)
		if len(parts) > 1 {
			abs += "." + strings.Join(parts[1:], ".")
		}
		return p.resolveAbsolute(abs, depth+1)
	}
	if len(parts) == 1 {
		for _, a := range m.Assigns {
			if a.Target == head {
				return ref{kind: "assign", file: file, name: head}
			}
		}
	}
	return ref{kind: "unknown"}
}

// resolveAbsolute resolves a fully qualified dotted name.
func (p *pyProject) resolveAbsolute(abs string, depth int) ref {
	parts := strings.Split(abs, ".")
	for i := len(parts); i > 0; i-- {
		file, ok := p.moduleFile(strings.Join(parts[:i], "."))
		if !ok {
			continue
		}
		rest := parts[i:]
		switch len(rest) {
		case 0:
			return ref{kind: "module", file: file}
		default:
			r := p.lookupDepth(file, strings.Join(rest, "."), depth+1)
			if r.kind == "unknown" {
				return ref{kind: "unknown", external: abs}
			}
			return r
		}
	}
	return ref{kind: "external", external: abs}
}
