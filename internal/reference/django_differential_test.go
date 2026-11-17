package reference

import (
	"bufio"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/byabasaija/indoc/internal/code"
)

// TestDjangoDifferential compares the static Django extractor with Django's
// own URL resolver running in the project's environment. Opt-in:
//
//	INDOC_DJANGO_REPO=~/projects/shop \
//	INDOC_DJANGO_PYTHON=~/projects/shop/.venv/bin/python \
//	INDOC_DJANGO_ENV='DJANGO_SETTINGS_MODULE=config.settings.test DATABASE_URL=sqlite:///dummy.db' \
//	  go test ./internal/reference -run DjangoDifferential -v
//
// Routes under includes from outside the repository (admin, allauth) are
// excluded from both sides; indoc reports those as external includes.
func TestDjangoDifferential(t *testing.T) {
	repo := os.Getenv("INDOC_DJANGO_REPO")
	if repo == "" {
		t.Skip("set INDOC_DJANGO_REPO to run the Django differential test")
	}
	repo = expandHome(repo)
	python := expandHome(os.Getenv("INDOC_DJANGO_PYTHON"))
	if python == "" {
		python = filepath.Join(repo, ".venv/bin/python")
	}
	script, _ := filepath.Abs("testdata/django_urls.py")
	cmd := exec.Command(python, script)
	cmd.Dir = repo
	cmd.Env = append(os.Environ(), strings.Fields(os.Getenv("INDOC_DJANGO_ENV"))...)
	cmd.Stderr = nil
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("django resolver: %v", err)
	}

	files, err := code.ListFiles(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := Extract(repo, files)
	if err != nil {
		t.Fatal(err)
	}

	ours := map[string][]string{} // path -> methods of each route with that path
	conditional := map[string]bool{}
	var externals []string
	for _, s := range report.Sections {
		if s.Kind != "routes" {
			continue
		}
		for _, e := range s.Entries {
			switch {
			case e.Detail == "INCLUDE":
				externals = append(externals, strings.TrimSuffix(e.Name, "*"))
			case strings.HasPrefix(e.Description, "Conditional"):
				conditional[e.Name] = true
			default:
				ours[e.Name] = append(ours[e.Name], e.Detail)
			}
		}
	}

	truth := map[string][]string{}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		var row struct {
			Path     string   `json:"path"`
			Methods  []string `json:"methods"`
			External bool     `json:"external"`
			Static   bool     `json:"static"`
		}
		if json.Unmarshal(sc.Bytes(), &row) != nil {
			continue
		}
		p := normalizeDjangoPath(row.Path)
		if p == "" || row.External || row.Static || conditional[p] {
			continue
		}
		m := "ANY"
		if row.Methods != nil {
			m = strings.Join(orderedList(row.Methods), ", ")
		}
		truth[p] = append(truth[p], m)
	}

	var missing, extra, wrongMethods, unknownMethods []string
	nTruth, nOurs := 0, 0
	for p, ms := range truth {
		nTruth += len(ms)
		got := append([]string(nil), ours[p]...)
		for _, m := range ms {
			i := indexOf(got, m)
			if i < 0 && m == "ANY" && len(got) > 0 {
				i = 0
			}
			switch {
			case i >= 0:
				got = append(got[:i], got[i+1:]...)
			case len(got) == 0:
				missing = append(missing, p)
			case got[0] == "ANY":
				unknownMethods = append(unknownMethods, p+" (django: "+m+")")
				got = got[1:]
			default:
				wrongMethods = append(wrongMethods, p+" indoc="+got[0]+" django="+m)
				got = got[1:]
			}
		}
		for range got {
			extra = append(extra, p)
		}
	}
	for p, ms := range ours {
		nOurs += len(ms)
		if _, ok := truth[p]; !ok {
			for range ms {
				extra = append(extra, p)
			}
		}
	}
	for _, l := range [][]string{missing, extra, wrongMethods, unknownMethods} {
		sort.Strings(l)
	}
	t.Logf("django routes: %d; indoc routes: %d; external includes: %v", nTruth, nOurs, externals)
	t.Logf("routes: %d matched, %d missing, %d extra; methods: %d wrong, %d unknown",
		nTruth-len(missing), len(missing), len(extra), len(wrongMethods), len(unknownMethods))
	for _, c := range report.Coverage {
		for _, u := range c.Unresolved {
			t.Logf("unresolved (%s): %s", c.Extractor, u)
		}
		for _, w := range c.Warnings {
			t.Logf("warning (%s): %s", c.Extractor, w)
		}
	}
	if len(missing)+len(extra)+len(wrongMethods) > 0 {
		t.Errorf("missing: %v\nextra: %v\nwrong methods: %v", missing, extra, wrongMethods)
	}
	if len(unknownMethods) > 0 {
		t.Logf("methods unknown to indoc: %v", unknownMethods)
	}
}

var (
	drfLookup    = regexp.MustCompile(`\(\?P<([a-zA-Z_]\w*)>\[\^/\.\]\+\)`)
	formatSuffix = regexp.MustCompile(`\\\.\(\?P<format>|<drf_format_suffix:format>`)
)

// normalizeDjangoPath converts DRF router regexes (^users/(?P<pk>[^/.]+)/$)
// to the form indoc emits (users/<pk>/). Format-suffix variants are dropped.
func normalizeDjangoPath(p string) string {
	if formatSuffix.MatchString(p) {
		return ""
	}
	if strings.Contains(p, "^") && strings.Contains(p, "$") && !strings.Contains(p, "(?P<") || drfLookup.MatchString(p) {
		p = drfLookup.ReplaceAllString(p, "<$1>")
		p = strings.ReplaceAll(p, "^", "")
		p = strings.ReplaceAll(p, "$", "")
	}
	return p
}

func orderedList(ms []string) []string {
	set := map[string]bool{}
	for _, m := range ms {
		set[m] = true
	}
	return ordered(set)
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, p[2:])
		}
	}
	return p
}

func indexOf(list []string, s string) int {
	for i, x := range list {
		if x == s {
			return i
		}
	}
	return -1
}
