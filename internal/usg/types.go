package usg

type Location struct {
	File      string `json:"file"`
	StartLine int    `json:"startLine,omitempty"`
	StartCol  int    `json:"startCol,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	EndCol    int    `json:"endCol,omitempty"`
	Line      int    `json:"line,omitempty"`
	Col       int    `json:"col,omitempty"`
}

type Module struct {
	ID       string   `json:"id"`
	Path     string   `json:"path"`
	Language string   `json:"language"`
	Imports  []string `json:"imports"`
	Exports  []string `json:"exports"`
}

type Symbol struct {
	ID        string   `json:"id"`
	QName     string   `json:"qname"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	ModuleID  string   `json:"moduleId"`
	Signature string   `json:"signature"`
	Loc       Location `json:"loc"`
}

type ExternalRef struct {
	Module string `json:"module"`
	Name   string `json:"name"`
}

type CallEdge struct {
	FromSymbolID string       `json:"fromSymbolId"`
	ToSymbolID   *string      `json:"toSymbolId"`
	ToExternal   *ExternalRef `json:"toExternal"`
	Loc          Location     `json:"loc"`
	Confidence   string       `json:"confidence"`
}

type InheritanceEdge struct {
	ChildSymbolID  string `json:"childSymbolId"`
	ParentSymbolID string `json:"parentSymbolId"`
	Confidence     string `json:"confidence"`
}

type InstantiationSite struct {
	SymbolID   string   `json:"symbolId"`
	Loc        Location `json:"loc"`
	Confidence string   `json:"confidence"`
}

type EntryPoint struct {
	File     string  `json:"file"`
	SymbolID *string `json:"symbolId"`
	Kind     string  `json:"kind"`
}

type RiskFlag struct {
	Loc  Location `json:"loc"`
	Kind string   `json:"kind"`
	Note string   `json:"note"`
}

type Stats struct {
	TotalModules         int     `json:"totalModules"`
	TotalSymbols         int     `json:"totalSymbols"`
	CallsTotal           int     `json:"callsTotal"`
	CallsResolved        int     `json:"callsResolved"`
	CallsInferred        int     `json:"callsInferred"`
	CallsDynamic         int     `json:"callsDynamic"`
	SymbolCoverageRatio  float64 `json:"symbolCoverageRatio"`
	ResolvedEdgeRatio    float64 `json:"resolvedEdgeRatio"`
	DynamicRatio         float64 `json:"dynamicRatio"`
	StructuralConfidence float64 `json:"structuralConfidence"`
}

type Edges struct {
	Calls        []CallEdge          `json:"calls"`
	Inherits     []InheritanceEdge   `json:"inherits"`
	Instantiates []InstantiationSite `json:"instantiates"`
}

type USG struct {
	Version     string       `json:"version"`
	Root        string       `json:"root"`
	GeneratedAt string       `json:"generatedAt"`
	Language    []string     `json:"language"`
	Modules     []Module     `json:"modules"`
	Symbols     []Symbol     `json:"symbols"`
	Edges       Edges        `json:"edges"`
	EntryPoints []EntryPoint `json:"entryPoints"`
	RiskFlags   []RiskFlag   `json:"riskFlags"`
	Stats       Stats        `json:"stats"`
}
