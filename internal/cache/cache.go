// Package cache implements the v0.25 incremental cache for the Digestron indexer.
//
// The cache is stored in <repoRoot>/.digestron/cache.v0.25.json.
// It records per-file metadata (mtime, size) alongside the last known USG so
// that a subsequent index run can skip re-extraction when nothing changed.
package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const cacheFile = "cache.v0.25.json"

// FileEntry records the last-known state of a single source file.
type FileEntry struct {
	Path      string   `json:"path"`
	MtimeUnix int64    `json:"mtimeUnix"`
	Size      int64    `json:"size"`
	ModuleID  string   `json:"moduleId,omitempty"`
	SymbolIDs []string `json:"symbolIds,omitempty"`
	EdgeCount int      `json:"edgeCount,omitempty"`
}

// Cache is the top-level cache document saved per repository.
type Cache struct {
	RepoRoot       string      `json:"repoRoot"`
	Engine         string      `json:"engine,omitempty"`
	TsconfigsUsed  []string    `json:"tsconfigsUsed,omitempty"`
	Files          []FileEntry `json:"files"`
}

// Save writes the cache to <root>/.digestron/cache.v0.25.json.
func Save(root string, c *Cache) error {
	dir := filepath.Join(root, ".digestron")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("cache: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("cache: marshal: %w", err)
	}
	dest := filepath.Join(dir, cacheFile)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("cache: write %s: %w", dest, err)
	}
	return nil
}

// Load reads the cache from <root>/.digestron/cache.v0.25.json.
// It returns (nil, nil) when no cache file exists yet.
func Load(root string) (*Cache, error) {
	src := filepath.Join(root, ".digestron", cacheFile)
	data, err := os.ReadFile(src) //nolint:gosec // path is under repoRoot
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("cache: read %s: %w", src, err)
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("cache: unmarshal: %w", err)
	}
	return &c, nil
}

// IsClean reports whether the cached metadata for root still matches the
// current file system state.  It returns true only when every file recorded
// in the cache has an unchanged mtime and size.
func IsClean(root string, c *Cache) bool {
	if c == nil || c.RepoRoot != root {
		return false
	}
	for _, fe := range c.Files {
		info, err := os.Stat(filepath.Join(root, fe.Path))
		if err != nil {
			return false
		}
		if info.ModTime().Unix() != fe.MtimeUnix || info.Size() != fe.Size {
			return false
		}
	}
	return true
}
