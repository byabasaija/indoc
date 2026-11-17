package reference

import (
	"path"
	"strings"

	"github.com/byabasaija/indoc/internal/code"
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

func isTest(p string) bool { return code.IsTest(p) }
