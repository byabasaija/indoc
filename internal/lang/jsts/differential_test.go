package jsts

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

// TestDifferentialTypeScript compares Parse with the TypeScript compiler API
// over a corpus. Opt-in:
//
//	INDOC_TS_CORPUS=~/projects/app:~/other INDOC_TS_MODULE=/path/to/node_modules/typescript \
//	  go test ./internal/lang/jsts -run Differential -v
func TestDifferentialTypeScript(t *testing.T) {
	corpus, module := os.Getenv("INDOC_TS_CORPUS"), os.Getenv("INDOC_TS_MODULE")
	if corpus == "" || module == "" {
		t.Skip("set INDOC_TS_CORPUS and INDOC_TS_MODULE")
	}
	var files []string
	mtimes := map[string]int64{}
	for _, root := range filepath.SplitList(corpus) {
		filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() && (d.Name() == ".git") {
				return filepath.SkipDir
			}
			info, ierr := d.Info()
			if !d.IsDir() && Grammar(p) != "" && ierr == nil && info.Size() < 1<<20 {
				files = append(files, p)
				mtimes[p] = info.ModTime().UnixNano()
			}
			return nil
		})
	}
	script, _ := filepath.Abs("testdata/ts_symbols.js")
	cmd := exec.Command("node", script, module)
	cmd.Stdin = strings.NewReader(strings.Join(files, "\n") + "\n")
	cmd.Stderr = os.Stderr
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	want := map[string][][3]any{}
	sc := bufio.NewScanner(out)
	sc.Buffer(make([]byte, 1<<20), 256<<20)
	for sc.Scan() {
		var row struct {
			Path    string   `json:"path"`
			Symbols [][3]any `json:"symbols"`
			Error   string   `json:"error"`
		}
		if json.Unmarshal(sc.Bytes(), &row) == nil && row.Error == "" {
			want[row.Path] = row.Symbols
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}

	var agreed, mismatched, skipped, changed, minified, symbols int
	var report []string
	byKind := map[string]int{}
	for _, p := range files {
		ref, ok := want[p]
		if !ok {
			skipped++
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			skipped++
			continue
		}
		if info, err := os.Stat(p); err != nil || info.ModTime().UnixNano() != mtimes[p] {
			changed++
			continue
		}
		// Minified bundles put whole libraries on a few lines; parsers disagree
		// on a handful of ambiguous constructs there, and indoc never documents
		// them (node_modules, dist, and build are not listed). Report, don't fail.
		if longestLine(src) > 20000 {
			minified++
			continue
		}
		f, err := Parse(p, src)
		if err != nil {
			mismatched++
			report = append(report, p+": "+err.Error())
			continue
		}
		var ours, theirs []string
		for _, s := range f.Symbols {
			ours = append(ours, fmt.Sprintf("%s %d-%d", s.Qual, s.Line, s.EndLine))
		}
		for _, s := range ref {
			theirs = append(theirs, fmt.Sprintf("%v %v-%v", s[0], s[1], s[2]))
		}
		symbols += len(theirs)
		sort.Strings(ours)
		sort.Strings(theirs)
		if strings.Join(ours, "\n") == strings.Join(theirs, "\n") {
			agreed++
			continue
		}
		mismatched++
		byKind[Grammar(p)]++
		if len(report) < 30 {
			report = append(report, p+"\n"+diffLines(theirs, ours))
		}
	}
	t.Logf("files: %d agreed, %d mismatched %v, %d skipped, %d minified (not compared), %d changed during the run; %d reference symbols",
		agreed, mismatched, byKind, skipped, minified, changed, symbols)
	if mismatched > 0 {
		t.Errorf("%d files disagree with the TypeScript compiler; first %d:\n\n%s", mismatched, len(report), strings.Join(report, "\n\n"))
	}
}

func diffLines(want, got []string) string {
	w, g := map[string]bool{}, map[string]bool{}
	for _, s := range want {
		w[s] = true
	}
	for _, s := range got {
		g[s] = true
	}
	var lines []string
	for _, s := range want {
		if !g[s] && len(lines) < 8 {
			lines = append(lines, "  typescript "+s)
		}
	}
	for _, s := range got {
		if !w[s] && len(lines) < 16 {
			lines = append(lines, "  indoc      "+s)
		}
	}
	return strings.Join(lines, "\n")
}

func longestLine(src []byte) int {
	longest, cur := 0, 0
	for _, b := range src {
		if b == '\n' {
			cur = 0
			continue
		}
		cur++
		if cur > longest {
			longest = cur
		}
	}
	return longest
}
