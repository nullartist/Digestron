package serve

import (
	"time"

	"github.com/nullartist/digestron/internal/proto"
	"github.com/nullartist/digestron/internal/usg"
)

// handleEnsureIndexed implements the "ensureIndexed" op (v0.28).
//
// Logic:
//  1. USG in memory → return source=memory
//  2. USG on disk → check freshness
//     - fresh  → return source=disk
//     - stale && autoIndex && reindexIfStale → fall-through to fresh index
//     - stale otherwise → return STALE_INDEX error with freshness details
//  3. No USG anywhere
//     - autoIndex=true → run index, return source=fresh-index
//     - else            → return NOT_INDEXED error
func handleEnsureIndexed(st *State, req proto.Request) proto.Response {
	repoAbs, err := resolveRepo(st, req.Params)
	if err != nil {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "BAD_REPO", Message: err.Error()}}
	}

	// Parse optional params.
	autoIndex := true
	if v, ok := req.Params["autoIndex"].(bool); ok {
		autoIndex = v
	}
	reindexIfStale := autoIndex
	if v, ok := req.Params["reindexIfStale"].(bool); ok {
		reindexIfStale = v
	}
	includeJS := false
	if v, ok := req.Params["includeJS"].(bool); ok {
		includeJS = v
	}
	includeTests := false
	if v, ok := req.Params["includeTests"].(bool); ok {
		includeTests = v
	}

	// ── 1. USG already in memory (truly warm from a previous request) ────────
	st.mu.RLock()
	existingRS, alreadyInMemory := st.repos[repoAbs]
	usgInMemory := alreadyInMemory && existingRS.USG != nil
	st.mu.RUnlock()

	if usgInMemory {
		st.mu.Lock()
		existingRS.LastUsedAt = time.Now()
		g := existingRS.USG
		st.mu.Unlock()
		return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: map[string]any{
			"repoRoot": repoAbs,
			"indexed":  true,
			"source":   "memory",
			"stats":    g.Stats,
		}}
	}

	rs, _ := st.getOrWarm(repoAbs)
	_ = rs // used below via state mutations

	// ── 2. USG exists on disk ──────────────────────────────────────────────
	if g, err := usg.Load(repoAbs); err == nil && g != nil {
		fresh, _ := CheckRepoFreshness(repoAbs, FreshnessOptions{
			IncludeJS:    includeJS,
			IncludeTests: includeTests,
			MaxFiles:     300000,
		})

		if fresh.IsStale {
			if !autoIndex || !reindexIfStale {
				return proto.Response{V: req.V, ID: req.ID, Ok: false,
					Error: &proto.ErrorObj{
						Code:    "STALE_INDEX",
						Message: "USG exists but is stale. Reindex required or call ensureIndexed with autoIndex=true and reindexIfStale=true",
					},
					Result: map[string]any{
						"freshness": fresh,
					}}
			}
			// Fall-through: trigger a fresh index below.
		} else {
			// Fresh: load into memory and return.
			st.mu.Lock()
			rs.USG = g
			rs.View = usg.BuildView(g)
			rs.LastUsedAt = time.Now()
			st.mu.Unlock()
			return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: map[string]any{
				"repoRoot":  repoAbs,
				"indexed":   true,
				"source":    "disk",
				"stats":     g.Stats,
				"freshness": fresh,
			}}
		}
	}

	// ── 3. No USG (or stale fall-through) ─────────────────────────────────
	if !autoIndex {
		return proto.Response{V: req.V, ID: req.ID, Ok: false,
			Error: &proto.ErrorObj{Code: "NOT_INDEXED", Message: "not indexed; call with autoIndex=true to index automatically"}}
	}

	// Run a fresh index.
	indexResp := handleIndex(st, req)
	if !indexResp.Ok {
		return indexResp
	}

	// Collect freshness for the response (best-effort).
	fresh, _ := CheckRepoFreshness(repoAbs, FreshnessOptions{
		IncludeJS:    includeJS,
		IncludeTests: includeTests,
		MaxFiles:     300000,
	})

	result := map[string]any{
		"repoRoot":  repoAbs,
		"indexed":   true,
		"source":    "fresh-index",
		"freshness": fresh,
	}
	// Merge stats from the index response when available.
	if m, ok := indexResp.Result.(map[string]any); ok {
		for _, k := range []string{"modules", "symbols", "calls", "cached"} {
			if v, ok := m[k]; ok {
				result[k] = v
			}
		}
	}

	return proto.Response{V: req.V, ID: req.ID, Ok: true, Result: result}
}
