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

// handleImpact builds a FocusPack for the requested symbol.
func handleImpact(st *State, req proto.Request) proto.Response {
	st.mu.RLock()
	g := st.graph
	st.mu.RUnlock()

	if g == nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "NOT_INDEXED", Message: "run index first"}}
	}

	ref, _ := req.Params["ref"].(string)
	if ref == "" {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "BAD_PARAMS", Message: "ref is required"}}
	}

	radius := 2
	if r, ok := req.Params["radius"].(float64); ok {
		radius = int(r)
	}
	budgetChars := 8000
	if b, ok := req.Params["budgetChars"].(float64); ok {
		budgetChars = int(b)
	}
	includeSnippets, _ := req.Params["includeSnippets"].(bool)

	opts := focus.Options{Radius: radius, BudgetChars: budgetChars}
	pack, err := focus.Build(g, ref, opts)
	if err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "SYMBOL_NOT_FOUND", Message: err.Error()}}
	}

	focusJSON, err := focus.FormatJSON(pack)
	if err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "JSON_ERROR", Message: err.Error()}}
	}

	result := map[string]any{
		"focusText": pack.Text,
		"focusJSON": json.RawMessage(focusJSON),
	}

	if includeSnippets {
		st.mu.RLock()
		repoRoot := st.repoRoot
		st.mu.RUnlock()

		var sreqs []snippets.SnippetRequest

		// Seed symbol range.
		seed := pack.SeedSymbol
		if seed.Loc.File != "" {
			sreqs = append(sreqs, snippets.SnippetRequest{
				File:      seed.Loc.File,
				StartLine: seed.Loc.StartLine,
				EndLine:   seed.Loc.EndLine,
				Label:     "seed",
				Priority:  10,
			})
		}

		// Top 3 callers (by first occurrence).
		callerCount := 0
		seenCallerFile := map[string]bool{}
		for _, e := range pack.Calls {
			if e.ToSymbolID != nil && *e.ToSymbolID == pack.SeedID {
				if callerCount >= 3 {
					break
				}
				if seenCallerFile[e.Loc.File] {
					continue
				}
				seenCallerFile[e.Loc.File] = true
				sreqs = append(sreqs, snippets.SnippetRequest{
					File:     e.Loc.File,
					Line:     e.Loc.Line,
					Label:    "caller",
					Priority: 5,
				})
				callerCount++
			}
		}

		// Top 3 callees.
		calleeCount := 0
		seenCalleeFile := map[string]bool{}
		for _, e := range pack.Calls {
			if e.FromSymbolID == pack.SeedID {
				if calleeCount >= 3 {
					break
				}
				if seenCalleeFile[e.Loc.File] {
					continue
				}
				seenCalleeFile[e.Loc.File] = true
				sreqs = append(sreqs, snippets.SnippetRequest{
					File:     e.Loc.File,
					Line:     e.Loc.Line,
					Label:    "callee",
					Priority: 3,
				})
				calleeCount++
			}
		}

		snipBudget := budgetChars
		if snipBudget > 9000 {
			snipBudget = 9000
		}
		blocks, snipText, _ := snippets.Build(sreqs, snippets.Options{
			RepoRoot:    repoRoot,
			BudgetChars: snipBudget,
		})
		result["snippetsText"] = snipText
		result["snippetsBlocks"] = blocks
	}

	return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: result}
}

// handleSnippets extracts code snippets for an explicit list of locations.
func handleSnippets(st *State, req proto.Request) proto.Response {
	st.mu.RLock()
	repoRoot := st.repoRoot
	st.mu.RUnlock()

	budgetChars := 8000
	if b, ok := req.Params["budgetChars"].(float64); ok {
		budgetChars = int(b)
	}

	// Decode locs from params["locs"] — a JSON array of SnippetRequest objects.
	var sreqs []snippets.SnippetRequest
	if raw, ok := req.Params["locs"]; ok {
		b, err := json.Marshal(raw)
		if err == nil {
			_ = json.Unmarshal(b, &sreqs)
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
		"blocks": blocks,
		"text":   text,
	}}
}
