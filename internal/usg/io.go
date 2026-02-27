package usg

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const outputDir = ".digestron"
const outputFile = "usg.v0.1.json"

// Save writes a USG to <root>/.digestron/usg.v0.1.json.
func Save(root string, u *USG) error {
	dir := filepath.Join(root, outputDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("usg: mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(u, "", "  ")
	if err != nil {
		return fmt.Errorf("usg: marshal: %w", err)
	}
	dest := filepath.Join(dir, outputFile)
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return fmt.Errorf("usg: write %s: %w", dest, err)
	}
	return nil
}

// Load reads a USG from <root>/.digestron/usg.v0.1.json.
func Load(root string) (*USG, error) {
	src := filepath.Join(root, outputDir, outputFile)
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("usg: read %s: %w", src, err)
	}
	var u USG
	if err := json.Unmarshal(data, &u); err != nil {
		return nil, fmt.Errorf("usg: unmarshal: %w", err)
	}
	return &u, nil
}

// OutputPath returns the path where the USG is written for a given repo root.
func OutputPath(root string) string {
	return filepath.Join(root, outputDir, outputFile)
}
