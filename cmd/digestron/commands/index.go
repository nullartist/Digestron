package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/indexer"
	"github.com/nullartist/digestron/internal/usg"
)

var indexFlags struct {
	Tsconfig     []string
	IncludeTests bool
}

var indexCmd = &cobra.Command{
	Use:   "index [path]",
	Short: "Index a TypeScript repository into a USG",
	Long:  `Runs the TypeScript extractor and writes .digestron/usg.v0.1.json in [path] (default: current directory).`,
	Args:  cobra.MaximumNArgs(1),
	RunE:  runIndex,
}

func init() {
	indexCmd.Flags().StringSliceVar(&indexFlags.Tsconfig, "tsconfig", nil, "tsconfig path(s) relative to root (auto-detected if empty)")
	indexCmd.Flags().BoolVar(&indexFlags.IncludeTests, "include-tests", false, "Include test files in the extraction")
}

func runIndex(_ *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("index: resolve root: %w", err)
	}

	scriptPath := filepath.Join("tools", "ts-extract", "src", "index.mjs")
	if _, err := os.Stat(scriptPath); os.IsNotExist(err) {
		return fmt.Errorf("index: ts-extract script not found at %s", scriptPath)
	}

	fmt.Printf("Indexing %s ...\n", absRoot)

	resp, err := indexer.RunTSExtract(scriptPath, absRoot, indexFlags.Tsconfig, indexFlags.IncludeTests)
	if err != nil {
		if resp != nil {
			for _, d := range resp.Diagnostics {
				fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", d.Level, d.Code, d.Message)
			}
		}
		return fmt.Errorf("index: extraction failed: %w", err)
	}

	for _, d := range resp.Diagnostics {
		fmt.Fprintf(os.Stderr, "[%s] %s: %s\n", d.Level, d.Code, d.Message)
	}

	// Unmarshal raw into USG edges/symbols/modules
	var raw struct {
		Modules      []usg.Module            `json:"modules"`
		Symbols      []usg.Symbol            `json:"symbols"`
		Calls        []usg.CallEdge          `json:"calls"`
		Inherits     []usg.InheritanceEdge   `json:"inherits"`
		Instantiates []usg.InstantiationSite `json:"instantiates"`
		EntryPoints  []usg.EntryPoint        `json:"entryPoints"`
		RiskFlags    []usg.RiskFlag          `json:"riskFlags"`
		Stats        usg.Stats               `json:"stats"`
	}
	if err := json.Unmarshal(resp.Raw, &raw); err != nil {
		return fmt.Errorf("index: unmarshal raw: %w", err)
	}

	graph := &usg.USG{
		Version:     "usg.v0.1",
		Root:        absRoot,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Language:    []string{"ts"},
		Modules:     raw.Modules,
		Symbols:     raw.Symbols,
		Edges: usg.Edges{
			Calls:        raw.Calls,
			Inherits:     raw.Inherits,
			Instantiates: raw.Instantiates,
		},
		EntryPoints: raw.EntryPoints,
		RiskFlags:   raw.RiskFlags,
		Stats:       raw.Stats,
	}

	if err := usg.Save(absRoot, graph); err != nil {
		return fmt.Errorf("index: save: %w", err)
	}

	out := usg.OutputPath(absRoot)
	fmt.Printf("USG written to %s\n", out)
	fmt.Printf("  modules=%d  symbols=%d  calls=%d  structuralConfidence=%.2f\n",
		graph.Stats.TotalModules,
		graph.Stats.TotalSymbols,
		graph.Stats.CallsTotal,
		graph.Stats.StructuralConfidence,
	)
	return nil
}
