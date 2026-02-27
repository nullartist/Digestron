package commands

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/usg"
)

var impactRoot string
var impactSymbol string

var impactCmd = &cobra.Command{
	Use:   "impact",
	Short: "Show symbols that call a given symbol",
	Long:  `Loads the USG and lists all callers of the specified symbol (by name or qname).`,
	RunE:  runImpact,
}

func init() {
	impactCmd.Flags().StringVar(&impactRoot, "root", ".", "Path to the repository root")
	impactCmd.Flags().StringVar(&impactSymbol, "symbol", "", "Symbol name or qname to look up")
	_ = impactCmd.MarkFlagRequired("symbol")
}

func runImpact(_ *cobra.Command, _ []string) error {
	absRoot, err := filepath.Abs(impactRoot)
	if err != nil {
		return fmt.Errorf("impact: resolve root: %w", err)
	}

	graph, err := usg.Load(absRoot)
	if err != nil {
		return fmt.Errorf("impact: load USG: %w", err)
	}

	// Find matching target symbols (by qname or name substring).
	var targetIDs []string
	for _, sym := range graph.Symbols {
		if sym.QName == impactSymbol || sym.Name == impactSymbol ||
			strings.HasSuffix(sym.QName, "::"+impactSymbol) {
			targetIDs = append(targetIDs, sym.ID)
		}
	}
	if len(targetIDs) == 0 {
		return fmt.Errorf("impact: symbol %q not found in USG", impactSymbol)
	}

	targetSet := make(map[string]bool, len(targetIDs))
	for _, id := range targetIDs {
		targetSet[id] = true
	}

	// Collect callers.
	callerIDs := make(map[string]bool)
	for _, call := range graph.Edges.Calls {
		if call.ToSymbolID != nil && targetSet[*call.ToSymbolID] {
			callerIDs[call.FromSymbolID] = true
		}
	}

	// Resolve caller symbols.
	symByID := make(map[string]usg.Symbol, len(graph.Symbols))
	for _, s := range graph.Symbols {
		symByID[s.ID] = s
	}

	fmt.Printf("Impact analysis for %q (%d match(es)):\n", impactSymbol, len(targetIDs))
	if len(callerIDs) == 0 {
		fmt.Println("  (no callers found)")
		return nil
	}
	for id := range callerIDs {
		if s, ok := symByID[id]; ok {
			fmt.Printf("  %s  [%s]  %s\n", s.QName, s.Kind, s.Loc.File)
		} else {
			fmt.Printf("  <unknown sym %s>\n", id)
		}
	}
	return nil
}
