package focus

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nullartist/digestron/internal/usg"
)

// helpers

func strPtr(s string) *string { return &s }

func makeUSG(syms []usg.Symbol, calls []usg.CallEdge) *usg.USG {
	return &usg.USG{
		Symbols: syms,
		Edges:   usg.Edges{Calls: calls},
		Stats:   usg.Stats{StructuralConfidence: 0.85},
	}
}

func sym(id, qname, name, kind, file string, line int) usg.Symbol {
	return usg.Symbol{
		ID:    id,
		QName: qname,
		Name:  name,
		Kind:  kind,
		Loc:   usg.Location{File: file, StartLine: line},
	}
}

// ---- Build() tests ----

func TestBuild_SeedNotFound(t *testing.T) {
	g := makeUSG(nil, nil)
	_, err := Build(g, "sym_missing", Options{})
	if err == nil {
		t.Fatal("expected error for missing seed")
	}
}

func TestBuild_SeedOnly(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Foo", "Foo", "function", "src/core.ts", 10)
	g := makeUSG([]usg.Symbol{seed}, nil)
	pack, err := Build(g, "sym_1", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pack.SeedSymbol.ID != "sym_1" {
		t.Errorf("seed mismatch: %s", pack.SeedSymbol.ID)
	}
	if strings.Contains(pack.Text, "CALLED BY") {
		t.Errorf("expected no callers section, got text:\n%s", pack.Text)
	}
	if strings.Contains(pack.Text, "CALLS:") {
		t.Errorf("expected no callees section, got text:\n%s", pack.Text)
	}
}

func TestBuild_CallersAndCallees(t *testing.T) {
	seed := sym("sym_seed", "src/core.ts::Process", "Process", "function", "src/core.ts", 5)
	caller := sym("sym_a", "src/api.ts::CreateOrder", "CreateOrder", "function", "src/api.ts", 10)
	callee := sym("sym_b", "src/db.ts::Save", "Save", "function", "src/db.ts", 3)

	calls := []usg.CallEdge{
		{FromSymbolID: "sym_a", ToSymbolID: strPtr("sym_seed"), Loc: usg.Location{File: "src/api.ts", Line: 10}, Confidence: "resolved"},
		{FromSymbolID: "sym_seed", ToSymbolID: strPtr("sym_b"), Loc: usg.Location{File: "src/core.ts", Line: 6}, Confidence: "resolved"},
	}
	g := makeUSG([]usg.Symbol{seed, caller, callee}, calls)

	pack, err := Build(g, "sym_seed", Options{Radius: 2, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(pack.Text, "CALLED BY") {
		t.Errorf("expected CALLED BY section in text:\n%s", pack.Text)
	}
	if !strings.Contains(pack.Text, "CALLS") {
		t.Errorf("expected CALLS section in text:\n%s", pack.Text)
	}
	// subgraph should include caller and callee
	symIDs := map[string]bool{}
	for _, s := range pack.Symbols {
		symIDs[s.ID] = true
	}
	if !symIDs["sym_a"] {
		t.Errorf("caller sym_a not in subgraph symbols")
	}
	if !symIDs["sym_b"] {
		t.Errorf("callee sym_b not in subgraph symbols")
	}
}

func TestBuild_InheritanceEdge(t *testing.T) {
	child := sym("sym_child", "src/a.ts::Dog", "Dog", "class", "src/a.ts", 1)
	parent := sym("sym_parent", "src/a.ts::Animal", "Animal", "class", "src/a.ts", 0)
	g := &usg.USG{
		Symbols: []usg.Symbol{child, parent},
		Edges: usg.Edges{
			Inherits: []usg.InheritanceEdge{
				{ChildSymbolID: "sym_child", ParentSymbolID: "sym_parent", Confidence: "resolved"},
			},
		},
	}
	pack, err := Build(g, "sym_child", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(pack.Text, "INHERITANCE") {
		t.Errorf("expected INHERITANCE section in text:\n%s", pack.Text)
	}
	if !strings.Contains(pack.Text, "src/a.ts::Animal") {
		t.Errorf("expected parent qname in text:\n%s", pack.Text)
	}
}

func TestBuild_RiskFlags(t *testing.T) {
	seed := sym("sym_s", "src/a.ts::Fn", "Fn", "function", "src/a.ts", 20)
	g := &usg.USG{
		Symbols: []usg.Symbol{seed},
		Edges:   usg.Edges{},
		RiskFlags: []usg.RiskFlag{
			{Loc: usg.Location{File: "src/a.ts", Line: 22}, Kind: "dynamic_dispatch", Note: "computed call"},
			{Loc: usg.Location{File: "src/other.ts", Line: 5}, Kind: "eval", Note: "eval usage"},
		},
	}
	pack, err := Build(g, "sym_s", Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Only same-file flag within window should be in the pack
	if len(pack.RiskFlags) != 1 {
		t.Errorf("expected 1 risk flag in pack, got %d", len(pack.RiskFlags))
	}
	if pack.RiskFlags[0].Kind != "dynamic_dispatch" {
		t.Errorf("wrong risk flag kind: %s", pack.RiskFlags[0].Kind)
	}
	if !strings.Contains(pack.Text, "RISK") {
		t.Errorf("expected RISK section in text:\n%s", pack.Text)
	}
}

// ---- text output tests ----

func TestBuild_TextContainsSeed(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Process", "Process", "function", "src/core.ts", 10)
	g := makeUSG([]usg.Symbol{seed}, nil)
	pack, _ := Build(g, "sym_1", Options{Radius: 2, BudgetChars: 8000})

	if !strings.Contains(pack.Text, "src/core.ts::Process") {
		t.Errorf("text should contain seed qname, got:\n%s", pack.Text)
	}
	if !strings.Contains(pack.Text, "SYMBOL:") {
		t.Errorf("text should contain SYMBOL: header")
	}
	if !strings.Contains(pack.Text, "STRUCTURAL CONFIDENCE") {
		t.Errorf("text should contain STRUCTURAL CONFIDENCE")
	}
}

func TestBuild_TextBudgetLimit(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Process", "Process", "function", "src/core.ts", 10)
	g := makeUSG([]usg.Symbol{seed}, nil)
	// Small budget: output must include the budget usage line and be reasonably short
	pack, _ := Build(g, "sym_1", Options{Radius: 2, BudgetChars: 50})
	if !strings.Contains(pack.Text, "budget:") {
		t.Errorf("expected budget line in text, got:\n%s", pack.Text)
	}
}

func TestBuild_ResolveByQName(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Foo", "Foo", "function", "src/core.ts", 1)
	g := makeUSG([]usg.Symbol{seed}, nil)
	pack, err := Build(g, "src/core.ts::Foo", Options{})
	if err != nil {
		t.Fatalf("unexpected error resolving by qname: %v", err)
	}
	if pack.SeedSymbol.ID != "sym_1" {
		t.Errorf("expected sym_1, got %s", pack.SeedSymbol.ID)
	}
}

// ---- FormatJSON() tests ----

func TestFormatJSON_ValidJSON(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Process", "Process", "function", "src/core.ts", 10)
	g := makeUSG([]usg.Symbol{seed}, nil)
	pack, _ := Build(g, "sym_1", Options{})
	data, err := FormatJSON(pack)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("invalid json: %v\n%s", err, data)
	}
	if out["seedId"] != "sym_1" {
		t.Errorf("wrong seedId in JSON: %v", out["seedId"])
	}
}

// ---- grouped edge output tests ----

func TestBuild_GroupedCallersOutput(t *testing.T) {
	seed := sym("sym_seed", "src/core.ts::Fn", "Fn", "function", "src/core.ts", 5)
	callerA := sym("sym_a", "src/a.ts::A", "A", "function", "src/a.ts", 1)
	callerB := sym("sym_b", "src/a.ts::B", "B", "function", "src/a.ts", 2)
	callerC := sym("sym_c", "src/b.ts::C", "C", "function", "src/b.ts", 3)

	calls := []usg.CallEdge{
		{FromSymbolID: "sym_a", ToSymbolID: strPtr("sym_seed"), Loc: usg.Location{File: "src/a.ts", Line: 10}, Confidence: "resolved"},
		{FromSymbolID: "sym_b", ToSymbolID: strPtr("sym_seed"), Loc: usg.Location{File: "src/a.ts", Line: 20}, Confidence: "resolved"},
		{FromSymbolID: "sym_c", ToSymbolID: strPtr("sym_seed"), Loc: usg.Location{File: "src/b.ts", Line: 5}, Confidence: "inferred"},
	}
	g := makeUSG([]usg.Symbol{seed, callerA, callerB, callerC}, calls)

	pack, err := Build(g, "sym_seed", Options{BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Grouped output should show files
	if !strings.Contains(pack.Text, "src/a.ts") {
		t.Errorf("expected src/a.ts in grouped output:\n%s", pack.Text)
	}
	if !strings.Contains(pack.Text, "src/b.ts") {
		t.Errorf("expected src/b.ts in grouped output:\n%s", pack.Text)
	}
}
