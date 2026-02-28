// Package snippets implements the v0.25 snippet extraction engine.
//
// Given a set of SnippetRequests (file + range or line), Build returns
// trimmed, labelled code blocks within a hard character budget.
package snippets

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SnippetRequest describes a code location to extract.
type SnippetRequest struct {
	File         string `json:"file"`
	StartLine    int    `json:"startLine,omitempty"`
	EndLine      int    `json:"endLine,omitempty"`
	Line         int    `json:"line,omitempty"`
	ContextLines int    `json:"contextLines,omitempty"`
	Label        string `json:"label,omitempty"`
	Priority     int    `json:"priority,omitempty"` // higher = earlier (seed > callers > callees)
}

// SnippetBlock is the extracted result for one merged code range.
type SnippetBlock struct {
	File      string `json:"file"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Label     string `json:"label,omitempty"`
	Text      string `json:"text"`
}

// Options controls snippet extraction.
type Options struct {
	RepoRoot    string
	BudgetChars int // hard character budget across all blocks
	MergeGap    int // merge adjacent ranges separated by at most this many lines
}

// Build extracts code snippets for the given requests and returns:
//   - the list of extracted blocks
//   - the concatenated text of all blocks (within budget)
//   - any non-fatal error
func Build(reqs []SnippetRequest, opt Options) ([]SnippetBlock, string, error) {
	if opt.BudgetChars <= 0 {
		opt.BudgetChars = 8000
	}
	if opt.MergeGap <= 0 {
		opt.MergeGap = 2
	}

	// Sort by priority desc, then file, then line — ensures deterministic output.
	sort.SliceStable(reqs, func(i, j int) bool {
		if reqs[i].Priority != reqs[j].Priority {
			return reqs[i].Priority > reqs[j].Priority
		}
		if reqs[i].File != reqs[j].File {
			return reqs[i].File < reqs[j].File
		}
		return effectiveLine(reqs[i]) < effectiveLine(reqs[j])
	})

	type rawRange struct {
		file     string
		a, b     int
		label    string
		priority int
	}

	var ranges []rawRange
	for _, s := range reqs {
		cl := s.ContextLines
		if cl <= 0 {
			cl = 20
		}
		a, b := s.StartLine, s.EndLine
		if s.Line > 0 {
			a = s.Line - cl
			b = s.Line + cl
		} else {
			a = a - cl
			b = b + cl
		}
		if a < 1 {
			a = 1
		}
		if b < a {
			b = a
		}
		ranges = append(ranges, rawRange{file: s.File, a: a, b: b, label: s.Label, priority: s.Priority})
	}

	// Group by file.
	byFile := map[string][]rawRange{}
	fileOrder := []string{}
	for _, r := range ranges {
		if _, exists := byFile[r.file]; !exists {
			fileOrder = append(fileOrder, r.file)
		}
		byFile[r.file] = append(byFile[r.file], r)
	}

	type mergedRange struct {
		file     string
		a, b     int
		labels   []string
		priority int
	}

	var mergedAll []mergedRange

	for _, f := range fileOrder {
		rs := byFile[f]
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].a < rs[j].a })

		cur := mergedRange{
			file:     f,
			a:        rs[0].a,
			b:        rs[0].b,
			labels:   []string{rs[0].label},
			priority: rs[0].priority,
		}
		for i := 1; i < len(rs); i++ {
			if rs[i].a <= cur.b+opt.MergeGap {
				if rs[i].b > cur.b {
					cur.b = rs[i].b
				}
				if rs[i].label != "" {
					cur.labels = append(cur.labels, rs[i].label)
				}
				if rs[i].priority > cur.priority {
					cur.priority = rs[i].priority
				}
			} else {
				mergedAll = append(mergedAll, cur)
				cur = mergedRange{
					file:     f,
					a:        rs[i].a,
					b:        rs[i].b,
					labels:   []string{rs[i].label},
					priority: rs[i].priority,
				}
			}
		}
		mergedAll = append(mergedAll, cur)
	}

	// Sort merged blocks by priority desc, then file, then start line.
	sort.SliceStable(mergedAll, func(i, j int) bool {
		if mergedAll[i].priority != mergedAll[j].priority {
			return mergedAll[i].priority > mergedAll[j].priority
		}
		if mergedAll[i].file != mergedAll[j].file {
			return mergedAll[i].file < mergedAll[j].file
		}
		return mergedAll[i].a < mergedAll[j].a
	})

	var blocks []SnippetBlock
	var text strings.Builder

	remaining := opt.BudgetChars
	for _, m := range mergedAll {
		if remaining <= 0 {
			break
		}

		abs, err := safeJoin(opt.RepoRoot, m.file)
		if err != nil {
			// skip paths outside repoRoot (path traversal guard)
			continue
		}

		lines, err := readLines(abs)
		if err != nil {
			// skip unreadable files gracefully
			continue
		}

		a := clamp(m.a, 1, len(lines))
		b := clamp(m.b, 1, len(lines))
		if b < a {
			b = a
		}

		header := fmt.Sprintf("--- %s:L%d-L%d [%s] ---\n", m.file, a, b, joinLabels(m.labels))
		body := buildBlockText(a, b, lines[a-1:b])
		blockText := header + body + "\n"

		if len(blockText) > remaining {
			blockText = trimBlock(header, lines, a, b, remaining)
			if blockText == "" {
				continue
			}
		}

		blocks = append(blocks, SnippetBlock{
			File:      m.file,
			StartLine: a,
			EndLine:   b,
			Label:     joinLabels(m.labels),
			Text:      blockText,
		})

		text.WriteString(blockText)
		remaining = opt.BudgetChars - text.Len()
	}

	return blocks, text.String(), nil
}

// safeJoin joins repoRoot and a relative file path and verifies the result
// stays within repoRoot (path traversal guard).
func safeJoin(repoRoot, file string) (string, error) {
	if repoRoot == "" {
		return filepath.FromSlash(strings.TrimPrefix(file, "./")), nil
	}
	abs := filepath.Join(repoRoot, filepath.FromSlash(strings.TrimPrefix(file, "./")))
	rootAbs, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	absClean, err := filepath.Abs(abs)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absClean, rootAbs+string(filepath.Separator)) && absClean != rootAbs {
		return "", fmt.Errorf("snippets: path %q is outside repo root", file)
	}
	return absClean, nil
}

func effectiveLine(s SnippetRequest) int {
	if s.Line > 0 {
		return s.Line
	}
	if s.StartLine > 0 {
		return s.StartLine
	}
	return 1
}

func joinLabels(ls []string) string {
	uniq := map[string]struct{}{}
	var out []string
	for _, l := range ls {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if _, ok := uniq[l]; ok {
			continue
		}
		uniq[l] = struct{}{}
		out = append(out, l)
	}
	if len(out) == 0 {
		return "snippet"
	}
	if len(out) > 3 {
		return strings.Join(out[:3], ",") + ",+more"
	}
	return strings.Join(out, ",")
}

func buildBlockText(start, end int, chunk []string) string {
	var b strings.Builder
	for i, line := range chunk {
		fmt.Fprintf(&b, "%5d | %s\n", start+i, line)
	}
	return b.String()
}

func trimBlock(header string, lines []string, a, b, budget int) string {
	if budget <= len(header)+80 {
		return ""
	}
	maxBody := budget - len(header) - 10
	total := b - a + 1
	if total <= 0 {
		return ""
	}

	// Rough estimate: 20 chars per line average.
	keep := maxBody / 20
	if keep < 6 {
		keep = 6
	}
	head := keep / 2
	tail := keep - head
	if head < 3 {
		head = 3
	}
	if tail < 3 {
		tail = 3
	}

	if head+tail >= total {
		body := buildBlockText(a, b, lines[a-1:b])
		return header + body + "\n"
	}

	// Clamp slice bounds to valid indices within lines.
	headEnd := clamp(a+head-1, a, len(lines))
	tailStart := clamp(b-tail, 0, len(lines)-1)
	tailEnd := clamp(b, 1, len(lines))

	var out strings.Builder
	out.WriteString(header)
	out.WriteString(buildBlockText(a, headEnd, lines[a-1:headEnd]))
	out.WriteString("  ... | ...\n")
	out.WriteString(buildBlockText(tailStart+1, tailEnd, lines[tailStart:tailEnd]))
	out.WriteString("\n")
	s := out.String()
	if len(s) > budget {
		return s[:budget]
	}
	return s
}

func readLines(abs string) ([]string, error) {
	f, err := os.Open(abs) //nolint:gosec // path is validated by safeJoin
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 2*1024*1024)

	var ls []string
	for sc.Scan() {
		ls = append(ls, sc.Text())
	}
	return ls, sc.Err()
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
