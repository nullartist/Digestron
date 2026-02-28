package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/focus"
	"github.com/nullartist/digestron/internal/search"
	"github.com/nullartist/digestron/internal/usg"
)

var impactFlags struct {
	Radius      int
	BudgetChars int
	JSON        bool
}

var impactCmd = &cobra.Command{
	Use:   "impact <symbol-ref> [path]",
	Short: "Print the Focus Pack for a symbol",
	Long: `Builds and prints an impact-aware Focus Pack for <symbol-ref>.
<symbol-ref> is a symbol name, qname, or symbolId.
[path] is the repository root (default: current directory).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runImpact,
}

func init() {
	impactCmd.Flags().IntVar(&impactFlags.Radius, "radius", 2, "BFS radius for caller traversal")
	impactCmd.Flags().IntVar(&impactFlags.BudgetChars, "budget-chars", 8000, "Max characters for text output")
	impactCmd.Flags().BoolVar(&impactFlags.JSON, "json", false, "Output the focus subgraph as JSON")
}

func runImpact(_ *cobra.Command, args []string) error {
	symbolRef := args[0]
	root := "."
	if len(args) == 2 {
		root = args[1]
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("impact: resolve root: %w", err)
	}

	graph, err := usg.Load(absRoot)
	if err != nil {
		return fmt.Errorf("impact: load USG: %w", err)
	}

	// Resolve symbol-ref → symbolId.
	// Accept bare symbolId directly (starts with "sym_"), or search by name/qname.
	seedID := ""
	if strings.HasPrefix(symbolRef, "sym_") {
		// verify it exists
		for _, s := range graph.Symbols {
			if s.ID == symbolRef {
				seedID = symbolRef
				break
			}
		}
	}
	if seedID == "" {
		results := search.Find(graph, symbolRef)
		if len(results) == 0 {
			return fmt.Errorf("impact: symbol %q not found in USG", symbolRef)
		}
		if len(results) > 1 {
			fmt.Fprintf(os.Stderr, "warning: %d symbols match %q; using top result\n", len(results), symbolRef)
		}
		seedID = results[0].Symbol.ID
	}

	opts := focus.Options{
		Radius:      impactFlags.Radius,
		BudgetChars: impactFlags.BudgetChars,
	}

	pack, err := focus.Build(graph, seedID, opts)
	if err != nil {
		return fmt.Errorf("impact: %w", err)
	}

	if impactFlags.JSON {
		data, err := focus.FormatJSON(pack)
		if err != nil {
			return fmt.Errorf("impact: json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(focus.FormatText(pack, graph, opts))
	return nil
}
