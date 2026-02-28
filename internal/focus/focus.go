// Package focus implements the USG v0.15 Focus Pack algorithm.
//
// BuildFocusPack builds a compact "impact slice" around a seed symbol using
// O(1) index lookups via usg.View, groups callers/callees by file, and
// formats the result within a character budget.
package focus

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nullartist/digestron/internal/usg"
)

// Options controls Focus Pack generation.
type Options struct {
	Radius             int // BFS hops from seed (default 2; reserved for future use)
	BudgetChars        int // max characters for text output (default 8000)
	MaxFiles           int // max distinct files in grouped output (default 12)
	MaxLinesPerFile    int // max lines shown per file in grouped output (default 6)
	MaxItemsPerSection int // max edges/items collected per section (default 40)
}

func (o *Options) defaults() {
	if o.Radius <= 0 {
		o.Radius = 2
	}
	if o.BudgetChars <= 0 {
		o.BudgetChars = 8000
	}
	if o.MaxFiles <= 0 {
		o.MaxFiles = 12
	}
	if o.MaxLinesPerFile <= 0 {
		o.MaxLinesPerFile = 6
	}
	if o.MaxItemsPerSection <= 0 {
		o.MaxItemsPerSection = 40
	}
}

// Pack is the computed focus pack.
type Pack struct {
	SeedID     string
	SeedSymbol usg.Symbol
	Text       string

	// subgraph retained for JSON output
	Symbols      []usg.Symbol
	Calls        []usg.CallEdge
	Instantiates []usg.InstantiationSite
	RiskFlags    []usg.RiskFlag
	EntryPoints  []usg.EntryPoint
}

// Build computes a Focus Pack for seedID from the given graph.
// seedID may be a symbol ID or qualified name.
func Build(graph *usg.USG, seedID string, opts Options) (*Pack, error) {
	opts.defaults()

	v := usg.BuildView(graph)

	seed, ok := resolveSymbol(v, seedID)
	if !ok {
		return nil, fmt.Errorf("focus: seed symbol %q not found", seedID)
	}

	callerEdges := callersOf(v, seed.ID, opts.MaxItemsPerSection)
	calleeEdges := calleesOf(v, seed.ID, opts.MaxItemsPerSection)
	inheritEdges := inheritanceOf(v, seed.ID, opts.MaxItemsPerSection)
	insts := instantiationsOf(v, seed.ID, opts.MaxItemsPerSection)
	risks := riskFlagsNear(graph, seed.Loc.File, seed.Loc.StartLine, 30, opts.MaxItemsPerSection)

	// Build subgraph symbol set
	subSymIDs := map[string]bool{seed.ID: true}
	for _, e := range callerEdges {
		subSymIDs[e.FromSymbolID] = true
	}
	for _, e := range calleeEdges {
		if e.ToSymbolID != nil {
			subSymIDs[*e.ToSymbolID] = true
		}
	}
	for _, e := range inheritEdges {
		subSymIDs[e.ParentSymbolID] = true
	}
	var subSymbols []usg.Symbol
	for id := range subSymIDs {
		if s, ok := v.SymbolByID[id]; ok {
			subSymbols = append(subSymbols, s)
		}
	}
	var subCalls []usg.CallEdge
	for _, call := range graph.Edges.Calls {
		if subSymIDs[call.FromSymbolID] && (call.ToSymbolID == nil || subSymIDs[*call.ToSymbolID]) {
			subCalls = append(subCalls, call)
		}
	}

	// Connected entry points (BFS inverse, limited to radius)
	reachable := bfsCallers(v, seed.ID, opts.Radius)
	var entryPoints []usg.EntryPoint
	epSeen := map[string]bool{}
	for _, sym := range reachable {
		for _, ep := range graph.EntryPoints {
			if !epSeen[ep.File] && (ep.File == sym.Loc.File || (ep.SymbolID != nil && *ep.SymbolID == sym.ID)) {
				epSeen[ep.File] = true
				entryPoints = append(entryPoints, ep)
				break
			}
		}
	}

	// Build text output with budget
	text := buildText(v, graph, seed, callerEdges, calleeEdges, inheritEdges, insts, risks, entryPoints, opts)

	return &Pack{
		SeedID:       seed.ID,
		SeedSymbol:   seed,
		Text:         text,
		Symbols:      subSymbols,
		Calls:        subCalls,
		Instantiates: insts,
		RiskFlags:    risks,
		EntryPoints:  entryPoints,
	}, nil
}

