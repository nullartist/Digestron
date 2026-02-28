package search

import (
	"strings"

	"github.com/nullartist/digestron/internal/usg"
)

// Result is a single symbol match with a relevance score.
type Result struct {
	Symbol usg.Symbol
	Score  int // higher = more relevant
}

// Find returns symbols matching query, sorted by descending score.
// Matching rules (additive):
//
//	exact qname match      → score 100
//	exact name match       → score  80
//	qname suffix ::query   → score  60
//	name contains query    → score  40
//	qname contains query   → score  20
func Find(graph *usg.USG, query string) []Result {
	if query == "" {
		return nil
	}
	var results []Result
	for _, sym := range graph.Symbols {
		score := scoreSymbol(sym, query)
		if score > 0 {
			results = append(results, Result{Symbol: sym, Score: score})
		}
	}
	// Sort descending by score (simple insertion-style for small slices is fine;
	// use a standard sort for correctness).
	sortResults(results)
	return results
}

func scoreSymbol(sym usg.Symbol, query string) int {
	score := 0
	if sym.QName == query {
		score += 100
	}
	if sym.Name == query {
		score += 80
	}
	if strings.HasSuffix(sym.QName, "::"+query) {
		score += 60
	}
	if score == 0 {
		// substring matches only if no exact match
		if strings.Contains(sym.Name, query) {
			score += 40
		}
		if strings.Contains(sym.QName, query) {
			score += 20
		}
	}
	return score
}

// sortResults sorts in-place by descending Score (stable).
func sortResults(r []Result) {
	// simple insertion sort (N is small in practice)
	for i := 1; i < len(r); i++ {
		for j := i; j > 0 && r[j].Score > r[j-1].Score; j-- {
			r[j], r[j-1] = r[j-1], r[j]
		}
	}
}
