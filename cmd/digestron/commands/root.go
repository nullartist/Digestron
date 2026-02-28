package commands

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootFlags holds flags that are shared across commands.
var rootFlags struct {
	Config string
}

var rootCmd = &cobra.Command{
	Use:   "digestron",
	Short: "Digestron — structural codebase indexer for LLMs",
	Long: `Digestron builds a Universal Symbol Graph (USG) of your codebase
and serves impact-aware context slices to LLMs.`,
}

// Execute runs the root command.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().StringVar(&rootFlags.Config, "config", "", "Path to digestron config file (default: auto)")

	rootCmd.AddCommand(doctorCmd)
	rootCmd.AddCommand(indexCmd)
	rootCmd.AddCommand(analyzeCmd)
	rootCmd.AddCommand(impactCmd)
}
