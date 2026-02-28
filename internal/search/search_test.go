package search

import (
	"testing"

	"github.com/nullartist/digestron/internal/usg"
)

func makeGraph(syms []usg.Symbol) *usg.USG {
	return &usg.USG{Symbols: syms}
}

func sym(id, qname, name, kind string) usg.Symbol {
	return usg.Symbol{ID: id, QName: qname, Name: name, Kind: kind}
}

func TestFind_EmptyQuery(t *testing.T) {
	g := makeGraph([]usg.Symbol{sym("s1", "pkg::Foo", "Foo", "class")})
	if got := Find(g, ""); got != nil {
		t.Fatalf("expected nil for empty query, got %v", got)
	}
}

func TestFind_ExactQName(t *testing.T) {
	g := makeGraph([]usg.Symbol{
		sym("s1", "src/core.ts::OrderProcessor", "OrderProcessor", "class"),
		sym("s2", "src/util.ts::Util", "Util", "class"),
	})
	results := Find(g, "src/core.ts::OrderProcessor")
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Symbol.ID != "s1" {
		t.Errorf("wrong symbol: %s", results[0].Symbol.ID)
	}
	if results[0].Score < 100 {
		t.Errorf("exact qname match should score >= 100, got %d", results[0].Score)
	}
}

func TestFind_ExactName(t *testing.T) {
	g := makeGraph([]usg.Symbol{
		sym("s1", "src/core.ts::OrderProcessor", "OrderProcessor", "class"),
		sym("s2", "src/util.ts::OrderProcessor.process", "process", "method"),
	})
	results := Find(g, "OrderProcessor")
	if len(results) == 0 {
		t.Fatal("expected results, got none")
	}
	// s1 should rank higher: exact name match
	if results[0].Symbol.ID != "s1" {
		t.Errorf("expected s1 to rank first, got %s", results[0].Symbol.ID)
	}
}

func TestFind_QNameSuffix(t *testing.T) {
	g := makeGraph([]usg.Symbol{
		sym("s1", "src/core.ts::OrderProcessor.process", "process", "method"),
		sym("s2", "src/util.ts::SomeOther", "SomeOther", "class"),
	})
	results := Find(g, "OrderProcessor.process")
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Symbol.ID != "s1" {
		t.Errorf("expected s1, got %s", results[0].Symbol.ID)
	}
}

func TestFind_SubstringMatch(t *testing.T) {
	g := makeGraph([]usg.Symbol{
		sym("s1", "src/core.ts::OrderProcessor", "OrderProcessor", "class"),
		sym("s2", "src/api.ts::createOrder", "createOrder", "function"),
	})
	results := Find(g, "Order")
	if len(results) < 2 {
		t.Fatalf("expected >=2 results, got %d", len(results))
	}
}

func TestFind_NoMatch(t *testing.T) {
	g := makeGraph([]usg.Symbol{
		sym("s1", "src/core.ts::Foo", "Foo", "class"),
	})
	results := Find(g, "zzz_no_match")
	if len(results) != 0 {
		t.Errorf("expected no results, got %d", len(results))
	}
}

func TestFind_ScoreOrdering(t *testing.T) {
	// exact name > substring
	g := makeGraph([]usg.Symbol{
		sym("s1", "src/a.ts::ContainsProcessor", "ContainsProcessor", "class"),
		sym("s2", "src/b.ts::Processor", "Processor", "class"),
	})
	results := Find(g, "Processor")
	if len(results) < 2 {
		t.Fatal("expected 2 results")
	}
	if results[0].Symbol.ID != "s2" {
		t.Errorf("exact name match should rank first, got %s first", results[0].Symbol.ID)
	}
}
