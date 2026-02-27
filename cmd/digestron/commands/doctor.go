package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var doctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check the Digestron environment",
	Long:  `Validates that all required tools (Node.js, ts-extract deps) are available.`,
	RunE:  runDoctor,
}

func runDoctor(_ *cobra.Command, _ []string) error {
	ok := true

	// Check Node.js
	nodeOut, err := exec.Command("node", "--version").Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "  ✗  node not found in PATH")
		ok = false
	} else {
		ver := strings.TrimSpace(string(nodeOut))
		major := parseNodeMajor(ver)
		if major < 18 {
			fmt.Fprintf(os.Stderr, "  ✗  node version %s is too old (need >= 18)\n", ver)
			ok = false
		} else {
			fmt.Printf("  ✓  node %s\n", ver)
		}
	}

	// Check ts-extract node_modules
	tsExtractDir := filepath.Join("tools", "ts-extract")
	nmDir := filepath.Join(tsExtractDir, "node_modules")
	if _, err := os.Stat(nmDir); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "  ✗  tools/ts-extract/node_modules not found — run: cd %s && npm install\n", tsExtractDir)
		ok = false
	} else {
		fmt.Printf("  ✓  tools/ts-extract/node_modules present\n")
	}

	if !ok {
		return fmt.Errorf("doctor: environment checks failed")
	}
	fmt.Println("doctor: all checks passed")
	return nil
}

// parseNodeMajor parses "v20.1.0" -> 20.
func parseNodeMajor(ver string) int {
	v := strings.TrimPrefix(ver, "v")
	parts := strings.SplitN(v, ".", 2)
	if len(parts) == 0 {
		return 0
	}
	n, _ := strconv.Atoi(parts[0])
	return n
}
