package python

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// TestDifferentialCPython compares Parse against CPython's ast module over a
// corpus of real code. It is opt-in because it needs Python and a corpus:
//
//	INDOC_PY_CORPUS=~/projects/your-django-app:~/other INDOC_PYTHON=python3.13 \
//	  go test ./internal/lang/python -run Differential -v
//
// Every symbol's qualified name, start line, and end line must agree.
func TestDifferentialCPython(t *testing.T) {
	corpus := os.Getenv("INDOC_PY_CORPUS")
	if corpus == "" {
		t.Skip("set INDOC_PY_CORPUS to run the CPython differential test")
	}
	python := os.Getenv("INDOC_PYTHON")
	if python == "" {
		python = "python3"
	}

	var files []string
	mtimes := map[string]int64{}
	for _, root := range filepath.SplitList(corpus) {
		root = expandHome(root)
		filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == "node_modules" || d.Name() == ".git" || d.Name() == "__pycache__") {
				return filepath.SkipDir
			}
			if !d.IsDir() && strings.HasSuffix(path, ".py") {
				files = append(files, path)
				if info, err := d.Info(); err == nil {
					mtimes[path] = info.ModTime().UnixNano()
				}
			}
			return nil
		})
	}
	if len(files) == 0 {
		t.Fatalf("no .py files under %s", corpus)
	}

	want := cpythonSymbols(t, python, files)

	var agreed, mismatched, skipped, symbols, changed int
	var report []string
	for _, path := range files {
		ref, ok := want[path]
		if !ok || ref == nil {
			skipped++
			continue
		}
		src, err := os.ReadFile(path)
		if err != nil {
			skipped++
			continue
		}
		// CPython read the file earlier; a file edited since then would be a
		// false mismatch.
		if info, err := os.Stat(path); err != nil || info.ModTime().UnixNano() != mtimes[path] {
			changed++
			continue
		}
		ParseModule(string(src)) // must never panic on real code
		got := ourSymbols(string(src))
		symbols += len(ref)
		if diff := diffSymbols(ref, got); diff != "" {
			mismatched++
			if len(report) < 25 {
				report = append(report, path+"\n"+diff)
			}
			continue
		}
		agreed++
	}

	t.Logf("files: %d agreed, %d mismatched, %d skipped (CPython could not parse), %d changed during the run; %d reference symbols",
		agreed, mismatched, skipped, changed, symbols)
	if mismatched > 0 {
		t.Errorf("%d files disagree with CPython; first %d:\n\n%s",
			mismatched, len(report), strings.Join(report, "\n\n"))
	}
}

type refSym struct {
	Qual  string
	Start int
	End   int
}

func cpythonSymbols(t *testing.T, python string, files []string) map[string][]refSym {
	t.Helper()
	script, err := filepath.Abs("testdata/ast_symbols.py")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(python, "-I", script)
	cmd.Stdin = strings.NewReader(strings.Join(files, "\n") + "\n")
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start %s: %v", python, err)
	}
	result := make(map[string][]refSym, len(files))
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 64<<20)
	for sc.Scan() {
		var row struct {
			Path    string  `json:"path"`
			Symbols [][]any `json:"symbols"`
			Error   string  `json:"error"`
		}
		if err := json.Unmarshal(sc.Bytes(), &row); err != nil {
			t.Fatalf("bad reference row: %v", err)
		}
		if row.Error != "" {
			result[row.Path] = nil
			continue
		}
		syms := make([]refSym, 0, len(row.Symbols))
		for _, s := range row.Symbols {
			qual, _ := s[0].(string)
			start, _ := s[1].(float64)
			end, _ := s[2].(float64)
			syms = append(syms, refSym{Qual: qual, Start: int(start), End: int(end)})
		}
		result[row.Path] = syms
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("%s failed: %v", python, err)
	}
	return result
}

func ourSymbols(src string) []refSym {
	syms := Parse(src)
	out := make([]refSym, len(syms))
	for i, s := range syms {
		out[i] = refSym{Qual: s.Qual, Start: s.Line, End: s.EndLine}
	}
	return out
}

func diffSymbols(want, got []refSym) string {
	sortSyms(want)
	sortSyms(got)
	var lines []string
	n := len(want)
	if len(got) > n {
		n = len(got)
	}
	for i := 0; i < n && len(lines) < 6; i++ {
		var w, g *refSym
		if i < len(want) {
			w = &want[i]
		}
		if i < len(got) {
			g = &got[i]
		}
		if w != nil && g != nil && *w == *g {
			continue
		}
		lines = append(lines, fmt.Sprintf("  cpython %s\n  indoc   %s", fmtSym(w), fmtSym(g)))
	}
	return strings.Join(lines, "\n")
}

func sortSyms(s []refSym) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Start != s[j].Start {
			return s[i].Start < s[j].Start
		}
		return s[i].Qual < s[j].Qual
	})
}

func fmtSym(s *refSym) string {
	if s == nil {
		return "(none)"
	}
	return fmt.Sprintf("%s %d-%d", s.Qual, s.Start, s.End)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}
