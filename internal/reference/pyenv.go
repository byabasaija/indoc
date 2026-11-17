package reference

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/doc"
	"github.com/byabasaija/indoc/internal/lang/python"
)

var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// Defaults are published in the docs site, so values that may be secrets are
// never shown: by variable name, and credentials embedded in URLs.
var (
	secretName    = regexp.MustCompile(`(?i)(SECRET|PASSWORD|PASSWD|PASS$|_PASS_|TOKEN|(^|_)KEY($|_)|APIKEY|PRIVATE|CREDENTIAL|SIGNING|SALT|DSN|AUTH)`)
	urlCredential = regexp.MustCompile(`(://)([^/@\s:"']+):([^/@\s"']+)@`)
)

// redactDefault returns what may be shown for a variable's default value.
func redactDefault(name, value string) string {
	if secretName.MatchString(name) && value != `""` && value != `''` && value != "None" {
		return "(set in code; not shown)"
	}
	return urlCredential.ReplaceAllString(value, "${1}${2}:***@")
}

// envMethods are django-environ readers; the value is the type shown.
var envMethods = map[string]string{
	"str": "str", "bool": "bool", "int": "int", "float": "float", "list": "list",
	"tuple": "tuple", "dict": "dict", "json": "json", "url": "url", "path": "path",
	"decimal": "decimal", "bytes": "bytes", "db": "database URL", "db_url": "database URL",
	"cache": "cache URL", "cache_url": "cache URL", "email": "email URL", "email_url": "email URL",
	"search_url": "search URL", "get_value": "",
}

// envDefaults are the variables django-environ reads when called without a name.
var envDefaults = map[string]string{
	"db": "DATABASE_URL", "db_url": "DATABASE_URL", "cache": "CACHE_URL", "cache_url": "CACHE_URL",
	"email": "EMAIL_URL", "email_url": "EMAIL_URL", "search_url": "SEARCH_URL",
}

// envExamples are committed example files. `.env` itself is never read: it
// holds secrets.
var envExamples = map[string]bool{
	".env.example": true, ".env.sample": true, ".env.template": true, ".env.dist": true,
	"env.example": true, "example.env": true, ".env.local.example": true,
}

type envRead struct {
	entry doc.RefEntry
	file  string
}

