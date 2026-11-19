// Package projectmap summarizes a repository for the agent writing its docs:
// languages, frameworks, entry points, task runner targets, existing docs,
// files that change most, and the modules everything else imports.
//
// Everything here is a hint for choosing what to document. Nothing is
// published.
package projectmap

import (
	"bufio"
	"encoding/json"
	"image"
	_ "image/jpeg" // logo sizes
	_ "image/png"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
	"github.com/byabasaija/indoc/internal/lang/python"
)

// Map is the summary.
type Map struct {
	Languages   []Language `json:"languages"`
	Frameworks  []string   `json:"frameworks"`
	EntryPoints []string   `json:"entry_points"`
	Tasks       []Task     `json:"tasks"`
	Docs        []string   `json:"docs"`      // Markdown not selected by pages:
	HotFiles    []Count    `json:"hot_files"` // most commits in the last 90 days
	Central     []Count    `json:"central"`   // most-imported in-repo Python modules
	Logos       []Logo     `json:"logos"`     // images that look like the project's logo, best first
}

// Logo is an image file that may be the project's logo. Width and Height are
// pixels (0 for SVG and formats indoc does not measure); a square mark suits
// the site header and favicon better than a tall lockup with text.
type Logo struct {
	Path   string `json:"path"`
	Width  int    `json:"width,omitempty"`
	Height int    `json:"height,omitempty"`
}

// Language is a file count per language.
type Language struct {
	Name   string `json:"name"`
	Files  int    `json:"files"`
	Parsed bool   `json:"parsed"` // indoc has a parser: citations and precise staleness
}

// Task is a command a task runner defines.
type Task struct {
	Runner  string `json:"runner"` // just, make, npm, python
	Name    string `json:"name"`
	Command string `json:"command,omitempty"` // how to run it
	Comment string `json:"comment,omitempty"`
}

// Count is a path with a count.
type Count struct {
	Path  string `json:"path"`
	Count int    `json:"count"`
}

var extLanguages = map[string]string{
	".py": "Python", ".go": "Go", ".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".rs": "Rust", ".java": "Java", ".kt": "Kotlin",
	".rb": "Ruby", ".php": "PHP", ".cs": "C#", ".swift": "Swift", ".dart": "Dart", ".c": "C",
	".h": "C", ".cpp": "C++", ".scala": "Scala", ".ex": "Elixir", ".vue": "Vue", ".svelte": "Svelte",
	".html": "HTML", ".sql": "SQL",
}

// Build summarizes the repository. pageFiles are the files already published.
func Build(root string, files, pageFiles []string) *Map {
	m := &Map{Languages: []Language{}, Frameworks: []string{}, EntryPoints: []string{}, Tasks: []Task{}, Docs: []string{}, HotFiles: []Count{}, Central: []Count{}, Logos: []Logo{}}
	published := map[string]bool{}
	for _, f := range pageFiles {
		published[f] = true
	}
	langs := map[string]*Language{}
	fileSet := map[string]bool{}
	for _, f := range files {
		fileSet[f] = true
		ext := strings.ToLower(path.Ext(f))
		if name, ok := extLanguages[ext]; ok {
			l := langs[name]
			if l == nil {
				l = &Language{Name: name, Parsed: code.Parsable(f)}
				langs[name] = l
			}
			l.Files++
		}
		if ext == ".md" && !published[f] && !isBoilerplateDoc(f) {
			m.Docs = append(m.Docs, f)
		}
	}
	for _, l := range langs {
		m.Languages = append(m.Languages, *l)
	}
	sort.Slice(m.Languages, func(i, j int) bool {
		if m.Languages[i].Files != m.Languages[j].Files {
			return m.Languages[i].Files > m.Languages[j].Files
		}
		return m.Languages[i].Name < m.Languages[j].Name
	})

	m.Frameworks = frameworks(root, fileSet)
	m.EntryPoints = entryPoints(root, files)
	m.Tasks = tasks(root, fileSet)
	m.HotFiles = hotFiles(root, fileSet)
	m.Central = centralModules(root, files)
	m.Logos = logos(root, files)
	return m
}

