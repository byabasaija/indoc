package scanner

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ScanResult holds the outcome of a codebase scan including discovered files and computed diffs.
type ScanResult struct {
	Files      map[string]FileMeta
	Diff       *DiffResult
	TotalFiles int
}

// Scanner orchestrates traversing source directories, filtering files, and computing hashes.
type Scanner struct {
	sourceDirs   []string
	excludes     map[string]bool
	includeTests bool
	cachePath    string
}

// NewScanner constructs a Scanner with normalized exclusion sets.
func NewScanner(sourceDirs []string, excludes []string, includeTests bool, cachePath string) *Scanner {
	excludeMap := make(map[string]bool)
	for _, exc := range excludes {
		clean := strings.TrimSpace(strings.ToLower(exc))
		if clean != "" {
			excludeMap[clean] = true
		}
	}

	// Always exclude internal indoc cache and standard VCS folders
	excludeMap[".git"] = true
	excludeMap[".indoc"] = true

	return &Scanner{
		sourceDirs:   sourceDirs,
		excludes:     excludeMap,
		includeTests: includeTests,
		cachePath:    cachePath,
	}
}

// Scan walks all configured source directories, computes SHA-256 hashes,
// and diffs the result against the cache.
func (s *Scanner) Scan() (*ScanResult, error) {
	currentFiles := make(map[string]FileMeta)

	for _, srcDir := range s.sourceDirs {
		info, err := os.Stat(srcDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("source directory '%s' does not exist", srcDir)
			}
			return nil, fmt.Errorf("failed to access source directory '%s': %w", srcDir, err)
		}

		if !info.IsDir() {
			// Single file scan
			if err := s.processFile(srcDir, currentFiles); err != nil {
				return nil, err
			}
			continue
		}

		// Directory walk
		err = filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			name := d.Name()

			// Check directory exclusion
			if d.IsDir() {
				// Skip hidden directories (starting with ".")
				if strings.HasPrefix(name, ".") && path != srcDir && path != "." {
					return filepath.SkipDir
				}
				// Skip excluded directory names
				if s.excludes[strings.ToLower(name)] {
					return filepath.SkipDir
				}
				return nil
			}

			// Skip hidden files
			if strings.HasPrefix(name, ".") {
				return nil
			}

			// Skip test files if not explicitly included
			if !s.includeTests && isTestFile(path) {
				return nil
			}

			// Only process recognized source code files
			if !IsSupportedSourceFile(path) {
				return nil
			}

			return s.processFile(path, currentFiles)
		})

		if err != nil {
			return nil, fmt.Errorf("error scanning directory '%s': %w", srcDir, err)
		}
	}

	// Load previous cache and compute diff
	cached, err := LoadCache(s.cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to load cache: %w", err)
	}

	diff := ComputeDiff(cached, currentFiles)

	return &ScanResult{
		Files:      currentFiles,
		Diff:       diff,
		TotalFiles: len(currentFiles),
	}, nil
}

// isTestFile returns true if a file matches standard testing patterns across languages.
func isTestFile(filePath string) bool {
	base := strings.ToLower(filepath.Base(filePath))

	// Go
	if strings.HasSuffix(base, "_test.go") {
		return true
	}
	// Python
	if (strings.HasPrefix(base, "test_") || strings.HasSuffix(base, "_test.py")) && strings.HasSuffix(base, ".py") {
		return true
	}
	// JavaScript / TypeScript
	if strings.HasSuffix(base, ".test.js") || strings.HasSuffix(base, ".spec.js") ||
		strings.HasSuffix(base, ".test.ts") || strings.HasSuffix(base, ".spec.ts") ||
		strings.HasSuffix(base, ".test.jsx") || strings.HasSuffix(base, ".spec.jsx") ||
		strings.HasSuffix(base, ".test.tsx") || strings.HasSuffix(base, ".spec.tsx") {
		return true
	}
	// Rust
	if strings.HasSuffix(base, "_test.rs") {
		return true
	}

	return false
}

// processFile extracts metadata, hashes content, and registers the file.
func (s *Scanner) processFile(filePath string, registry map[string]FileMeta) error {
	lang, ok := DetectLanguage(filePath)
	if !ok {
		return nil
	}

	info, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to inspect file '%s': %w", filePath, err)
	}

	hash, err := ComputeFileSHA256(filePath)
	if err != nil {
		return fmt.Errorf("failed to hash file '%s': %w", filePath, err)
	}

	// Normalize path to slash for cross-platform consistency (Windows vs Unix)
	cleanPath := filepath.ToSlash(filePath)

	registry[cleanPath] = FileMeta{
		Path:     cleanPath,
		Hash:     hash,
		Language: lang,
		Size:     info.Size(),
	}

	return nil
}