func extractPythonEnv(root string, files []SourceFile) ([]doc.RefEntry, *Coverage) {
	cov := &Coverage{Extractor: "python-env"}
	py := newPyProject(root, nil)

	// Names bound to django-environ readers anywhere in the project.
	envObjects := map[string]bool{}
	usesEnviron := false
	var mods []*python.Module
	var paths []string
	for _, f := range files {
		if f.Language != "Python" {
			continue
		}
		m := py.load(f.Path)
		mods = append(mods, m)
		paths = append(paths, f.Path)
		for _, imp := range m.Imports {
			if imp.Module == "environ" || imp.Local == "environ" && imp.Name == "" {
				usesEnviron = true
			}
		}
		for _, a := range m.Assigns {
			if c, ok := a.Value.(*python.Call); ok {
				if fn := c.FuncName(); fn == "environ.Env" || fn == "Env" || fn == "environ.FileAwareEnv" {
					envObjects[a.Target] = true
				}
			}
		}
	}
	if usesEnviron && len(envObjects) == 0 {
		envObjects["env"] = true
	}
	if len(envObjects) > 0 {
		cov.Evidence = append(cov.Evidence, "django-environ")
	}

	reads := map[string]*envRead{}
	var order []string
	for i, m := range mods {
		file := paths[i]
		decouple, osEnviron, osGetenv := "", "", ""
		for _, imp := range m.Imports {
			switch {
			case imp.Module == "decouple" && imp.Name == "config":
				decouple = imp.Local
			case imp.Module == "os" && imp.Name == "environ":
				osEnviron = imp.Local
			case imp.Module == "os" && imp.Name == "getenv":
				osGetenv = imp.Local
			}
		}
		if decouple != "" && !contains(cov.Evidence, "python-decouple") {
			cov.Evidence = append(cov.Evidence, "python-decouple")
		}
		cov.Files++

		type found struct {
			name, detail string
			line         int
		}
		var hits []found
		for _, c := range m.AllCalls {
			if name, detail, ok := envFromCall(c, envObjects, decouple, osEnviron, osGetenv); ok {
				hits = append(hits, found{name, detail, c.Line})
			}
		}
		for _, sub := range m.Subscripts {
			if name, ok := envFromSubscript(sub, osEnviron); ok {
				hits = append(hits, found{name, "required", sub.Line})
			}
		}
		sort.SliceStable(hits, func(a, b int) bool { return hits[a].line < hits[b].line })
		for _, h := range hits {
			if r, seen := reads[h.name]; seen {
				if r.entry.Detail == "" && h.detail != "" {
					r.entry.Detail = h.detail
				}
				continue
			}
			reads[h.name] = &envRead{entry: doc.RefEntry{Name: h.name, Detail: h.detail, Source: fmt.Sprintf("%s:%d", file, h.line)}, file: file}
			order = append(order, h.name)
		}
	}

	for i, m := range mods {
		for _, e := range settingsFields(py, paths[i], m) {
			if _, seen := reads[e.Name]; seen {
				continue
			}
			if !contains(cov.Evidence, "pydantic-settings") {
				cov.Evidence = append(cov.Evidence, "pydantic-settings")
			}
			reads[e.Name] = &envRead{entry: e, file: paths[i]}
			order = append(order, e.Name)
		}
	}

	var out []doc.RefEntry
	for _, name := range order {
		out = append(out, reads[name].entry)
	}
	cov.Entries = len(out)
	if cov.Entries == 0 && len(cov.Evidence) == 0 {
		return nil, nil
	}
	return out, cov
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// envFromCall recognizes an environment read in a call.
func envFromCall(c *python.Call, envObjects map[string]bool, decouple, osEnviron, osGetenv string) (name, detail string, ok bool) {
	fn := c.FuncName()
	if fn == "" {
		return "", "", false
	}
	obj, method, hasMethod := strings.Cut(fn, ".")
	switch {
	case envObjects[fn]:
		return envCall(c, "", false)
	case hasMethod && envObjects[obj] && !strings.Contains(method, "."):
		kind, known := envMethods[method]
		if !known {
			return "", "", false
		}
		if c.Arg(0, "var") == nil {
			if def, ok := envDefaults[method]; ok {
				if d := c.Kw("default"); d != nil {
					return def, kind + ", default " + short(redactDefault(def, d.Text())), true
				}
				return def, kind + ", required", true
			}
		}
		return envCall(c, kind, true)
	case decouple != "" && fn == decouple:
		return envCall(c, "", true)
	case fn == "os.environ.get", fn == "os.getenv",
		osEnviron != "" && fn == osEnviron+".get", osGetenv != "" && fn == osGetenv:
		return optionalCall(c)
	}
	return "", "", false
}

// envFromSubscript recognizes os.environ["X"].
func envFromSubscript(sub python.Subscript, osEnviron string) (string, bool) {
	d, ok := sub.Value.(*python.Dotted)
	if !ok || !(d.Path == "os.environ" || osEnviron != "" && d.Path == osEnviron) {
		return "", false
	}
	s, ok := sub.Key.(*python.Str)
	if !ok || !s.Literal || !envName.MatchString(s.Value) {
		return "", false
	}
	return s.Value, true
}

// envCall reads `env("X", default=...)`-style calls. When positionalDefault
// is set, the second positional argument is the default (env.str, config).
func envCall(c *python.Call, kind string, positionalDefault bool) (string, string, bool) {
	if c == nil {
		return "", "", false
	}
	s, ok := c.Arg(0, "var").(*python.Str)
	if !ok || !s.Literal || !envName.MatchString(s.Value) {
		if s2, ok2 := c.Arg(0, "option").(*python.Str); ok2 && s2.Literal && envName.MatchString(s2.Value) {
			s = s2
		} else {
			return "", "", false
		}
	}
	def := c.Kw("default")
	if def == nil && positionalDefault && len(c.Args) > 1 {
		def = c.Args[1]
	}
	parts := []string{}
	if kind != "" {
		parts = append(parts, kind)
	}
	if def == nil {
		parts = append(parts, "required")
	} else {
		parts = append(parts, "default "+short(redactDefault(s.Value, def.Text())))
	}
	return s.Value, strings.Join(parts, ", "), true
}

func optionalCall(c *python.Call) (string, string, bool) {
	if c == nil || len(c.Args) == 0 {
		return "", "", false
	}
	s, ok := c.Args[0].(*python.Str)
	if !ok || !s.Literal || !envName.MatchString(s.Value) {
		return "", "", false
	}
	if len(c.Args) > 1 {
		return s.Value, "default " + short(redactDefault(s.Value, c.Args[1].Text())), true
	}
	return s.Value, "optional", true
}

// readEnvExample returns the keys of the first committed example env file.
func readEnvExample(root string, all []string) (map[string]bool, string) {
	for _, f := range all {
		if !envExamples[path.Base(f)] {
			continue
		}
		file, err := os.Open(filepath.Join(root, filepath.FromSlash(f)))
		if err != nil {
			continue
		}
		keys := map[string]bool{}
		sc := bufio.NewScanner(file)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			line = strings.TrimPrefix(line, "export ")
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, _, ok := strings.Cut(line, "="); ok && envName.MatchString(strings.TrimSpace(k)) {
				keys[strings.TrimSpace(k)] = true
			}
		}
		file.Close()
		return keys, f
	}
	return nil, ""
}

