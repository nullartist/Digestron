// Package serve implements the Digestron stdio NDJSON server (v0.25).
//
// Start it with Run(repoPath); it reads one JSON request per line from stdin
// and writes one JSON response per line to stdout.
package serve

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/nullartist/digestron/internal/cache"
	"github.com/nullartist/digestron/internal/focus"
	"github.com/nullartist/digestron/internal/indexer"
	"github.com/nullartist/digestron/internal/proto"
	"github.com/nullartist/digestron/internal/search"
	"github.com/nullartist/digestron/internal/snippets"
	"github.com/nullartist/digestron/internal/usg"
)

// State holds the in-memory server state shared across requests.
type State struct {
	mu       sync.RWMutex
	repoRoot string
	graph    *usg.USG
	view     *usg.View
}

// Run starts the NDJSON stdio server rooted at repoPath.
// It blocks until stdin is closed or an I/O error occurs.
func Run(repoPath string) error {
	repoAbs, err := filepath.Abs(repoPath)
	if err != nil {
		return err
	}

	st := &State{repoRoot: repoAbs}

	// Try to warm up from a previously indexed USG so the first request is fast.
	if g, err := usg.Load(repoAbs); err == nil {
		st.graph = g
		st.view = usg.BuildView(g)
	}

	in := bufio.NewScanner(os.Stdin)
	// Allow large requests (up to 8 MiB per line).
	in.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	out := bufio.NewWriter(os.Stdout)
	defer out.Flush()

	for in.Scan() {
		line := in.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		var req proto.Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeResp(out, proto.Response{
				V:     proto.Version,
				ID:    "",
				Ok:    false,
				Error: &proto.ErrorObj{Code: "BAD_JSON", Message: err.Error()},
			})
			continue
		}

		resp := handle(st, req)
		writeResp(out, resp)
	}

	return in.Err()
}

// writeResp marshals a response as a single JSON line and flushes immediately.
func writeResp(w *bufio.Writer, resp proto.Response) {
	if resp.V == "" {
		resp.V = proto.Version
	}
	b, _ := json.Marshal(resp)
	fmt.Fprintln(w, string(b))
	w.Flush() //nolint:errcheck // best-effort flush on stdout
}

// handle dispatches a request to the appropriate handler.
func handle(st *State, req proto.Request) proto.Response {
	if req.V == "" {
		req.V = proto.Version
	}
	op := strings.ToLower(req.Op)

	switch op {
	case "doctor":
		// In serve mode, doctor is a lightweight health probe.
		// Full dependency installation belongs in the CLI doctor command.
		return proto.Response{V: req.V, ID: req.ID, Ok: true,
			Result: map[string]any{"status": "ok"}}

	case "health":
		st.mu.RLock()
		indexed := st.graph != nil
		st.mu.RUnlock()
		return proto.Response{V: req.V, ID: req.ID, Ok: true,
			Result: map[string]any{"indexed": indexed}}

	case "index":
		return handleIndex(st, req)

	case "search":
		return handleSearch(st, req)

	case "impact":
		return handleImpact(st, req)

	case "snippets":
		return handleSnippets(st, req)

	default:
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "UNKNOWN_OP", Message: "unknown op: " + req.Op}}
	}
}