// buildText assembles the text representation of the focus pack within the budget.
func buildText(v *usg.View, graph *usg.USG, seed usg.Symbol,
	callerEdges, calleeEdges []usg.CallEdge,
	inheritEdges []usg.InheritanceEdge,
	insts []usg.InstantiationSite,
	risks []usg.RiskFlag,
	entryPoints []usg.EntryPoint,
	opts Options,
) string {
	var b strings.Builder
	budget := opts.BudgetChars

	write := func(s string) bool {
		if budget > 0 && b.Len()+len(s) > budget {
			return false
		}
		b.WriteString(s)
		return true
	}

	write(fmt.Sprintf("SYMBOL: %s [%s]\n", seed.QName, seed.Kind))
	write(fmt.Sprintf("SIGNATURE: %s\n", orNA(seed.Signature)))
	write(fmt.Sprintf("LOC: %s:%d-%d\n\n", seed.Loc.File, seed.Loc.StartLine, seed.Loc.EndLine))

	if len(callerEdges) > 0 {
		write("CALLED BY:\n")
		writeGroupedEdges(&b, v, callerEdges, opts, false)
		write("\n")
	}

	if len(calleeEdges) > 0 {
		write("CALLS:\n")
		writeGroupedEdges(&b, v, calleeEdges, opts, true)
		write("\n")
	}

	if len(insts) > 0 {
		write("INSTANTIATIONS:\n")
		writeGroupedLocations(&b, insts, opts)
		write("\n")
	}

	if len(inheritEdges) > 0 {
		write("INHERITANCE:\n")
		for i, e := range inheritEdges {
			if i >= opts.MaxItemsPerSection {
				break
			}
			parent := shortSymbolQName(v, e.ParentSymbolID)
			if !write(fmt.Sprintf("  - %s (%s)\n", parent, e.Confidence)) {
				break
			}
		}
		write("\n")
	}

	if len(entryPoints) > 0 {
		write("ENTRY POINTS:\n")
		seen := map[string]bool{}
		for _, ep := range entryPoints {
			if seen[ep.File] {
				continue
			}
			seen[ep.File] = true
			if !write(fmt.Sprintf("  [%s] %s\n", ep.Kind, ep.File)) {
				break
			}
		}
		write("\n")
	}

	if len(risks) > 0 {
		write("RISK:\n")
		for i, r := range risks {
			if i >= opts.MaxItemsPerSection {
				break
			}
			if !write(fmt.Sprintf("  - %s:%d %s (%s)\n", r.Loc.File, r.Loc.Line, r.Kind, r.Note)) {
				break
			}
		}
		write("\n")
	}

	write(fmt.Sprintf("STRUCTURAL CONFIDENCE (repo): %.2f\n", graph.Stats.StructuralConfidence))

	result := b.String()
	contentLen := len(result)
	if budget > 0 && contentLen > budget {
		result = result[:budget] + "\n... (truncated to budget)\n"
	}
	result += fmt.Sprintf("\nbudget: %d/%d chars used\n", contentLen, budget)
	return result
}