// settingsFields lists the environment variables a pydantic BaseSettings
// subclass reads: one per annotated field, named env_prefix + FIELD (or the
// field's alias).
func settingsFields(py *pyProject, file string, m *python.Module) []doc.RefEntry {
	var out []doc.RefEntry
	classes := make([]*python.Class, 0, len(m.Classes))
	for _, c := range m.Classes {
		classes = append(classes, c)
	}
	sort.Slice(classes, func(i, j int) bool { return classes[i].Line < classes[j].Line })
	for _, cls := range classes {
		if !isSettingsClass(py, file, cls, 0) {
			continue
		}
		prefix := ""
		for _, a := range cls.Assigns {
			if c, ok := a.Value.(*python.Call); ok && a.Target == "model_config" {
				if s, ok := c.Kw("env_prefix").(*python.Str); ok {
					prefix = s.Value
				}
			}
		}
		for _, a := range cls.Assigns {
			if a.Annotation == nil || strings.HasPrefix(a.Target, "_") || a.Target == "model_config" ||
				strings.HasPrefix(a.Annotation.Text(), "ClassVar") {
				continue
			}
			name := strings.ToUpper(prefix + a.Target)
			def := a.Value
			if c, ok := a.Value.(*python.Call); ok && lastSeg(c.FuncName()) == "Field" {
				for _, kw := range []string{"validation_alias", "alias"} {
					if s, ok := c.Kw(kw).(*python.Str); ok && s.Literal {
						name = s.Value
						break
					}
				}
				def = c.Arg(0, "default")
				if def == nil && c.Kw("default_factory") != nil {
					def = &python.Opaque{Src: "(computed)"}
				}
			}
			detail := a.Annotation.Text()
			if def == nil || def.Text() == "..." {
				detail += ", required"
			} else {
				detail += ", default " + short(redactDefault(name, def.Text()))
			}
			out = append(out, doc.RefEntry{
				Name:    name,
				Detail:  detail,
				Handler: cls.Name + "." + a.Target,
				Source:  fmt.Sprintf("%s:%d", file, a.Line),
			})
		}
	}
	return out
}

func isSettingsClass(py *pyProject, file string, cls *python.Class, depth int) bool {
	if depth > 5 {
		return false
	}
	for _, b := range cls.Bases {
		n, ok := b.(*python.Dotted)
		if !ok {
			continue
		}
		if lastSeg(n.Path) == "BaseSettings" {
			return true
		}
		if r := py.lookup(file, n.Path); r.kind == "class" {
			if isSettingsClass(py, r.file, py.load(r.file).Classes[r.name], depth+1) {
				return true
			}
		}
	}
	return false
}
