package commands

import (
	"github.com/nullartist/digestron/internal/serve"
	"github.com/spf13/cobra"
)

var serveCmd = &cobra.Command{
	Use:   "serve [path]",
	Short: "Run Digestron as a stdio server (NDJSON) for editors and MCP clients",
	Long: `Starts a persistent Digestron process that reads NDJSON requests from stdin
and writes NDJSON responses to stdout.

Protocol version: digestron.proto.v0.25

Supported ops: doctor, health, index, search, impact, snippets`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repo := "."
		if len(args) == 1 {
			repo = args[0]
		}
		return serve.Run(repo)
	},
}

func init() {
	rootCmd.AddCommand(serveCmd)
}
