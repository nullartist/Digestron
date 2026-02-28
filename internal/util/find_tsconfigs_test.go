package util

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindTSConfigs_Empty(t *testing.T) {
	dir := t.TempDir()
	got, err := FindTSConfigs(dir, FindTSConfigsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no results, got %v", got)
	}
}

func TestFindTSConfigs_RootOnly(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), "{}")

	got, err := FindTSConfigs(dir, FindTSConfigsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 result, got %v", got)
	}
	if got[0] != "./tsconfig.json" {
		t.Errorf("unexpected path: %s", got[0])
	}
}

func TestFindTSConfigs_Ranking(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), "{}")
	writeFile(t, filepath.Join(dir, "tsconfig.base.json"), "{}")

	pkgDir := filepath.Join(dir, "packages", "core")
	must(t, os.MkdirAll(pkgDir, 0o755))
	writeFile(t, filepath.Join(pkgDir, "tsconfig.json"), "{}")

	got, err := FindTSConfigs(dir, FindTSConfigsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// root tsconfig.json must be first
	if got[0] != "./tsconfig.json" {
		t.Errorf("expected ./tsconfig.json first, got %s", got[0])
	}
	// tsconfig.base.json must be second
	if got[1] != "./tsconfig.base.json" {
		t.Errorf("expected ./tsconfig.base.json second, got %s", got[1])
	}
}

func TestFindTSConfigs_SkipsNodeModules(t *testing.T) {
	dir := t.TempDir()
	nmDir := filepath.Join(dir, "node_modules", "some-pkg")
	must(t, os.MkdirAll(nmDir, 0o755))
	writeFile(t, filepath.Join(nmDir, "tsconfig.json"), "{}")
	writeFile(t, filepath.Join(dir, "tsconfig.json"), "{}")

	got, err := FindTSConfigs(dir, FindTSConfigsOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range got {
		if contains(p, "node_modules") {
			t.Errorf("should not include node_modules path: %s", p)
		}
	}
	if len(got) != 1 {
		t.Errorf("expected 1 result (root only), got %v", got)
	}
}

func TestFindTSConfigs_SkipsTestConfigsByDefault(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), "{}")
	writeFile(t, filepath.Join(dir, "tsconfig.test.json"), "{}")

	got, err := FindTSConfigs(dir, FindTSConfigsOptions{IncludeTests: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, p := range got {
		if p == "./tsconfig.test.json" {
			t.Errorf("tsconfig.test.json should be excluded when IncludeTests=false")
		}
	}
}

func TestFindTSConfigs_IncludesTestConfigsWhenRequested(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tsconfig.json"), "{}")
	writeFile(t, filepath.Join(dir, "tsconfig.test.json"), "{}")

	got, err := FindTSConfigs(dir, FindTSConfigsOptions{IncludeTests: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	found := false
	for _, p := range got {
		if p == "./tsconfig.test.json" {
			found = true
		}
	}
	if !found {
		t.Errorf("tsconfig.test.json should be included when IncludeTests=true, got %v", got)
	}
}

func TestFindTSConfigs_MaxResults(t *testing.T) {
	dir := t.TempDir()
	for _, sub := range []string{"a", "b", "c"} {
		d := filepath.Join(dir, sub)
		must(t, os.MkdirAll(d, 0o755))
		writeFile(t, filepath.Join(d, "tsconfig.json"), "{}")
	}

	got, err := FindTSConfigs(dir, FindTSConfigsOptions{MaxResults: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) > 2 {
		t.Errorf("expected at most 2 results, got %d: %v", len(got), got)
	}
}

// helpers

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o755))
	must(t, os.WriteFile(path, []byte(content), 0o644))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}
