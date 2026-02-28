package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/nullartist/digestron/internal/util"
)

// ExtractRequest is the JSON payload sent to ts-extract via stdin.
type ExtractRequest struct {
	RepoRoot      string          `json:"repoRoot"`
	TsconfigPaths []string        `json:"tsconfigPaths"`
	IncludeTests  bool            `json:"includeTests"`
	MaxFiles      int             `json:"maxFiles"`
	SampleFiles   int             `json:"sampleFiles,omitempty"` // only process first N files (ts-morph side)
	ForceEngine   string          `json:"forceEngine,omitempty"` // "ts-morph" | "tsc-api"
	Emit          map[string]bool `json:"emit"`
}

// RunOptions groups the options for RunTSExtract.
type RunOptions struct {
	Tsconfigs    []string
	IncludeTests bool
	SampleFiles  int
	ForceEngine  string
	Emit         map[string]bool
}

// Diagnostic is a single diagnostic message from ts-extract.
type Diagnostic struct {
	Level   string `json:"level"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ExtractResponse is the JSON payload received from ts-extract via stdout.
type ExtractResponse struct {
	Ok          bool            `json:"ok"`
	ToolVersion string          `json:"toolVersion"`
	Engine      string          `json:"engine"`
	Diagnostics []Diagnostic    `json:"diagnostics"`
	Raw         json.RawMessage `json:"raw"`
}

// RunTSExtract executes the ts-extract Node.js tool as a child process.
// If opt.Tsconfigs is empty, tsconfig paths are auto-detected from repoRoot.
func RunTSExtract(repoRoot string, opt RunOptions) (*ExtractResponse, error) {
	tsconfigs := opt.Tsconfigs
	if len(tsconfigs) == 0 {
		auto, err := util.FindTSConfigs(repoRoot, util.FindTSConfigsOptions{
			MaxResults:   50,
			IncludeTests: opt.IncludeTests,
		})
		if err != nil {
			return nil, fmt.Errorf("ts-extract: auto-detect tsconfigs: %w", err)
		}
		tsconfigs = auto
	}

	emit := opt.Emit
	if emit == nil {
		emit = map[string]bool{
			"modules":      true,
			"symbols":      true,
			"calls":        true,
			"inherits":     true,
			"instantiates": true,
			"entryPoints":  true,
			"riskFlags":    true,
		}
	}

	req := ExtractRequest{
		RepoRoot:      repoRoot,
		TsconfigPaths: tsconfigs,
		IncludeTests:  opt.IncludeTests,
		MaxFiles:      200000,
		SampleFiles:   opt.SampleFiles,
		ForceEngine:   opt.ForceEngine,
		Emit:          emit,
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ts-extract: marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Script path is resolved from the process CWD (where digestron is run from).
	script, err := filepath.Abs(filepath.Join("tools", "ts-extract", "src", "index.mjs"))
	if err != nil {
		return nil, fmt.Errorf("ts-extract: resolve script path: %w", err)
	}

	cmd := exec.CommandContext(ctx, "node", script)
	cmd.Stdin = bytes.NewReader(payload)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := stderr.String()
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("ts-extract failed: %s", msg)
	}

	var resp ExtractResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("invalid ts-extract json: %w\nraw: %s", err, stdout.String())
	}
	if !resp.Ok {
		return &resp, fmt.Errorf("ts-extract returned ok=false")
	}
	return &resp, nil
}
