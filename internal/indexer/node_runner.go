package indexer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"time"
)

// ExtractRequest is the JSON payload sent to ts-extract via stdin.
type ExtractRequest struct {
	RepoRoot      string          `json:"repoRoot"`
	TsconfigPaths []string        `json:"tsconfigPaths"`
	IncludeTests  bool            `json:"includeTests"`
	MaxFiles      int             `json:"maxFiles"`
	Emit          map[string]bool `json:"emit"`
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

// RunTSExtract executes the ts-extract Node.js tool as a child process,
// sending a request via stdin and returning the parsed response.
func RunTSExtract(scriptPath string, repoRoot string, tsconfigs []string, includeTests bool) (*ExtractResponse, error) {
	req := ExtractRequest{
		RepoRoot:      repoRoot,
		TsconfigPaths: tsconfigs,
		IncludeTests:  includeTests,
		MaxFiles:      200000,
		Emit: map[string]bool{
			"modules":     true,
			"symbols":     true,
			"calls":       true,
			"inherits":    true,
			"instantiates": true,
			"entryPoints": true,
			"riskFlags":   true,
		},
	}

	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ts-extract: marshal request: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "node", scriptPath)
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
		return &resp, errors.New("ts-extract returned ok=false")
	}
	return &resp, nil
}
