package snippets

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helpers

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeTempFile: %v", err)
	}
	return name // return relative name
}

func makeLines(n int) string {
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		sb.WriteString(strings.Repeat("x", 40))
		sb.WriteByte('\n')
	}
	return sb.String()
}

// ---- Build tests ----

func TestBuild_EmptyRequests(t *testing.T) {
	blocks, text, err := Build(nil, Options{BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 0 || text != "" {
		t.Errorf("expected empty result for no requests")
	}
}

func TestBuild_SingleLine(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	rel := writeTempFile(t, dir, "foo.ts", content)

	reqs := []SnippetRequest{{File: rel, Line: 3, ContextLines: 1, Label: "test"}}
	blocks, text, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if !strings.Contains(text, "foo.ts") {
		t.Errorf("expected file name in text, got:\n%s", text)
	}
	if !strings.Contains(text, "[test]") {
		t.Errorf("expected label in header, got:\n%s", text)
	}
	// lines 2-4 should be present
	if !strings.Contains(text, "line2") || !strings.Contains(text, "line4") {
		t.Errorf("expected context lines in output, got:\n%s", text)
	}
}

func TestBuild_RangeRequest(t *testing.T) {
	dir := t.TempDir()
	content := makeLines(50)
	rel := writeTempFile(t, dir, "range.ts", content)

	reqs := []SnippetRequest{{File: rel, StartLine: 10, EndLine: 20, ContextLines: 2}}
	blocks, _, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if blocks[0].StartLine > 10 || blocks[0].EndLine < 20 {
		t.Errorf("range not expanded correctly: start=%d end=%d", blocks[0].StartLine, blocks[0].EndLine)
	}
}

func TestBuild_MergeAdjacentRanges(t *testing.T) {
	dir := t.TempDir()
	content := makeLines(100)
	rel := writeTempFile(t, dir, "merge.ts", content)

	reqs := []SnippetRequest{
		{File: rel, Line: 10, ContextLines: 3, Label: "a"},
		{File: rel, Line: 14, ContextLines: 3, Label: "b"}, // overlaps with first
	}
	blocks, _, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 8000, MergeGap: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The two requests should be merged into one block.
	if len(blocks) != 1 {
		t.Errorf("expected 1 merged block, got %d", len(blocks))
	}
}

func TestBuild_PriorityOrdering(t *testing.T) {
	dir := t.TempDir()
	writeTempFile(t, dir, "a.ts", makeLines(50))
	writeTempFile(t, dir, "b.ts", makeLines(50))

	reqs := []SnippetRequest{
		{File: "b.ts", Line: 10, ContextLines: 2, Label: "callee", Priority: 3},
		{File: "a.ts", Line: 10, ContextLines: 2, Label: "seed", Priority: 10},
	}
	blocks, _, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) < 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	// seed (priority 10) should come before callee (priority 3)
	if blocks[0].Label != "seed" {
		t.Errorf("expected seed block first, got label=%q", blocks[0].Label)
	}
}

func TestBuild_BudgetRespected(t *testing.T) {
	dir := t.TempDir()
	// File with many lines so it would exceed a small budget
	content := makeLines(500)
	rel := writeTempFile(t, dir, "big.ts", content)

	reqs := []SnippetRequest{{File: rel, StartLine: 1, EndLine: 500, ContextLines: 0}}
	_, text, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 500})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(text) > 600 { // small slack for header/elision markers
		t.Errorf("text exceeded budget: len=%d", len(text))
	}
}

func TestBuild_MissingFileSkipped(t *testing.T) {
	dir := t.TempDir()
	reqs := []SnippetRequest{{File: "nonexistent.ts", Line: 5, Label: "x"}}
	blocks, text, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(blocks) != 0 || text != "" {
		t.Errorf("expected empty result for missing file")
	}
}

func TestBuild_PathTraversalRejected(t *testing.T) {
	dir := t.TempDir()
	// Craft a path that attempts to escape repoRoot
	reqs := []SnippetRequest{{File: "../../etc/passwd", Line: 1}}
	blocks, text, err := Build(reqs, Options{RepoRoot: dir, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should be silently skipped (path traversal guard)
	if len(blocks) != 0 || text != "" {
		t.Errorf("expected traversal path to be skipped")
	}
}

// ---- joinLabels tests ----

func TestJoinLabels_Empty(t *testing.T) {
	if got := joinLabels(nil); got != "snippet" {
		t.Errorf("joinLabels(nil) = %q, want 'snippet'", got)
	}
}

func TestJoinLabels_Dedup(t *testing.T) {
	if got := joinLabels([]string{"a", "a", "b"}); got != "a,b" {
		t.Errorf("joinLabels dedup = %q, want 'a,b'", got)
	}
}

func TestJoinLabels_TruncateLong(t *testing.T) {
	got := joinLabels([]string{"a", "b", "c", "d"})
	if !strings.HasSuffix(got, ",+more") {
		t.Errorf("joinLabels long = %q, want suffix ',+more'", got)
	}
}

// ---- clamp tests ----

func TestClamp(t *testing.T) {
	if clamp(0, 1, 10) != 1 {
		t.Error("clamp below lo")
	}
	if clamp(5, 1, 10) != 5 {
		t.Error("clamp in range")
	}
	if clamp(15, 1, 10) != 10 {
		t.Error("clamp above hi")
	}
}
