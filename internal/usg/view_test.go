package usg

import (
	"testing"
)

func strPtr(s string) *string { return &s }

func TestBuildView_EmptyGraph(t *testing.T) {
	g := &USG{}
	v := BuildView(g)
	if v.Graph != g {
		t.Error("Graph pointer mismatch")
	}
	if len(v.SymbolByID) != 0 {
		t.Errorf("expected empty SymbolByID, got %d", len(v.SymbolByID))
	}
	if len(v.SymbolByQName) != 0 {
		t.Errorf("expected empty SymbolByQName, got %d", len(v.SymbolByQName))
	}
	if len(v.CallersByTo) != 0 {
		t.Errorf("expected empty CallersByTo, got %d", len(v.CallersByTo))
	}
	if len(v.CalleesByFrom) != 0 {
		t.Errorf("expected empty CalleesByFrom, got %d", len(v.CalleesByFrom))
	}
}

func TestBuildView_SymbolIndexes(t *testing.T) {
	syms := []Symbol{
		{ID: "sym_1", QName: "src/a.ts::Foo", Name: "Foo", Kind: "function"},
		{ID: "sym_2", QName: "src/b.ts::Bar", Name: "Bar", Kind: "class"},
	}
	g := &USG{Symbols: syms}
	v := BuildView(g)

	if got := v.SymbolByID["sym_1"]; got.Name != "Foo" {
		t.Errorf("SymbolByID[sym_1] = %q, want Foo", got.Name)
	}
	if got := v.SymbolByID["sym_2"]; got.Name != "Bar" {
		t.Errorf("SymbolByID[sym_2] = %q, want Bar", got.Name)
	}
	if got := v.SymbolByQName["src/a.ts::Foo"]; got.ID != "sym_1" {
		t.Errorf("SymbolByQName[src/a.ts::Foo] = %q, want sym_1", got.ID)
	}
	if got := v.SymbolByQName["src/b.ts::Bar"]; got.ID != "sym_2" {
		t.Errorf("SymbolByQName[src/b.ts::Bar] = %q, want sym_2", got.ID)
	}
}

func TestBuildView_CallEdgeIndexes(t *testing.T) {
	calls := []CallEdge{
		{FromSymbolID: "sym_a", ToSymbolID: strPtr("sym_b"), Confidence: "resolved"},
		{FromSymbolID: "sym_a", ToSymbolID: strPtr("sym_c"), Confidence: "resolved"},
		{FromSymbolID: "sym_b", ToSymbolID: strPtr("sym_c"), Confidence: "inferred"},
		{FromSymbolID: "sym_x", ToSymbolID: nil, Confidence: "dynamic"}, // external call
	}
	g := &USG{Edges: Edges{Calls: calls}}
	v := BuildView(g)

	// CallersByTo: sym_b is called by sym_a
	if got := v.CallersByTo["sym_b"]; len(got) != 1 || got[0].FromSymbolID != "sym_a" {
		t.Errorf("CallersByTo[sym_b] = %v, want [{sym_a -> sym_b}]", got)
	}
	// CallersByTo: sym_c is called by sym_a and sym_b
	if got := v.CallersByTo["sym_c"]; len(got) != 2 {
		t.Errorf("CallersByTo[sym_c] len = %d, want 2", len(got))
	}
	// CalleesByFrom: sym_a calls sym_b and sym_c
	if got := v.CalleesByFrom["sym_a"]; len(got) != 2 {
		t.Errorf("CalleesByFrom[sym_a] len = %d, want 2", len(got))
	}
	// nil ToSymbolID edges are indexed in CalleesByFrom only
	if got := v.CalleesByFrom["sym_x"]; len(got) != 1 {
		t.Errorf("CalleesByFrom[sym_x] len = %d, want 1", len(got))
	}
	// nil ToSymbolID should NOT appear in CallersByTo
	for k := range v.CallersByTo {
		for _, e := range v.CallersByTo[k] {
			if e.ToSymbolID == nil {
				t.Errorf("CallersByTo should not contain edges with nil ToSymbolID")
			}
		}
	}
}
