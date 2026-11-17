package reference

import (
	"path"
	"strings"
)

var languages = map[string]string{
	".go": "Go", ".py": "Python",
	".js": "JavaScript", ".jsx": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript",
}

// Sources selects the files reference extraction reads: languages it has
// patterns for, excluding tests (routes and env reads in tests are noise).
func Sources(files []string) []SourceFile {
	var out []SourceFile
	for _, f := range files {
		lang, ok := languages[strings.ToLower(path.Ext(f))]
		if !ok || isTest(f) {
			continue
		}
		out = append(out, SourceFile{Path: f, Language: lang})
	}
	return out
}

func isTest(p string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if seg == "tests" || seg == "test" || seg == "__tests__" {
			return true
		}
	}
	base := strings.ToLower(path.Base(p))
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || base == "conftest.py"):
		return true
	}
	for _, s := range []string{".test.", ".spec."} {
		if strings.Contains(base, s) {
			return true
		}
	}
	return false
}
