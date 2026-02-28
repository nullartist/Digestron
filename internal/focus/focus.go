// Package focus implements the USG v0.1 Focus Pack algorithm.
//
// It builds a compact "impact slice" around a seed symbol by BFS-traversing the
// call graph up to a given radius, then formats the result as both plain text and
// a JSON subgraph — all within a character budget.
package focus

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nullartist/digestron/internal/usg"
)

const (
	// riskFlagLineRadius is the number of lines around the seed's location
	// within which risk flags are considered "nearby".
	riskFlagLineRadius = 30

	// maxListItems is the default display cap for callers, callees, and
	// instantiation sites in the text output.
	maxListItems = 10
)

// Options controls Focus Pack generation.
type Options struct {
	Radius      int // BFS hops from seed (default 2)
	BudgetChars int // max characters for text output (default 8000)
	MaxFiles    int // max distinct files in text output (default 12)
}

func (o *Options) defaults() {
	if o.Radius == 0 {
		o.Radius = 2
	}
	if o.BudgetChars == 0 {
		o.BudgetChars = 8000
	}
	if o.MaxFiles == 0 {
		o.MaxFiles = 12
	}
}

// Pack is the computed focus pack.
type Pack struct {
	SeedID       string
	SeedSymbol   *usg.Symbol
	Callers      []usg.Symbol
	Callees      []usg.Symbol
	Inherits     []usg.Symbol // parent symbols
	Instantiates []usg.InstantiationSite
	RiskFlags    []usg.RiskFlag
	EntryPoints  []usg.EntryPoint

	// subgraph for JSON output
	Symbols []usg.Symbol
	Calls   []usg.CallEdge
}

// Build computes a Focus Pack for seedID from the given graph.
func Build(graph *usg.USG, seedID string, opts Options) (*Pack, error) {
	opts.defaults()

	symByID := make(map[string]*usg.Symbol, len(graph.Symbols))
	for i := range graph.Symbols {
		symByID[graph.Symbols[i].ID] = &graph.Symbols[i]
	}

	seed := symByID[seedID]
	if seed == nil {
		return nil, fmt.Errorf("focus: seed symbol %q not found", seedID)
	}

	pack := &Pack{SeedID: seedID, SeedSymbol: seed}

	// --- Direct callers (symbols that call the seed) ---
	callerSet := make(map[string]bool)
	for _, call := range graph.Edges.Calls {
		if call.ToSymbolID != nil && *call.ToSymbolID == seedID {
			callerSet[call.FromSymbolID] = true
		}
	}
	for id := range callerSet {
		if s := symByID[id]; s != nil {
			pack.Callers = append(pack.Callers, *s)
		}
	}

	// --- Direct callees (symbols that the seed calls) ---
	calleeSet := make(map[string]bool)
	for _, call := range graph.Edges.Calls {
		if call.FromSymbolID == seedID && call.ToSymbolID != nil {
			calleeSet[*call.ToSymbolID] = true
		}
	}
	for id := range calleeSet {
		if s := symByID[id]; s != nil {
			pack.Callees = append(pack.Callees, *s)
		}
	}

	// --- Inheritance: parent symbols of seed ---
	for _, inh := range graph.Edges.Inherits {
		if inh.ChildSymbolID == seedID {
			if s := symByID[inh.ParentSymbolID]; s != nil {
				pack.Inherits = append(pack.Inherits, *s)
			}
		}
	}

	// --- Instantiation sites of the seed ---
	for _, inst := range graph.Edges.Instantiates {
		if inst.SymbolID == seedID {
			pack.Instantiates = append(pack.Instantiates, inst)
		}
	}

	// --- Risk flags in same file or within riskFlagLineRadius lines of seed ---
	if seed.Loc.File != "" {
		for _, rf := range graph.RiskFlags {
			if rf.Loc.File == seed.Loc.File {
				lineClose := seed.Loc.StartLine == 0 ||
					(rf.Loc.Line >= seed.Loc.StartLine-riskFlagLineRadius && rf.Loc.Line <= seed.Loc.EndLine+riskFlagLineRadius)
				if lineClose {
					pack.RiskFlags = append(pack.RiskFlags, rf)
				}
			}
		}
	}

	// --- Connected entry points (BFS inverse, limited to radius) ---
	reachable := bfsCallers(graph, seedID, opts.Radius)
	epFiles := make(map[string]bool)
	for _, ep := range graph.EntryPoints {
		epFiles[ep.File] = true
	}
	for _, sym := range reachable {
		for _, ep := range graph.EntryPoints {
			if ep.File == sym.Loc.File || (ep.SymbolID != nil && *ep.SymbolID == sym.ID) {
				pack.EntryPoints = append(pack.EntryPoints, ep)
				break
			}
		}
	}

	// --- Build subgraph (seed + callers + callees) ---
	subSymIDs := make(map[string]bool)
	subSymIDs[seedID] = true
	for _, s := range pack.Callers {
		subSymIDs[s.ID] = true
	}
	for _, s := range pack.Callees {
		subSymIDs[s.ID] = true
	}
	for _, s := range pack.Inherits {
		subSymIDs[s.ID] = true
	}
	for id := range subSymIDs {
		if s := symByID[id]; s != nil {
			pack.Symbols = append(pack.Symbols, *s)
		}
	}
	for _, call := range graph.Edges.Calls {
		if subSymIDs[call.FromSymbolID] && (call.ToSymbolID == nil || subSymIDs[*call.ToSymbolID]) {
			pack.Calls = append(pack.Calls, call)
		}
	}

	return pack, nil
}