// skipLogoDir reports images in test data, third-party code, or indoc's own
// output, judged by whole folder names.
func skipLogoDir(f string) bool {
	dirs := strings.Split(strings.ToLower(path.Dir(f)), "/")
	for _, d := range dirs {
		switch d {
		case "test", "tests", "testdata", "fixtures", "__tests__", "vendor", "site-packages", "node_modules", ".indoc":
			return true
		}
	}
	return false
}

var logoExts = map[string]int{".svg": 0, ".png": 1, ".webp": 2, ".jpg": 3, ".jpeg": 3, ".ico": 4}

// logos finds images named like a logo, ranked by name (logo, then brand or
// mark, then favicon or icon), format (SVG first), and depth. Test fixtures
// and third-party folders are skipped.
func logos(root string, files []string) []Logo {
	type cand struct {
		path             string
		name, ext, depth int
	}
	var cands []cand
	for _, f := range files {
		ext, ok := logoExts[strings.ToLower(path.Ext(f))]
		if !ok {
			continue
		}
		if skipLogoDir(f) {
			continue
		}
		base := strings.ToLower(path.Base(f))
		name := -1
		switch {
		case strings.Contains(base, "logo"):
			name = 0
		case strings.Contains(base, "brand") || strings.Contains(base, "wordmark"):
			name = 1
		case strings.Contains(base, "favicon") || strings.HasPrefix(base, "icon") || strings.Contains(base, "apple-touch-icon"):
			name = 2
		}
		if name < 0 {
			continue
		}
		cands = append(cands, cand{f, name, ext, strings.Count(f, "/")})
	}
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.name != b.name {
			return a.name < b.name
		}
		if a.ext != b.ext {
			return a.ext < b.ext
		}
		if a.depth != b.depth {
			return a.depth < b.depth
		}
		return a.path < b.path
	})
	out := []Logo{}
	for _, c := range cands {
		if len(out) == 8 {
			break
		}
		l := Logo{Path: c.path}
		if fh, err := os.Open(filepath.Join(root, filepath.FromSlash(c.path))); err == nil {
			if cfg, _, err := image.DecodeConfig(fh); err == nil {
				l.Width, l.Height = cfg.Width, cfg.Height
			}
			fh.Close()
		}
		out = append(out, l)
	}
	return out
}

func isBoilerplateDoc(f string) bool {
	switch strings.ToUpper(strings.TrimSuffix(path.Base(f), path.Ext(f))) {
	case "LICENSE", "CHANGELOG", "CODE_OF_CONDUCT", "SECURITY", "PULL_REQUEST_TEMPLATE":
		return true
	}
	return strings.HasPrefix(f, ".github/")
}

func read(root, f string) string {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
	if err != nil {
		return ""
	}
	return string(data)
}

// dependencyMarkers maps a dependency name to the framework shown.
var dependencyMarkers = []struct{ dep, name string }{
	{"djangorestframework", "Django REST framework"}, {"django", "Django"}, {"fastapi", "FastAPI"},
	{"flask", "Flask"}, {"celery", "Celery"}, {"pydantic", "Pydantic"}, {"click", "Click"}, {"typer", "Typer"},
	{"next", "Next.js"}, {"express", "Express"}, {"@nestjs/core", "NestJS"}, {"react", "React"},
	{"vue", "Vue"}, {"svelte", "Svelte"}, {"astro", "Astro"},
	{"github.com/spf13/cobra", "Cobra"}, {"github.com/gin-gonic/gin", "Gin"}, {"github.com/labstack/echo", "Echo"},
	{"github.com/go-chi/chi", "chi"}, {"github.com/gofiber/fiber", "Fiber"},
}

