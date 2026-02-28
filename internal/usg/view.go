package usg

// View provides O(1) index maps over a USG for fast lookups.
// It does not modify the underlying graph.
type View struct {
	Graph         *USG
	SymbolByID    map[string]Symbol
	SymbolByQName map[string]Symbol
	CallersByTo   map[string][]CallEdge
	CalleesByFrom map[string][]CallEdge
}

// BuildView constructs index maps from g for efficient lookups.
func BuildView(g *USG) *View {
	v := &View{
		Graph:         g,
		SymbolByID:    make(map[string]Symbol, len(g.Symbols)),
		SymbolByQName: make(map[string]Symbol, len(g.Symbols)),
		CallersByTo:   make(map[string][]CallEdge),
		CalleesByFrom: make(map[string][]CallEdge),
	}
	for _, s := range g.Symbols {
		v.SymbolByID[s.ID] = s
		v.SymbolByQName[s.QName] = s
	}
	for _, e := range g.Edges.Calls {
		v.CalleesByFrom[e.FromSymbolID] = append(v.CalleesByFrom[e.FromSymbolID], e)
		if e.ToSymbolID != nil {
			v.CallersByTo[*e.ToSymbolID] = append(v.CallersByTo[*e.ToSymbolID], e)
		}
	}
	return v
}
