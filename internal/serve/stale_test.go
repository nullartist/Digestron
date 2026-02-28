package serve

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nullartist/digestron/internal/proto"
	"github.com/nullartist/digestron/internal/usg"
)

// ---- CheckRepoFreshness tests ----

func TestCheckRepoFreshness_NoUSG_NoSources(t *testing.T) {
	dir := t.TempDir()
	fresh, err := CheckRepoFreshness(dir, FreshnessOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fresh.IsStale {
		t.Error("IsStale should be false when no USG exists")
	}
	if !fresh.USGModTime.IsZero() {
		t.Error("USGModTime should be zero when no USG file exists")
	}
}

func TestCheckRepoFreshness_NoUSG_WithSources(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "foo.ts"), []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	fresh, err := CheckRepoFreshness(dir, FreshnessOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No USG → cannot be stale.
	if fresh.IsStale {
		t.Error("IsStale should be false when no USG file exists")
	}
	if fresh.RepoLatestFile == "" {
		t.Error("expected RepoLatestFile to be set")
	}
}

func TestCheckRepoFreshness_FreshUSG(t *testing.T) {
	dir := t.TempDir()

	// Write a source file with a past mtime.
	srcPath := filepath.Join(dir, "a.ts")
	if err := os.WriteFile(srcPath, []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(srcPath, past, past); err != nil {
		t.Fatal(err)
	}

	// Write USG file with a recent mtime (after the source file).
	usgDir := filepath.Join(dir, ".digestron")
	if err := os.MkdirAll(usgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usgPath := filepath.Join(usgDir, "usg.v0.1.json")
	if err := os.WriteFile(usgPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(usgPath, now, now); err != nil {
		t.Fatal(err)
	}

	fresh, err := CheckRepoFreshness(dir, FreshnessOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fresh.IsStale {
		t.Errorf("IsStale should be false when USG is newer than sources; usgModTime=%v repoLatest=%v",
			fresh.USGModTime, fresh.RepoLatestTime)
	}
}

func TestCheckRepoFreshness_StaleUSG(t *testing.T) {
	dir := t.TempDir()

	// Write USG file with an old mtime.
	usgDir := filepath.Join(dir, ".digestron")
	if err := os.MkdirAll(usgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usgPath := filepath.Join(usgDir, "usg.v0.1.json")
	if err := os.WriteFile(usgPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(usgPath, old, old); err != nil {
		t.Fatal(err)
	}

	// Write a source file with a recent mtime.
	srcPath := filepath.Join(dir, "b.ts")
	if err := os.WriteFile(srcPath, []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh, err := CheckRepoFreshness(dir, FreshnessOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fresh.IsStale {
		t.Errorf("IsStale should be true when a source file is newer than the USG; usgModTime=%v repoLatest=%v",
			fresh.USGModTime, fresh.RepoLatestTime)
	}
	if fresh.RepoLatestFile == "" {
		t.Error("expected RepoLatestFile to be populated")
	}
}

func TestCheckRepoFreshness_IgnoresNodeModules(t *testing.T) {
	dir := t.TempDir()

	// Write USG file with an old mtime.
	usgDir := filepath.Join(dir, ".digestron")
	if err := os.MkdirAll(usgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usgPath := filepath.Join(usgDir, "usg.v0.1.json")
	if err := os.WriteFile(usgPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(usgPath, old, old); err != nil {
		t.Fatal(err)
	}

	// A .ts file inside node_modules should be ignored.
	nmDir := filepath.Join(dir, "node_modules", "pkg")
	if err := os.MkdirAll(nmDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nmDir, "index.ts"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	fresh, err := CheckRepoFreshness(dir, FreshnessOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fresh.IsStale {
		t.Error("IsStale should be false when only node_modules changed")
	}
}

func TestCheckRepoFreshness_IncludeJS(t *testing.T) {
	dir := t.TempDir()

	// Write USG file with an old mtime.
	usgDir := filepath.Join(dir, ".digestron")
	if err := os.MkdirAll(usgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usgPath := filepath.Join(usgDir, "usg.v0.1.json")
	if err := os.WriteFile(usgPath, []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(usgPath, old, old); err != nil {
		t.Fatal(err)
	}

	// A .js file should be considered only when IncludeJS=true.
	if err := os.WriteFile(filepath.Join(dir, "c.js"), []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}

	freshNoJS, _ := CheckRepoFreshness(dir, FreshnessOptions{IncludeJS: false})
	if freshNoJS.IsStale {
		t.Error("IsStale should be false when IncludeJS=false and only .js changed")
	}

	freshWithJS, _ := CheckRepoFreshness(dir, FreshnessOptions{IncludeJS: true})
	if !freshWithJS.IsStale {
		t.Error("IsStale should be true when IncludeJS=true and a .js file is newer than USG")
	}
}

// ---- handleEnsureIndexed tests (via handle()) ----

func TestHandle_EnsureIndexed_Memory(t *testing.T) {
	g := &usg.USG{
		Symbols: []usg.Symbol{sym("s1", "a::Foo", "Foo", "function", "a.ts", 1)},
		Stats:   usg.Stats{TotalModules: 1},
	}
	st := makeState(g)

	resp := handle(st, req("ensureIndexed", map[string]interface{}{
		"autoIndex": false,
	}))
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp.Error)
	}
	m := resp.Result.(map[string]any)
	if m["source"] != "memory" {
		t.Errorf("source = %v, want memory", m["source"])
	}
	if m["indexed"] != true {
		t.Error("indexed should be true")
	}
}

func TestHandle_EnsureIndexed_NotIndexed_NoAutoIndex(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("ensureIndexed", map[string]interface{}{
		"autoIndex": false,
	}))
	if resp.Ok {
		t.Error("expected ok=false when not indexed and autoIndex=false")
	}
	if resp.Error == nil || resp.Error.Code != "NOT_INDEXED" {
		t.Errorf("expected NOT_INDEXED, got %+v", resp.Error)
	}
}

func TestHandle_EnsureIndexed_Stale_NoAutoIndex(t *testing.T) {
	dir := t.TempDir()

	// Write an old USG file.
	usgDir := filepath.Join(dir, ".digestron")
	if err := os.MkdirAll(usgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usgPath := filepath.Join(usgDir, "usg.v0.1.json")
	if err := os.WriteFile(usgPath, []byte(`{"version":"usg.v0.1","root":"`+dir+`","modules":[],"symbols":[],"edges":{},"stats":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(usgPath, old, old); err != nil {
		t.Fatal(err)
	}

	// A newer .ts file makes the index stale.
	if err := os.WriteFile(filepath.Join(dir, "src.ts"), []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := NewState(dir)
	resp := handle(st, proto.Request{V: proto.Version, ID: "t1", Op: "ensureIndexed", Params: map[string]interface{}{
		"repoRoot":  dir,
		"autoIndex": false,
	}})
	if resp.Ok {
		t.Error("expected ok=false for stale index with autoIndex=false")
	}
	if resp.Error == nil || resp.Error.Code != "STALE_INDEX" {
		t.Errorf("expected STALE_INDEX, got %+v", resp.Error)
	}
	// freshness details should be in the result
	if resp.Result == nil {
		t.Error("expected result with freshness details")
	}
}

func TestHandle_EnsureIndexed_Stale_ReindexDisabled(t *testing.T) {
	dir := t.TempDir()

	// Write an old USG file.
	usgDir := filepath.Join(dir, ".digestron")
	if err := os.MkdirAll(usgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	usgPath := filepath.Join(usgDir, "usg.v0.1.json")
	if err := os.WriteFile(usgPath, []byte(`{"version":"usg.v0.1","root":"`+dir+`","modules":[],"symbols":[],"edges":{},"stats":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(usgPath, old, old); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(dir, "src.ts"), []byte("export {}"), 0o644); err != nil {
		t.Fatal(err)
	}

	st := NewState(dir)
	resp := handle(st, proto.Request{V: proto.Version, ID: "t2", Op: "ensureIndexed", Params: map[string]interface{}{
		"repoRoot":       dir,
		"autoIndex":      true,
		"reindexIfStale": false,
	}})
	if resp.Ok {
		t.Error("expected ok=false when reindexIfStale=false")
	}
	if resp.Error == nil || resp.Error.Code != "STALE_INDEX" {
		t.Errorf("expected STALE_INDEX, got %+v", resp.Error)
	}
}

func writeOldUSG(t *testing.T, dir string) {
t.Helper()
usgDir := filepath.Join(dir, ".digestron")
if err := os.MkdirAll(usgDir, 0o755); err != nil {
t.Fatal(err)
}
usgPath := filepath.Join(usgDir, "usg.v0.1.json")
if err := os.WriteFile(usgPath, []byte("{}"), 0o644); err != nil {
t.Fatal(err)
}
old := time.Now().Add(-2 * time.Hour)
if err := os.Chtimes(usgPath, old, old); err != nil {
t.Fatal(err)
}
}

func TestCheckRepoFreshness_ConfigFiles(t *testing.T) {
configFiles := []string{
"package.json",
"package-lock.json",
"pnpm-lock.yaml",
"yarn.lock",
"bun.lockb",
"tsconfig.json",
"tsconfig.base.json",
"tsconfig.build.json",
"turbo.json",
"nx.json",
".nvmrc",
"vite.config.ts",
"vite.config.js",
"webpack.config.js",
"rollup.config.mjs",
"esbuild.config.js",
"next.config.js",
"next.config.ts",
}

for _, name := range configFiles {
t.Run(name, func(t *testing.T) {
dir := t.TempDir()
writeOldUSG(t, dir)

if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o644); err != nil {
t.Fatal(err)
}

fresh, err := CheckRepoFreshness(dir, FreshnessOptions{})
if err != nil {
t.Fatalf("unexpected error: %v", err)
}
if !fresh.IsStale {
t.Errorf("IsStale should be true when %s is newer than USG (repoLatestFile=%s)", name, fresh.RepoLatestFile)
}
})
}
}
