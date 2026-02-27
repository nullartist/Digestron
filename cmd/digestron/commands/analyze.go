package commands

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/usg"
)

var analyzeRoot string

var analyzeCmd = &cobra.Command{
	Use:   "analyze",
	Short: "Print a summary of the indexed USG",
	Long:  `Loads .digestron/usg.v0.1.json and prints a structural summary.`,
	RunE:  runAnalyze,
}

func init() {
	analyzeCmd.Flags().StringVar(&analyzeRoot, "root", ".", "Path to the repository root")
}

func runAnalyze(_ *cobra.Command, _ []string) error {
	absRoot, err := filepath.Abs(analyzeRoot)
	if err != nil {
		return fmt.Errorf("analyze: resolve root: %w", err)
	}

	graph, err := usg.Load(absRoot)
	if err != nil {
		return fmt.Errorf("analyze: load USG: %w", err)
	}

	s := graph.Stats
	fmt.Printf("USG v%s  root=%s  generated=%s\n", graph.Version, graph.Root, graph.GeneratedAt)
	fmt.Printf("  modules             : %d\n", s.TotalModules)
	fmt.Printf("  symbols             : %d\n", s.TotalSymbols)
	fmt.Printf("  calls (total)       : %d\n", s.CallsTotal)
	fmt.Printf("    resolved          : %d\n", s.CallsResolved)
	fmt.Printf("    inferred          : %d\n", s.CallsInferred)
	fmt.Printf("    dynamic           : %d\n", s.CallsDynamic)
	fmt.Printf("  resolvedEdgeRatio   : %.3f\n", s.ResolvedEdgeRatio)
	fmt.Printf("  dynamicRatio        : %.3f\n", s.DynamicRatio)
	fmt.Printf("  symbolCoverageRatio : %.3f\n", s.SymbolCoverageRatio)
	fmt.Printf("  structuralConfidence: %.3f\n", s.StructuralConfidence)

	if len(graph.RiskFlags) > 0 {
		fmt.Printf("\nRisk flags (%d):\n", len(graph.RiskFlags))
		for _, rf := range graph.RiskFlags {
			fmt.Printf("  [%s] %s:%d — %s\n", rf.Kind, rf.Loc.File, rf.Loc.Line, rf.Note)
		}
	}

	if len(graph.EntryPoints) > 0 {
		fmt.Printf("\nEntry points (%d):\n", len(graph.EntryPoints))
		for _, ep := range graph.EntryPoints {
			fmt.Printf("  [%s] %s\n", ep.Kind, ep.File)
		}
	}

	return nil
}