// handleIndex runs the TypeScript extractor, persists the USG, and updates State.
func handleIndex(st *State, req proto.Request) proto.Response {
	st.mu.RLock()
	repoRoot := st.repoRoot
	st.mu.RUnlock()

	// Optional params.
	var tsconfigs []string
	var includeTests bool
	var forceEngine string

	if tc, ok := req.Params["tsconfigs"]; ok {
		if arr, ok := tc.([]interface{}); ok {
			for _, v := range arr {
				if s, ok := v.(string); ok {
					tsconfigs = append(tsconfigs, s)
				}
			}
		}
	}
	if it, ok := req.Params["includeTests"]; ok {
		if b, ok := it.(bool); ok {
			includeTests = b
		}
	}
	if fe, ok := req.Params["forceEngine"]; ok {
		if s, ok := fe.(string); ok {
			forceEngine = s
		}
	}

	// Check incremental cache — skip extraction if nothing changed.
	if c, err := cache.Load(repoRoot); err == nil && cache.IsClean(repoRoot, c) {
		if g, err := usg.Load(repoRoot); err == nil {
			st.mu.Lock()
			st.graph = g
			st.view = usg.BuildView(g)
			st.mu.Unlock()
			return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: map[string]any{
				"cached":  true,
				"modules": g.Stats.TotalModules,
				"symbols": g.Stats.TotalSymbols,
				"calls":   g.Stats.CallsTotal,
			}}
		}
	}

	resp, err := indexer.RunTSExtract(repoRoot, indexer.RunOptions{
		Tsconfigs:    tsconfigs,
		IncludeTests: includeTests,
		ForceEngine:  forceEngine,
	})
	if err != nil {
		msg := err.Error()
		if resp != nil && len(resp.Diagnostics) > 0 {
			msg = resp.Diagnostics[0].Message
		}
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "INDEX_FAILED", Message: msg}}
	}

	var raw struct {
		Modules      []usg.Module            `json:"modules"`
		Symbols      []usg.Symbol            `json:"symbols"`
		Calls        []usg.CallEdge          `json:"calls"`
		Inherits     []usg.InheritanceEdge   `json:"inherits"`
		Instantiates []usg.InstantiationSite `json:"instantiates"`
		EntryPoints  []usg.EntryPoint        `json:"entryPoints"`
		RiskFlags    []usg.RiskFlag          `json:"riskFlags"`
		Stats        usg.Stats               `json:"stats"`
	}
	if err := json.Unmarshal(resp.Raw, &raw); err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "UNMARSHAL_FAILED", Message: err.Error()}}
	}

	g := &usg.USG{
		Version:     "usg.v0.1",
		Root:        repoRoot,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Language:    []string{"ts"},
		Modules:     raw.Modules,
		Symbols:     raw.Symbols,
		Edges: usg.Edges{
			Calls:        raw.Calls,
			Inherits:     raw.Inherits,
			Instantiates: raw.Instantiates,
		},
		EntryPoints: raw.EntryPoints,
		RiskFlags:   raw.RiskFlags,
		Stats:       raw.Stats,
	}

	if err := usg.Save(repoRoot, g); err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "SAVE_FAILED", Message: err.Error()}}
	}

	// Update in-memory state.
	st.mu.Lock()
	st.graph = g
	st.view = usg.BuildView(g)
	st.mu.Unlock()

	// Persist cache metadata.
	c := &cache.Cache{RepoRoot: repoRoot, Engine: resp.Engine}
	for _, m := range g.Modules {
		info, err := os.Stat(filepath.Join(repoRoot, m.Path))
		if err != nil {
			continue
		}
		var symIDs []string
		for _, s := range g.Symbols {
			if s.ModuleID == m.ID {
				symIDs = append(symIDs, s.ID)
			}
		}
		edgeCount := 0
		for _, e := range g.Edges.Calls {
			if e.Loc.File == m.Path {
				edgeCount++
			}
		}
		c.Files = append(c.Files, cache.FileEntry{
			Path:      m.Path,
			MtimeUnix: info.ModTime().Unix(),
			Size:      info.Size(),
			ModuleID:  m.ID,
			SymbolIDs: symIDs,
			EdgeCount: edgeCount,
		})
	}
	_ = cache.Save(repoRoot, c) // best-effort

	return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: map[string]any{
		"cached":  false,
		"modules": g.Stats.TotalModules,
		"symbols": g.Stats.TotalSymbols,
		"calls":   g.Stats.CallsTotal,
	}}
}

// handleSearch searches the in-memory USG for a symbol by name/qname.
func handleSearch(st *State, req proto.Request) proto.Response {
	st.mu.RLock()
	g := st.graph
	st.mu.RUnlock()

	if g == nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "NOT_INDEXED", Message: "run index first"}}
	}

	query, _ := req.Params["query"].(string)
	if query == "" {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "BAD_PARAMS", Message: "query is required"}}
	}

	results := search.Find(g, query)
	type hit struct {
		ID    string `json:"id"`
		QName string `json:"qname"`
		Kind  string `json:"kind"`
		File  string `json:"file"`
		Line  int    `json:"line"`
		Score int    `json:"score"`
	}
	hits := make([]hit, 0, len(results))
	for _, r := range results {
		hits = append(hits, hit{
			ID:    r.Symbol.ID,
			QName: r.Symbol.QName,
			Kind:  r.Symbol.Kind,
			File:  r.Symbol.Loc.File,
			Line:  r.Symbol.Loc.StartLine,
			Score: r.Score,
		})
	}
	return proto.Response{V: req.V, ID: req.ID, Ok: true,
		Result: map[string]any{"hits": hits}}
}

