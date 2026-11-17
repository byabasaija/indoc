// Package code lists a repository's files and analyzes them with whichever
// language parser applies.
package code

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/byabasaija/indoc/internal/lang"
	"github.com/byabasaija/indoc/internal/lang/golang"
	"github.com/byabasaija/indoc/internal/lang/python"
)

// File is the analysis of one source file.
type File struct {
	// Parsed is true when a language parser understood the file. Unparsed
	// files only have Body (a hash of their bytes).
	Parsed  bool
	Symbols []lang.Symbol
	// Body hashes the whole file with comments and layout removed (or raw
	// bytes when unparsed).
	Body string
}

// Parsable reports whether a parser exists for path.
func Parsable(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py", ".go":
		return true
	}
	return false
}

// Analyze parses src according to path's extension.
func Analyze(path string, src []byte) File {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".py":
		s := string(src)
		return File{Parsed: true, Symbols: python.Parse(s), Body: python.FileHash(s)}
	case ".go":
		return File{Parsed: true, Symbols: golang.Parse(src), Body: golang.FileHash(src)}
	}
	sum := sha256.Sum256(src)
	return File{Body: hex.EncodeToString(sum[:])[:16]}
}

// Surfaces maps each symbol's qualified name to its surface hash. Repeated
// names (a property getter and setter, if/else definitions) get "#2", "#3".
func (f File) Surfaces() map[string]string {
	out := make(map[string]string, len(f.Symbols))
	for _, s := range f.Symbols {
		key := s.Qual
		for i := 2; ; i++ {
			if _, taken := out[key]; !taken {
				break
			}
			key = fmt.Sprintf("%s#%d", s.Qual, i)
		}
		out[key] = s.Surface
	}
	return out
}

// Lookup finds a symbol by qualified name.
func (f File) Lookup(qual string) (lang.Symbol, bool) {
	for _, s := range f.Symbols {
		if s.Qual == qual {
			return s, true
		}
	}
	return lang.Symbol{}, false
}

// skipDirs are never documentation sources.
var skipDirs = map[string]bool{
	"node_modules": true, "__pycache__": true, "vendor": true,
	"dist": true, "build": true, "site-packages": true,
}

// ListFiles returns repository files under root as slash-separated relative
// paths, sorted. Hidden directories, dependency folders, virtualenvs, and
// names in exclude are skipped.
func ListFiles(root string, exclude []string) ([]string, error) {
	excluded := map[string]bool{}
	for _, e := range exclude {
		if e = strings.TrimSpace(e); e != "" {
			excluded[e] = true
		}
	}
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if p == root {
				return nil
			}
			if strings.HasPrefix(name, ".") || skipDirs[name] || excluded[name] || isVirtualenv(p) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() || excluded[name] {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list files under %s: %w", root, err)
	}
	sort.Strings(files)
	return files, nil
}

func isVirtualenv(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "pyvenv.cfg"))
	return err == nil
}

// IsTest reports whether a path is a test file by common conventions:
// tests/test/__tests__ directories, test_*.py, *_test.py, conftest.py,
// *_test.go, and *.test.* / *.spec.* files.
func IsTest(p string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(filepath.Dir(p)), "/") {
		if seg == "tests" || seg == "test" || seg == "__tests__" {
			return true
		}
	}
	base := strings.ToLower(filepath.Base(p))
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasSuffix(base, ".py") && (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py") || base == "conftest.py"):
		return true
	}
	return strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")
}
