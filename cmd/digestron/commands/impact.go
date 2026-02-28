package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/focus"
	"github.com/nullartist/digestron/internal/search"
	"github.com/nullartist/digestron/internal/snippets"
	"github.com/nullartist/digestron/internal/usg"
)

var impactFlags struct {
	Radius         int
	BudgetChars    int
	JSON           bool
	Snippets       bool
	SnippetsBudget int
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
	impactCmd.Flags().IntVar(&impactFlags.BudgetChars, "budget-chars", 9000, "Max characters for text output")
	impactCmd.Flags().BoolVar(&impactFlags.JSON, "json", false, "Output the focus pack as JSON (FocusPackJSON v0.25)")
	impactCmd.Flags().BoolVar(&impactFlags.Snippets, "snippets", false, "Include code snippets in JSON output (requires --json)")
	impactCmd.Flags().IntVar(&impactFlags.SnippetsBudget, "snippets-budget", 8000, "Snippet budget in chars (requires --snippets)")
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
		v := usg.BuildView(graph)
		fp := focus.BuildPackJSON(pack, graph, v, absRoot, impactFlags.Radius, impactFlags.BudgetChars)

		if impactFlags.Snippets {
			sreqs := focus.BuildSnippetRequests(fp, 3, 3)
			blocks, text, _ := snippets.Build(sreqs, snippets.Options{
				RepoRoot:    absRoot,
				BudgetChars: impactFlags.SnippetsBudget,
				MergeGap:    2,
			})
			snipBlocks := make([]focus.SnippetBlockJSON, 0, len(blocks))
			for _, b := range blocks {
				snipBlocks = append(snipBlocks, focus.SnippetBlockJSON{
					File: b.File, StartLine: b.StartLine, EndLine: b.EndLine,
					Label: b.Label, Text: b.Text,
				})
			}
			fp.Snippets = &focus.SnippetsPayload{
				BudgetChars: impactFlags.SnippetsBudget,
				Blocks:      snipBlocks,
				Text:        text,
			}
		}

		data, err := json.MarshalIndent(fp, "", "  ")
		if err != nil {
			return fmt.Errorf("impact: json: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Print(pack.Text)
	return nil
}