var depToken = regexp.MustCompile(`[A-Za-z0-9_.@/-]+`)

// Frameworks lists the frameworks a repository uses, most specific first.
func Frameworks(root string, files []string) []string {
	set := map[string]bool{}
	for _, f := range files {
		set[f] = true
	}
	return frameworks(root, set)
}

func frameworks(root string, files map[string]bool) []string {
	deps := map[string]bool{}
	for _, f := range []string{"pyproject.toml", "requirements.txt", "requirements/base.txt", "go.mod", "Pipfile", "setup.py"} {
		if files[f] {
			for _, tok := range depToken.FindAllString(strings.ToLower(read(root, f)), -1) {
				deps[strings.Trim(tok, `"'`)] = true
			}
		}
	}
	if files["package.json"] {
		var pkg struct {
			Deps    map[string]string `json:"dependencies"`
			DevDeps map[string]string `json:"devDependencies"`
		}
		if json.Unmarshal([]byte(read(root, "package.json")), &pkg) == nil {
			for d := range pkg.Deps {
				deps[d] = true
			}
			for d := range pkg.DevDeps {
				deps[d] = true
			}
		}
	}
	var out []string
	for _, mk := range dependencyMarkers {
		if deps[mk.dep] || matchesModule(deps, mk.dep) {
			out = append(out, mk.name)
		}
	}
	if files["manage.py"] && !contains(out, "Django") {
		out = append(out, "Django")
	}
	return out
}

// matchesModule matches Go module paths with version suffixes (…/echo/v4).
func matchesModule(deps map[string]bool, mod string) bool {
	if !strings.Contains(mod, "/") {
		return false
	}
	for d := range deps {
		if strings.HasPrefix(d, mod+"/v") {
			return true
		}
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func entryPoints(root string, files []string) []string {
	out := []string{}
	for _, f := range files {
		base := path.Base(f)
		switch {
		case base == "manage.py", base == "wsgi.py", base == "asgi.py", base == "__main__.py":
			out = append(out, f)
		case base == "main.go" && strings.Contains(read(root, f), "func main()"):
			out = append(out, f)
		case base == "main.py" && strings.Count(f, "/") <= 2:
			out = append(out, f)
		}
	}
	if pj := read(root, "package.json"); pj != "" {
		var pkg struct {
			Main string `json:"main"`
			Bin  any    `json:"bin"`
		}
		if json.Unmarshal([]byte(pj), &pkg) == nil {
			if pkg.Main != "" {
				out = append(out, "package.json main: "+pkg.Main)
			}
			if pkg.Bin != nil {
				out = append(out, "package.json bin")
			}
		}
	}
	return out
}

var (
	justRecipe = regexp.MustCompile(`^@?([A-Za-z_][\w-]*)(\s+[^:=]*)?:([^=]|$)`)
	makeTarget = regexp.MustCompile(`^([A-Za-z0-9][\w.-]*)\s*:([^=]|$)`)
	tomlKV     = regexp.MustCompile(`^\s*"?([\w.-]+)"?\s*=\s*"([^"]*)"`)
)

func tasks(root string, files map[string]bool) []Task {
	out := []Task{}
	for _, name := range []string{"justfile", "Justfile", ".justfile"} {
		if files[name] {
			out = append(out, lineTasks(read(root, name), "just", justRecipe)...)
			break
		}
	}
	for _, name := range []string{"Makefile", "makefile", "GNUmakefile"} {
		if files[name] {
			out = append(out, lineTasks(read(root, name), "make", makeTarget)...)
			break
		}
	}
	if pj := read(root, "package.json"); pj != "" {
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if json.Unmarshal([]byte(pj), &pkg) == nil {
			var names []string
			for n := range pkg.Scripts {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				out = append(out, Task{Runner: "npm", Name: n, Command: "npm run " + n, Comment: pkg.Scripts[n]})
			}
		}
	}
	if pp := read(root, "pyproject.toml"); pp != "" {
		table := ""
		sc := bufio.NewScanner(strings.NewReader(pp))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "[") {
				table = line
				continue
			}
			if table == "[project.scripts]" || table == "[tool.poetry.scripts]" {
				if m := tomlKV.FindStringSubmatch(line); m != nil {
					out = append(out, Task{Runner: "python", Name: m[1], Command: m[1], Comment: m[2]})
				}
			}
		}
	}
	return out
}

