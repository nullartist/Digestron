package focus

import (
	"sort"

	"github.com/nullartist/digestron/internal/snippets"
	"github.com/nullartist/digestron/internal/usg"
)

// FocusPackJSON is the stable v0.25 JSON payload returned by the impact handler
// and emitted by the CLI --json flag.
type FocusPackJSON struct {
	Version   string           `json:"version"` // "focus.v0.25"
	RepoRoot  string           `json:"repoRoot"`
	Seed      FocusSeed        `json:"seed"`
	Summary   FocusSummary     `json:"summary"`
	Relations FocusRelations   `json:"relations"`
	Snippets  *SnippetsPayload `json:"snippets,omitempty"`
	Stats     usg.Stats        `json:"stats"`
}

// FocusSeed identifies the seed symbol.
type FocusSeed struct {
	SymbolID  string       `json:"symbolId"`
	QName     string       `json:"qname"`
	Kind      string       `json:"kind"`
	Signature string       `json:"signature"`
	Loc       usg.Location `json:"loc"`
}

// FocusSummary summarises the query parameters and relation counts.
type FocusSummary struct {
	Radius        int `json:"radius"`
	BudgetChars   int `json:"budgetChars"`
	MaxFiles      int `json:"maxFiles"`
	CallersCount  int `json:"callersCount"`
	CalleesCount  int `json:"calleesCount"`
	InstCount     int `json:"instantiationsCount"`
	InheritsCount int `json:"inheritsCount"`
	RisksCount    int `json:"risksCount"`
}

// FocusRelations holds the bounded, deterministic relation slices.
type FocusRelations struct {
	Callers        []usg.CallEdge          `json:"callers"`
	Callees        []usg.CallEdge          `json:"callees"`
	Instantiations []usg.InstantiationSite `json:"instantiations"`
	Inherits       []usg.InheritanceEdge   `json:"inherits"`
	RiskFlags      []usg.RiskFlag          `json:"riskFlags"`
}

// SnippetsPayload carries extracted code snippets within a character budget.
type SnippetsPayload struct {
	BudgetChars int               `json:"budgetChars"`
	Blocks      []SnippetBlockJSON `json:"blocks"`
	Text        string            `json:"text"`
}

// SnippetBlockJSON is the JSON-serialisable form of a single snippet block.
type SnippetBlockJSON struct {
	File      string `json:"file"`
	StartLine int    `json:"startLine"`
	EndLine   int    `json:"endLine"`
	Label     string `json:"label,omitempty"`
	Text      string `json:"text"`
}

// BuildPackJSON constructs a FocusPackJSON from a computed Pack and its graph/view.
// callerLimit and calleeLimit bound the relation slices (0 → defaults of 60/60).
func BuildPackJSON(pack *Pack, graph *usg.USG, view *usg.View, repoRoot string, radius, budget int) FocusPackJSON {
	seed := pack.SeedSymbol

	callers := view.CallersByTo[seed.ID]
	callees := view.CalleesByFrom[seed.ID]
	if len(callers) > 60 {
		callers = callers[:60]
	}
	if len(callees) > 60 {
		callees = callees[:60]
	}

	var insts []usg.InstantiationSite
	for _, it := range graph.Edges.Instantiates {
		if it.SymbolID == seed.ID {
			insts = append(insts, it)
			if len(insts) >= 40 {
				break
			}
		}
	}

	var inh []usg.InheritanceEdge
	for _, e := range graph.Edges.Inherits {
		if e.ChildSymbolID == seed.ID {
			inh = append(inh, e)
			if len(inh) >= 20 {
				break
			}
		}
	}

	var risks []usg.RiskFlag
	for _, r := range graph.RiskFlags {
		lo := seed.Loc.StartLine - 30
		hi := seed.Loc.StartLine + 30
		if r.Loc.File == seed.Loc.File && r.Loc.Line >= lo && r.Loc.Line <= hi {
			risks = append(risks, r)
			if len(risks) >= 20 {
				break
			}
		}
	}

	// Stable sort by file then line for deterministic output.
	sort.Slice(callers, func(i, j int) bool {
		if callers[i].Loc.File != callers[j].Loc.File {
			return callers[i].Loc.File < callers[j].Loc.File
		}
		return callers[i].Loc.Line < callers[j].Loc.Line
	})
	sort.Slice(callees, func(i, j int) bool {
		if callees[i].Loc.File != callees[j].Loc.File {
			return callees[i].Loc.File < callees[j].Loc.File
		}
		return callees[i].Loc.Line < callees[j].Loc.Line
	})

	if insts == nil {
		insts = []usg.InstantiationSite{}
	}
	if inh == nil {
		inh = []usg.InheritanceEdge{}
	}
	if risks == nil {
		risks = []usg.RiskFlag{}
	}

	return FocusPackJSON{
		Version:  "focus.v0.25",
		RepoRoot: repoRoot,
		Seed: FocusSeed{
			SymbolID:  seed.ID,
			QName:     seed.QName,
			Kind:      seed.Kind,
			Signature: seed.Signature,
			Loc:       seed.Loc,
		},
		Summary: FocusSummary{
			Radius:        radius,
			BudgetChars:   budget,
			MaxFiles:      12,
			CallersCount:  len(callers),
			CalleesCount:  len(callees),
			InstCount:     len(insts),
			InheritsCount: len(inh),
			RisksCount:    len(risks),
		},
		Relations: FocusRelations{
			Callers:        callers,
			Callees:        callees,
			Instantiations: insts,
			Inherits:       inh,
			RiskFlags:      risks,
		},
		Stats: graph.Stats,
	}
}

// BuildSnippetRequests builds SnippetRequests for the seed, top callers, callees,
// and risks from a FocusPackJSON. topCallers and topCallees bound the number of
// caller/callee edges included (0 → default of 3 each).
func BuildSnippetRequests(fp FocusPackJSON, topCallers, topCallees int) []snippets.SnippetRequest {
	if topCallers <= 0 {
		topCallers = 3
	}
	if topCallees <= 0 {
		topCallees = 3
	}

	var reqs []snippets.SnippetRequest

	if fp.Seed.Loc.File != "" {
		reqs = append(reqs, snippets.SnippetRequest{
			File: fp.Seed.Loc.File, StartLine: fp.Seed.Loc.StartLine, EndLine: fp.Seed.Loc.EndLine,
			ContextLines: 12, Label: "seed", Priority: 100,
		})
	}
	for i, c := range fp.Relations.Callers {
		if i >= topCallers {
			break
		}
		reqs = append(reqs, snippets.SnippetRequest{
			File: c.Loc.File, Line: c.Loc.Line, ContextLines: 18, Label: "caller", Priority: 70,
		})
	}
	for i, c := range fp.Relations.Callees {
		if i >= topCallees {
			break
		}
		reqs = append(reqs, snippets.SnippetRequest{
			File: c.Loc.File, Line: c.Loc.Line, ContextLines: 18, Label: "callee", Priority: 60,
		})
	}
	for i, r := range fp.Relations.RiskFlags {
		if i >= 2 {
			break
		}
		reqs = append(reqs, snippets.SnippetRequest{
			File: r.Loc.File, Line: r.Loc.Line, ContextLines: 10, Label: "risk", Priority: 40,
		})
	}
	return reqs
}
