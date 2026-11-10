package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/byabasaija/indoc/internal/doc"
)

const (
	// DefaultCacheDir is the standard directory where indoc stores local cache and metadata.
	DefaultCacheDir = ".indoc"

	// DefaultCacheFile is the path to the cache manifest.
	DefaultCacheFile = ".indoc/cache.json"

	// CacheSchemaVersion defines the version of the cache format.
	CacheSchemaVersion = "1"
)

// FileMeta captures the state of a single source file at scan time.
type FileMeta struct {
	Path     string `json:"path"`
	Hash     string `json:"hash"`
	Language string `json:"language"`
	Size     int64  `json:"size"`
}

// CacheManifest represents the saved cache manifest in .indoc/cache.json.
// Document is the last bundled narrative so the next scan can hand it back for editing.
type CacheManifest struct {
	Version   string              `json:"version"`
	LastScan  string              `json:"last_scan"`
	FileCache map[string]FileMeta `json:"file_cache"`
	Document  *doc.Document       `json:"document,omitempty"`
}

// DiffResult categorizes files based on comparison between current disk state and cache.
type DiffResult struct {
	Added     []FileMeta
	Modified  []FileMeta
	Unchanged []FileMeta
	Deleted   []string
}

// HasChanges returns true if any files were added, modified, or deleted.
func (d *DiffResult) HasChanges() bool {
	return len(d.Added) > 0 || len(d.Modified) > 0 || len(d.Deleted) > 0
}

// NewCacheManifest creates an empty initialized CacheManifest.
func NewCacheManifest() *CacheManifest {
	return &CacheManifest{
		Version:   CacheSchemaVersion,
		LastScan:  time.Now().UTC().Format(time.RFC3339),
		FileCache: make(map[string]FileMeta),
	}
}

// LoadCache reads .indoc/cache.json from disk.
// If the file does not exist, an empty initialized CacheManifest is returned without error.
func LoadCache(cachePath string) (*CacheManifest, error) {
	if cachePath == "" {
		cachePath = DefaultCacheFile
	}

	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return NewCacheManifest(), nil
		}
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	manifest := NewCacheManifest()
	if err := json.Unmarshal(data, manifest); err != nil {
		// If cache is corrupted, return a fresh manifest rather than blocking the build
		return NewCacheManifest(), nil
	}

	if manifest.FileCache == nil {
		manifest.FileCache = make(map[string]FileMeta)
	}

	return manifest, nil
}

// SaveCache writes the CacheManifest to disk as formatted JSON.
func SaveCache(cachePath string, manifest *CacheManifest) error {
	if cachePath == "" {
		cachePath = DefaultCacheFile
	}

	manifest.LastScan = time.Now().UTC().Format(time.RFC3339)
	manifest.Version = CacheSchemaVersion

	dir := filepath.Dir(cachePath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create cache directory: %w", err)
		}
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to serialize cache manifest: %w", err)
	}

	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// ComputeDiff compares current scanned files against the previous cache manifest.
func ComputeDiff(cached *CacheManifest, current map[string]FileMeta) *DiffResult {
	diff := &DiffResult{
		Added:     make([]FileMeta, 0),
		Modified:  make([]FileMeta, 0),
		Unchanged: make([]FileMeta, 0),
		Deleted:   make([]string, 0),
	}

	if cached == nil || cached.FileCache == nil {
		for _, meta := range current {
			diff.Added = append(diff.Added, meta)
		}
		return diff
	}

	// 1. Check current files against cached state
	for path, currentMeta := range current {
		cachedMeta, exists := cached.FileCache[path]
		if !exists {
			diff.Added = append(diff.Added, currentMeta)
		} else if cachedMeta.Hash != currentMeta.Hash {
			diff.Modified = append(diff.Modified, currentMeta)
		} else {
			diff.Unchanged = append(diff.Unchanged, currentMeta)
		}
	}

	// 2. Check for deleted files (in cache, but missing from current scan)
	for cachedPath := range cached.FileCache {
		if _, exists := current[cachedPath]; !exists {
			diff.Deleted = append(diff.Deleted, cachedPath)
		}
	}

	return diff
}
