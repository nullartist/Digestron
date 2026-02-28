package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/search"
	"github.com/nullartist/digestron/internal/usg"
)

var analyzeFlags struct {
	JSON bool
}

var analyzeCmd = &cobra.Command{
	Use:   "analyze <query> [path]",
	Short: "Search symbols in the indexed USG",
	Long: `Searches the USG for symbols matching <query> (by name or qname).
[path] is the repository root (default: current directory).`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runAnalyze,
}

func init() {
	analyzeCmd.Flags().BoolVar(&analyzeFlags.JSON, "json", false, "Output results as JSON")
}

func runAnalyze(_ *cobra.Command, args []string) error {
	query := args[0]
	root := "."
	if len(args) == 2 {
		root = args[1]
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("analyze: resolve root: %w", err)
	}

	graph, err := usg.Load(absRoot)
	if err != nil {
		return fmt.Errorf("analyze: load USG: %w", err)
	}

	results := search.Find(graph, query)

	if analyzeFlags.JSON {
		type jsonResult struct {
			SymbolID  string `json:"symbolId"`
			QName     string `json:"qname"`
			Name      string `json:"name"`
			Kind      string `json:"kind"`
			Signature string `json:"signature"`
			Score     int    `json:"score"`
		}
		var out []jsonResult
		for _, r := range results {
			out = append(out, jsonResult{
				SymbolID:  r.Symbol.ID,
				QName:     r.Symbol.QName,
				Name:      r.Symbol.Name,
				Kind:      r.Symbol.Kind,
				Signature: r.Symbol.Signature,
				Score:     r.Score,
			})
		}
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(out)
	}

	fmt.Printf("Searching %q in %s\n", query, absRoot)
	fmt.Printf("Found %d match(es):\n\n", len(results))
	for i, r := range results {
		fmt.Printf("  %d. [%s] %s\n", i+1, r.Symbol.Kind, r.Symbol.QName)
		fmt.Printf("     id: %s\n", r.Symbol.ID)
		if r.Symbol.Signature != "" {
			fmt.Printf("     sig: %s\n", r.Symbol.Signature)
		}
		fmt.Printf("     loc: %s:%d\n", r.Symbol.Loc.File, r.Symbol.Loc.StartLine)
		fmt.Println()
	}
	if len(results) == 0 {
		fmt.Println("  (no matches)")
	}
	return nil
}