// bfsCallers returns symbols reachable by inverse-call BFS up to maxHops.
func bfsCallers(graph *usg.USG, seedID string, maxHops int) []usg.Symbol {
	symByID := make(map[string]*usg.Symbol, len(graph.Symbols))
	for i := range graph.Symbols {
		symByID[graph.Symbols[i].ID] = &graph.Symbols[i]
	}

	visited := map[string]bool{seedID: true}
	frontier := []string{seedID}

	for hop := 0; hop < maxHops && len(frontier) > 0; hop++ {
		var next []string
		for _, id := range frontier {
			for _, call := range graph.Edges.Calls {
				if call.ToSymbolID != nil && *call.ToSymbolID == id && !visited[call.FromSymbolID] {
					visited[call.FromSymbolID] = true
					next = append(next, call.FromSymbolID)
				}
			}
		}
		frontier = next
	}

	var result []usg.Symbol
	for id := range visited {
		if id == seedID {
			continue
		}
		if s := symByID[id]; s != nil {
			result = append(result, *s)
		}
	}
	return result
}

// FormatText renders the Focus Pack as a compact plain-text block
// trimmed to the character budget.
func FormatText(pack *Pack, graph *usg.USG, opts Options) string {
	opts.defaults()

	var sb strings.Builder
	seedName := pack.SeedSymbol.QName
	fmt.Fprintf(&sb, "=== Focus Pack: %s (radius=%d) ===\n", seedName, opts.Radius)
	fmt.Fprintf(&sb, "structuralConfidence: %.2f\n\n", graph.Stats.StructuralConfidence)

	// SEED
	fmt.Fprintf(&sb, "[SEED] %s  [%s]\n", pack.SeedSymbol.QName, pack.SeedSymbol.Kind)
	if pack.SeedSymbol.Signature != "" {
		fmt.Fprintf(&sb, "  signature: %s\n", pack.SeedSymbol.Signature)
	}
	fmt.Fprintf(&sb, "  loc: %s:%d\n\n", pack.SeedSymbol.Loc.File, pack.SeedSymbol.Loc.StartLine)

	// CALLERS
	fmt.Fprintf(&sb, "[CALLERS] %d found:\n", len(pack.Callers))
	for i, s := range pack.Callers {
		if i >= maxListItems {
			fmt.Fprintf(&sb, "  ...+%d more\n", len(pack.Callers)-maxListItems)
			break
		}
		fmt.Fprintf(&sb, "  %s  [%s]  @%s:%d\n", s.QName, s.Kind, s.Loc.File, s.Loc.StartLine)
	}
	sb.WriteByte('\n')

	// CALLEES
	fmt.Fprintf(&sb, "[CALLEES] %d found:\n", len(pack.Callees))
	for i, s := range pack.Callees {
		if i >= maxListItems {
			fmt.Fprintf(&sb, "  ...+%d more\n", len(pack.Callees)-maxListItems)
			break
		}
		fmt.Fprintf(&sb, "  %s  [%s]  @%s:%d\n", s.QName, s.Kind, s.Loc.File, s.Loc.StartLine)
	}
	sb.WriteByte('\n')

	// INHERITS
	if len(pack.Inherits) > 0 {
		fmt.Fprintf(&sb, "[INHERITS] %d parent(s):\n", len(pack.Inherits))
		for _, s := range pack.Inherits {
			fmt.Fprintf(&sb, "  %s  [%s]\n", s.QName, s.Kind)
		}
		sb.WriteByte('\n')
	}

	// INSTANTIATIONS
	if len(pack.Instantiates) > 0 {
		fmt.Fprintf(&sb, "[INSTANTIATIONS] %d site(s):\n", len(pack.Instantiates))
		for i, inst := range pack.Instantiates {
			if i >= maxListItems {
				fmt.Fprintf(&sb, "  ...+%d more\n", len(pack.Instantiates)-maxListItems)
				break
			}
			fmt.Fprintf(&sb, "  new [%s]  @%s:%d  confidence=%s\n",
				pack.SeedSymbol.Name, inst.Loc.File, inst.Loc.Line, inst.Confidence)
		}
		sb.WriteByte('\n')
	}

	// ENTRY POINTS
	if len(pack.EntryPoints) > 0 {
		seen := make(map[string]bool)
		fmt.Fprintf(&sb, "[ENTRY POINTS] reachable:\n")
		for _, ep := range pack.EntryPoints {
			if seen[ep.File] {
				continue
			}
			seen[ep.File] = true
			fmt.Fprintf(&sb, "  [%s] %s\n", ep.Kind, ep.File)
		}
		sb.WriteByte('\n')
	}

	// RISK FLAGS
	fmt.Fprintf(&sb, "[RISK FLAGS] %d nearby\n", len(pack.RiskFlags))
	for _, rf := range pack.RiskFlags {
		fmt.Fprintf(&sb, "  [%s] @%s:%d — %s\n", rf.Kind, rf.Loc.File, rf.Loc.Line, rf.Note)
	}

	result := sb.String()

	// budget trim: record actual content length before truncation
	contentLen := len(result)
	if opts.BudgetChars > 0 && contentLen > opts.BudgetChars {
		result = result[:opts.BudgetChars] + "\n... (truncated to budget)\n"
	}

	result += fmt.Sprintf("\nbudget: %d/%d chars used\n", contentLen, opts.BudgetChars)
	return result
}

// SubGraph is the JSON representation of the focus pack subgraph.
type SubGraph struct {
	SeedID       string              `json:"seedId"`
	Symbols      []usg.Symbol        `json:"symbols"`
	Calls        []usg.CallEdge      `json:"calls"`
	Instantiates []usg.InstantiationSite `json:"instantiates"`
	RiskFlags    []usg.RiskFlag      `json:"riskFlags"`
	EntryPoints  []usg.EntryPoint    `json:"entryPoints"`
}

// FormatJSON renders the Focus Pack as a JSON subgraph.
func FormatJSON(pack *Pack) ([]byte, error) {
	sg := SubGraph{
		SeedID:       pack.SeedID,
		Symbols:      pack.Symbols,
		Calls:        pack.Calls,
		Instantiates: pack.Instantiates,
		RiskFlags:    pack.RiskFlags,
		EntryPoints:  pack.EntryPoints,
	}
	return json.MarshalIndent(sg, "", "  ")
}