// handleImpact builds a FocusPackJSON for the requested symbol.
func handleImpact(st *State, req proto.Request) proto.Response {
	st.mu.RLock()
	g := st.graph
	v := st.view
	repoRoot := st.repoRoot
	st.mu.RUnlock()

	// Try to load from disk when not yet indexed (nice UX).
	if g == nil || v == nil {
		if rr, ok := req.Params["repoRoot"].(string); ok && rr != "" {
			if abs, err := filepath.Abs(rr); err == nil {
				repoRoot = abs
			}
		}
		loaded, err := usg.Load(repoRoot)
		if err != nil {
			return proto.Response{V: req.V, ID: req.ID, Ok: false,
				Error: &proto.ErrorObj{Code: "NOT_INDEXED", Message: "run index first"}}
		}
		g = loaded
		v = usg.BuildView(loaded)
		st.mu.Lock()
		st.repoRoot = repoRoot
		st.graph = g
		st.view = v
		st.mu.Unlock()
	}

	ref, _ := req.Params["ref"].(string)
	if ref == "" {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "BAD_PARAMS", Message: "ref is required"}}
	}

	radius := 2
	if r, ok := req.Params["radius"].(float64); ok && int(r) > 0 {
		radius = int(r)
	}
	budgetChars := 9000
	if b, ok := req.Params["budgetChars"].(float64); ok && int(b) > 0 {
		budgetChars = int(b)
	}
	includeSnippets, _ := req.Params["includeSnippets"].(bool)
	snipBudget := 8000
	if x, ok := req.Params["snippetsBudgetChars"].(float64); ok && int(x) > 0 {
		snipBudget = int(x)
	}

	opts := focus.Options{Radius: radius, BudgetChars: budgetChars}
	pack, err := focus.Build(g, ref, opts)
	if err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "SYMBOL_NOT_FOUND", Message: err.Error()}}
	}

	fp := focus.BuildPackJSON(pack, g, v, repoRoot, radius, budgetChars)

	if includeSnippets {
		sreqs := focus.BuildSnippetRequests(fp, 3, 3)
		blocks, text, _ := snippets.Build(sreqs, snippets.Options{
			RepoRoot:    repoRoot,
			BudgetChars: snipBudget,
			MergeGap:    2,
		})
		snipBlocks := make([]focus.SnippetBlockJSON, 0, len(blocks))
		for _, b := range blocks {
			snipBlocks = append(snipBlocks, focus.SnippetBlockJSON{
				File: b.File, StartLine: b.StartLine, EndLine: b.EndLine,
				Label: b.Label, Text: b.Text,
			})
		}
		fp.Snippets = &focus.SnippetsPayload{
			BudgetChars: snipBudget,
			Blocks:      snipBlocks,
			Text:        text,
		}
	}

	return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: map[string]any{
		"focusText": pack.Text,
		"focus":     fp,
	}}
}

// handleSnippets extracts code snippets for an explicit list of locations.
// Accepts either a "requests" or legacy "locs" array in params.
func handleSnippets(st *State, req proto.Request) proto.Response {
	st.mu.RLock()
	repoRoot := st.repoRoot
	st.mu.RUnlock()

	budgetChars := 8000
	if b, ok := req.Params["budgetChars"].(float64); ok && int(b) > 0 {
		budgetChars = int(b)
	}

	// Accept "requests" (v0.25) or legacy "locs".
	var sreqs []snippets.SnippetRequest
	for _, key := range []string{"requests", "locs"} {
		if raw, ok := req.Params[key]; ok {
			b, err := json.Marshal(raw)
			if err == nil {
				_ = json.Unmarshal(b, &sreqs)
			}
			break
		}
	}

	if len(sreqs) == 0 {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "BAD_PARAMS", Message: "locs is required and must be non-empty"}}
	}

	blocks, text, err := snippets.Build(sreqs, snippets.Options{
		RepoRoot:    repoRoot,
		BudgetChars: budgetChars,
	})
	if err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "SNIPPETS_ERROR", Message: err.Error()}}
	}

	return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: map[string]any{
		"budgetChars": budgetChars,
		"blocks":      blocks,
		"text":        text,
	}}
}