// bfsCallers returns symbols reachable by inverse-call BFS up to maxHops.
func bfsCallers(v *usg.View, seedID string, maxHops int) []usg.Symbol {
	visited := map[string]bool{seedID: true}
	frontier := []string{seedID}

	for hop := 0; hop < maxHops && len(frontier) > 0; hop++ {
		var next []string
		for _, id := range frontier {
			for _, call := range v.CallersByTo[id] {
				if !visited[call.FromSymbolID] {
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
		if s, ok := v.SymbolByID[id]; ok {
			result = append(result, s)
		}
	}
	return result
}

// --- grouping helpers ---

type fileLines struct {
	File  string
	Lines []int
	Items []string
}

func writeGroupedEdges(b *strings.Builder, v *usg.View, edges []usg.CallEdge, opt Options, includeTarget bool) {
	m := map[string]*fileLines{}
	order := []string{}

	for _, e := range edges {
		f := e.Loc.File
		if _, ok := m[f]; !ok {
			m[f] = &fileLines{File: f}
			order = append(order, f)
		}
		m[f].Lines = append(m[f].Lines, e.Loc.Line)

		if includeTarget {
			target := "<unknown>"
			if e.ToSymbolID != nil {
				target = shortSymbolQName(v, *e.ToSymbolID)
			} else if e.ToExternal != nil {
				target = fmt.Sprintf("%s.%s", e.ToExternal.Module, e.ToExternal.Name)
			}
			m[f].Items = append(m[f].Items, fmt.Sprintf("%d -> %s (%s)", e.Loc.Line, target, e.Confidence))
		} else {
			m[f].Items = append(m[f].Items, fmt.Sprintf("%d (%s)", e.Loc.Line, e.Confidence))
		}
	}

	sort.SliceStable(order, func(i, j int) bool {
		ai := len(m[order[i]].Lines)
		aj := len(m[order[j]].Lines)
		if ai != aj {
			return ai > aj
		}
		return order[i] < order[j]
	})

	filesShown := 0
	for _, f := range order {
		if filesShown >= opt.MaxFiles {
			fmt.Fprintf(b, "  - +%d more files\n", len(order)-filesShown)
			return
		}
		filesShown++
		fl := m[f]
		fmt.Fprintf(b, "  - %s:\n", fl.File)

		printed := 0
		for _, item := range fl.Items {
			if printed >= opt.MaxLinesPerFile {
				break
			}
			fmt.Fprintf(b, "      - %s\n", item)
			printed++
		}
		if remaining := len(fl.Items) - printed; remaining > 0 {
			fmt.Fprintf(b, "      - +%d more\n", remaining)
		}
	}
}

func writeGroupedLocations(b *strings.Builder, insts []usg.InstantiationSite, opt Options) {
	m := map[string]*fileLines{}
	order := []string{}

	for _, i := range insts {
		f := i.Loc.File
		if _, ok := m[f]; !ok {
			m[f] = &fileLines{File: f}
			order = append(order, f)
		}
		m[f].Lines = append(m[f].Lines, i.Loc.Line)
		m[f].Items = append(m[f].Items, fmt.Sprintf("%d (%s)", i.Loc.Line, i.Confidence))
	}

	sort.SliceStable(order, func(i, j int) bool {
		ai := len(m[order[i]].Lines)
		aj := len(m[order[j]].Lines)
		if ai != aj {
			return ai > aj
		}
		return order[i] < order[j]
	})

	filesShown := 0
	for _, f := range order {
		if filesShown >= opt.MaxFiles {
			fmt.Fprintf(b, "  - +%d more files\n", len(order)-filesShown)
			return
		}
		filesShown++
		fl := m[f]
		fmt.Fprintf(b, "  - %s:\n", fl.File)

		printed := 0
		for _, item := range fl.Items {
			if printed >= opt.MaxLinesPerFile {
				break
			}
			fmt.Fprintf(b, "      - %s\n", item)
			printed++
		}
		if remaining := len(fl.Items) - printed; remaining > 0 {
			fmt.Fprintf(b, "      - +%d more\n", remaining)
		}
	}
}

// --- data access helpers (via View) ---

func resolveSymbol(v *usg.View, ref string) (usg.Symbol, bool) {
	if s, ok := v.SymbolByID[ref]; ok {
		return s, true
	}
	if s, ok := v.SymbolByQName[ref]; ok {
		return s, true
	}
	return usg.Symbol{}, false
}

func callersOf(v *usg.View, symbolID string, limit int) []usg.CallEdge {
	edges := v.CallersByTo[symbolID]
	if len(edges) > limit {
		return edges[:limit]
	}
	return edges
}

func calleesOf(v *usg.View, symbolID string, limit int) []usg.CallEdge {
	edges := v.CalleesByFrom[symbolID]
	if len(edges) > limit {
		return edges[:limit]
	}
	return edges
}

func inheritanceOf(v *usg.View, childID string, limit int) []usg.InheritanceEdge {
	var out []usg.InheritanceEdge
	for _, e := range v.Graph.Edges.Inherits {
		if e.ChildSymbolID == childID {
			out = append(out, e)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func instantiationsOf(v *usg.View, symbolID string, limit int) []usg.InstantiationSite {
	var out []usg.InstantiationSite
	for _, i := range v.Graph.Edges.Instantiates {
		if i.SymbolID == symbolID {
			out = append(out, i)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func shortSymbolQName(v *usg.View, id string) string {
	if s, ok := v.SymbolByID[id]; ok {
		return s.QName
	}
	return id
}

func riskFlagsNear(g *usg.USG, file string, line, window, limit int) []usg.RiskFlag {
	var out []usg.RiskFlag
	min := line - window
	max := line + window
	for _, r := range g.RiskFlags {
		if r.Loc.File != file {
			continue
		}
		if r.Loc.Line >= min && r.Loc.Line <= max {
			out = append(out, r)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}

func orNA(s string) string {
	if strings.TrimSpace(s) == "" {
		return "n/a"
	}
	return s
}

// SubGraph is the JSON representation of the focus pack subgraph.
type SubGraph struct {
	SeedID       string                  `json:"seedId"`
	Symbols      []usg.Symbol            `json:"symbols"`
	Calls        []usg.CallEdge          `json:"calls"`
	Instantiates []usg.InstantiationSite `json:"instantiates"`
	RiskFlags    []usg.RiskFlag          `json:"riskFlags"`
	EntryPoints  []usg.EntryPoint        `json:"entryPoints"`
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

