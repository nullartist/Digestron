package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSaveLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		RepoRoot: dir,
		Engine:   "ts-morph",
		Files: []FileEntry{
			{Path: "src/a.ts", MtimeUnix: 1000, Size: 512, ModuleID: "mod_a"},
		},
	}
	if err := Save(dir, c); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil")
	}
	if got.RepoRoot != dir {
		t.Errorf("RepoRoot mismatch: %q", got.RepoRoot)
	}
	if got.Engine != "ts-morph" {
		t.Errorf("Engine mismatch: %q", got.Engine)
	}
	if len(got.Files) != 1 || got.Files[0].ModuleID != "mod_a" {
		t.Errorf("Files mismatch: %+v", got.Files)
	}
}

func TestLoad_NoCacheFile(t *testing.T) {
	dir := t.TempDir()
	got, err := Load(dir)
	if err != nil {
		t.Fatalf("Load on empty dir: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing cache, got %+v", got)
	}
}

func TestIsClean_NilCache(t *testing.T) {
	dir := t.TempDir()
	if IsClean(dir, nil) {
		t.Error("IsClean(nil) should return false")
	}
}

func TestIsClean_RootMismatch(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{RepoRoot: "/other"}
	if IsClean(dir, c) {
		t.Error("IsClean with mismatched root should return false")
	}
}

func TestIsClean_MatchingFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a real file and record its stat.
	p := filepath.Join(dir, "src", "a.ts")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)
	c := &Cache{
		RepoRoot: dir,
		Files: []FileEntry{{
			Path:      "src/a.ts",
			MtimeUnix: info.ModTime().Unix(),
			Size:      info.Size(),
		}},
	}
	if !IsClean(dir, c) {
		t.Error("IsClean should return true when mtime/size match")
	}
}

func TestIsClean_ChangedFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "src", "b.ts")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(p)

	// Simulate an old mtime in the cache.
	c := &Cache{
		RepoRoot: dir,
		Files: []FileEntry{{
			Path:      "src/b.ts",
			MtimeUnix: info.ModTime().Unix() - 1,
			Size:      info.Size(),
		}},
	}
	if IsClean(dir, c) {
		t.Error("IsClean should return false when mtime differs")
	}
}

func TestIsClean_MissingFile(t *testing.T) {
	dir := t.TempDir()
	c := &Cache{
		RepoRoot: dir,
		Files:    []FileEntry{{Path: "nonexistent.ts", MtimeUnix: time.Now().Unix(), Size: 10}},
	}
	if IsClean(dir, c) {
		t.Error("IsClean should return false for a missing file")
	}
}