// lineTasks reads recipe/target names, with the comment line above as description.
func lineTasks(src, runner string, re *regexp.Regexp) []Task {
	var out []Task
	seen := map[string]bool{}
	comment := ""
	for _, line := range strings.Split(src, "\n") {
		if strings.HasPrefix(line, "#") {
			comment = strings.TrimSpace(strings.TrimLeft(line, "#"))
			continue
		}
		if strings.TrimSpace(line) == "" || line[0] == ' ' || line[0] == '\t' {
			if strings.TrimSpace(line) == "" {
				comment = ""
			}
			continue
		}
		m := re.FindStringSubmatch(line)
		if m == nil || seen[m[1]] || strings.HasPrefix(m[1], ".") || strings.Contains(m[1], "%") {
			comment = ""
			continue
		}
		seen[m[1]] = true
		out = append(out, Task{Runner: runner, Name: m[1], Command: runner + " " + m[1], Comment: comment})
		comment = ""
	}
	return out
}

// hotFiles counts commits per file over the last 90 days. Without git, it is empty.
func hotFiles(root string, files map[string]bool) []Count {
	cmd := exec.Command("git", "-C", root, "log", "--since=90.days", "--name-only", "--format=", "--no-merges")
	out, err := cmd.Output()
	if err != nil {
		return []Count{}
	}
	counts := map[string]int{}
	for _, f := range strings.Split(string(out), "\n") {
		f = strings.TrimSpace(f)
		if files[f] && !strings.HasSuffix(f, ".md") && !strings.Contains(f, "migrations/") && !strings.HasSuffix(f, ".lock") {
			counts[f]++
		}
	}
	return top(counts, 15)
}

// centralModules counts, for each in-repo Python module, how many files import it.
func centralModules(root string, files []string) []Count {
	modFile := map[string]string{}
	var pyFiles []string
	for _, f := range files {
		if !strings.HasSuffix(f, ".py") || strings.Contains(f, "/tests/") || strings.Contains(f, "migrations/") {
			continue
		}
		pyFiles = append(pyFiles, f)
		mod := strings.TrimSuffix(strings.TrimSuffix(f, ".py"), "/__init__")
		modFile[strings.ReplaceAll(mod, "/", ".")] = f
	}
	if len(pyFiles) == 0 {
		return []Count{}
	}
	counts := map[string]int{}
	for _, f := range pyFiles {
		src := read(root, f)
		if len(src) > 1<<20 {
			continue
		}
		seen := map[string]bool{}
		for _, imp := range python.ParseModule(src).Imports {
			target := imp.Module
			if imp.Level > 0 {
				pkg := strings.Split(strings.ReplaceAll(strings.TrimSuffix(f, ".py"), "/", "."), ".")
				up := imp.Level
				if up > len(pkg) {
					up = len(pkg)
				}
				target = strings.Trim(strings.Join(pkg[:len(pkg)-up], ".")+"."+imp.Module, ".")
			}
			for _, cand := range []string{target + "." + imp.Name, target} {
				if dst, ok := modFile[cand]; ok && dst != f && !seen[dst] {
					seen[dst] = true
					counts[dst]++
					break
				}
			}
		}
	}
	return top(counts, 10)
}

func top(counts map[string]int, n int) []Count {
	out := make([]Count, 0, len(counts))
	for p, c := range counts {
		out = append(out, Count{Path: p, Count: c})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Path < out[j].Path
	})
	if len(out) > n {
		out = out[:n]
	}
	return out
}
