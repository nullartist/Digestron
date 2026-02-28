package commands

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/nullartist/digestron/internal/indexer"
	"github.com/nullartist/digestron/internal/util"
)

var doctorTsconfig string

var doctorCmd = &cobra.Command{
	Use:   "doctor [path]",
	Short: "Check the Digestron environment",
	Long: `Validates that all required tools (Node.js, ts-extract deps) are available.
Optionally checks for tsconfig.json files under [path] (default: current directory).`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoctor,
}

func init() {
	doctorCmd.Flags().StringVar(&doctorTsconfig, "tsconfig", "", "Explicit tsconfig path to validate (optional)")
}

func runDoctor(_ *cobra.Command, args []string) error {
	root := "."
	if len(args) == 1 {
		root = args[0]
	}
	absRoot, _ := filepath.Abs(root)

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

	// Check ts-extract node_modules; install if absent using npm ci (with lockfile) or npm install.
	tsExtractDir := filepath.Join("tools", "ts-extract")
	nmDir := filepath.Join(tsExtractDir, "node_modules")
	lockFile := filepath.Join(tsExtractDir, "package-lock.json")
	useCI := fileExists(lockFile)
	if _, err := os.Stat(nmDir); os.IsNotExist(err) {
		npmBin, lookErr := exec.LookPath("npm")
		if lookErr != nil {
			fmt.Fprintf(os.Stderr, "  ⚠  npm not found in PATH, trying 'npm' anyway\n")
			npmBin = "npm"
		}
		if useCI {
			fmt.Println("  • node_modules not found. Running npm ci in tools/ts-extract ...")
			if out, err := runCmd(tsExtractDir, npmBin, "ci"); err != nil {
				fmt.Fprintf(os.Stderr, "  ✗  npm ci failed: %v\n%s\n", err, out)
				ok = false
			} else {
				fmt.Println("  ✓  npm dependencies installed (ci)")
			}
		} else {
			fmt.Println("  • node_modules not found. Running npm install in tools/ts-extract ...")
			if out, err := runCmd(tsExtractDir, npmBin, "install"); err != nil {
				fmt.Fprintf(os.Stderr, "  ✗  npm install failed: %v\n%s\n", err, out)
				ok = false
			} else {
				fmt.Println("  ✓  npm dependencies installed")
			}
		}
	} else {
		fmt.Printf("  ✓  tools/ts-extract/node_modules present\n")
	}

	// Detect tsconfigs in the target path
	if doctorTsconfig != "" {
		abs := doctorTsconfig
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(absRoot, abs)
		}
		if _, err := os.Stat(abs); err != nil {
			fmt.Fprintf(os.Stderr, "  ✗  tsconfig not found: %s\n", abs)
			ok = false
		} else {
			fmt.Printf("  ✓  tsconfig: %s\n", abs)
		}
	} else {
		detected, _ := util.FindTSConfigs(absRoot, util.FindTSConfigsOptions{MaxResults: 20})
		if len(detected) == 0 {
			fmt.Printf("  ⚠  no tsconfig.json detected under %s\n", absRoot)
		} else {
			fmt.Println("  ✓  tsconfig candidates:")
			for i, p := range detected {
				if i >= 10 {
					fmt.Printf("      - +%d more\n", len(detected)-10)
					break
				}
				fmt.Printf("      - %s\n", p)
			}
		}
	}

	if !ok {
		return fmt.Errorf("doctor: environment checks failed")
	}

	// mini-index health check
	fmt.Println("• mini-index health check (ts-morph)...")
	sampleOpts := indexer.RunOptions{
		IncludeTests: false,
		SampleFiles:  120,
		ForceEngine:  "ts-morph",
		Emit: map[string]bool{
			"modules": true, "symbols": true, "calls": true,
			"inherits": true, "instantiates": true, "entryPoints": true, "riskFlags": true,
		},
	}
	morph, morphErr := indexer.RunTSExtract(absRoot, sampleOpts)
	if morphErr != nil {
		fmt.Println("  ! ts-morph mini-index failed:", morphErr)
	} else {
		stats := indexer.ExtractStats(morph.Raw)
		fmt.Printf("  ts-morph: resolvedEdgeRatio=%.2f dynamicRatio=%.2f confidence=%.2f\n",
			stats.ResolvedEdgeRatio, stats.DynamicRatio, stats.StructuralConfidence)

		needsCompare := stats.ResolvedEdgeRatio < 0.55 || stats.DynamicRatio > 0.20
		if needsCompare {
			fmt.Println("  • ts-morph looks structurally weak; comparing with tsc-api...")
			tscOpts := sampleOpts
			tscOpts.ForceEngine = "tsc-api"
			tsc, tscErr := indexer.RunTSExtract(absRoot, tscOpts)
			if tscErr != nil {
				fmt.Println("  ! tsc-api mini-index failed:", tscErr)
			} else {
				stats2 := indexer.ExtractStats(tsc.Raw)
				fmt.Printf("  tsc-api:  resolvedEdgeRatio=%.2f dynamicRatio=%.2f confidence=%.2f\n",
					stats2.ResolvedEdgeRatio, stats2.DynamicRatio, stats2.StructuralConfidence)
				if stats2.StructuralConfidence > stats.StructuralConfidence+0.05 {
					fmt.Println("  ✓ Recommendation: use tsc-api for this repo (more reliable resolution).")
					fmt.Println("    Tip: digestron index --engine tsc-api")
				} else {
					fmt.Println("  ✓ Recommendation: keep ts-morph (tsc-api not significantly better).")
				}
			}
		}
	}

	fmt.Println("doctor: all checks passed")
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runCmd runs a command in dir and returns combined output.
func runCmd(dir, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.Bytes(), err
	}
	return out.Bytes(), nil
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
