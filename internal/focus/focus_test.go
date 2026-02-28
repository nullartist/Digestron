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
	if len(pack.Callers) != 0 {
		t.Errorf("expected no callers, got %d", len(pack.Callers))
	}
	if len(pack.Callees) != 0 {
		t.Errorf("expected no callees, got %d", len(pack.Callees))
	}
}

func TestBuild_CallersAndCallees(t *testing.T) {
	seed := sym("sym_seed", "src/core.ts::Process", "Process", "function", "src/core.ts", 5)
	caller := sym("sym_a", "src/api.ts::CreateOrder", "CreateOrder", "function", "src/api.ts", 10)
	callee := sym("sym_b", "src/db.ts::Save", "Save", "function", "src/db.ts", 3)

	calls := []usg.CallEdge{
		{FromSymbolID: "sym_a", ToSymbolID: strPtr("sym_seed"), Confidence: "resolved"},
		{FromSymbolID: "sym_seed", ToSymbolID: strPtr("sym_b"), Confidence: "resolved"},
	}
	g := makeUSG([]usg.Symbol{seed, caller, callee}, calls)

	pack, err := Build(g, "sym_seed", Options{Radius: 2, BudgetChars: 8000})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pack.Callers) != 1 {
		t.Errorf("expected 1 caller, got %d", len(pack.Callers))
	}
	if pack.Callers[0].ID != "sym_a" {
		t.Errorf("wrong caller: %s", pack.Callers[0].ID)
	}
	if len(pack.Callees) != 1 {
		t.Errorf("expected 1 callee, got %d", len(pack.Callees))
	}
	if pack.Callees[0].ID != "sym_b" {
		t.Errorf("wrong callee: %s", pack.Callees[0].ID)
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
	if len(pack.Inherits) != 1 {
		t.Errorf("expected 1 parent, got %d", len(pack.Inherits))
	}
	if pack.Inherits[0].ID != "sym_parent" {
		t.Errorf("wrong parent: %s", pack.Inherits[0].ID)
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
	// Only same-file flag within radius should be included
	if len(pack.RiskFlags) != 1 {
		t.Errorf("expected 1 risk flag, got %d", len(pack.RiskFlags))
	}
	if pack.RiskFlags[0].Kind != "dynamic_dispatch" {
		t.Errorf("wrong risk flag kind: %s", pack.RiskFlags[0].Kind)
	}
}

// ---- FormatText() tests ----

func TestFormatText_ContainsSeed(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Process", "Process", "function", "src/core.ts", 10)
	g := makeUSG([]usg.Symbol{seed}, nil)
	pack, _ := Build(g, "sym_1", Options{})
	text := FormatText(pack, g, Options{Radius: 2, BudgetChars: 8000})

	if !strings.Contains(text, "src/core.ts::Process") {
		t.Errorf("text should contain seed qname, got:\n%s", text)
	}
	if !strings.Contains(text, "[SEED]") {
		t.Errorf("text should contain [SEED] header")
	}
	if !strings.Contains(text, "structuralConfidence") {
		t.Errorf("text should contain structuralConfidence")
	}
}

func TestFormatText_BudgetTruncation(t *testing.T) {
	seed := sym("sym_1", "src/core.ts::Process", "Process", "function", "src/core.ts", 10)
	g := makeUSG([]usg.Symbol{seed}, nil)
	pack, _ := Build(g, "sym_1", Options{})
	// Very small budget
	text := FormatText(pack, g, Options{Radius: 2, BudgetChars: 50})
	if !strings.Contains(text, "truncated") {
		t.Errorf("expected truncation marker, got:\n%s", text)
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
