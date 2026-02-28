package util

import (
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// FindTSConfigsOptions controls tsconfig autodetection.
type FindTSConfigsOptions struct {
	MaxResults   int
	IncludeTests bool
}

// FindTSConfigs walks repoRoot and returns relative paths to tsconfig*.json files,
// ranked so that root tsconfig.json comes first, tsconfig.base.json second, then
// by path depth (ascending). Directories like node_modules, dist, and .next are
// skipped entirely.
func FindTSConfigs(repoRoot string, opt FindTSConfigsOptions) ([]string, error) {
	if opt.MaxResults <= 0 {
		opt.MaxResults = 50
	}

	var found []string

	ignoreDir := func(name string) bool {
		switch name {
		case "node_modules", "dist", "build", "out", ".next", ".turbo", ".git", ".digestron":
			return true
		default:
			return false
		}
	}

	ignoreFile := func(rel string) bool {
		r := strings.ToLower(rel)
		if !opt.IncludeTests {
			if strings.Contains(r, "test") && strings.Contains(r, "tsconfig") {
				return true
			}
			if strings.Contains(r, "__tests__") || strings.Contains(r, "/tests/") {
				return true
			}
		}
		return false
	}

	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if ignoreDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if name == "tsconfig.json" || (strings.HasPrefix(name, "tsconfig.") && strings.HasSuffix(name, ".json")) {
			rel, rerr := filepath.Rel(repoRoot, path)
			if rerr != nil {
				return nil
			}
			rel = filepath.ToSlash(rel)
			if ignoreFile(rel) {
				return nil
			}
			found = append(found, "./"+rel)
			if len(found) >= opt.MaxResults {
				return fs.SkipAll
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Rank: root tsconfig.json first, tsconfig.base.json second, then by depth.
	sort.SliceStable(found, func(i, j int) bool {
		a := found[i]
		b := found[j]

		score := func(p string) int {
			s := 0
			lp := strings.ToLower(p)
			if lp == "./tsconfig.json" {
				s += 1000
			}
			if strings.Contains(lp, "tsconfig.base") {
				s += 500
			}
			s -= strings.Count(lp, "/") * 5
			return s
		}

		sa, sb := score(a), score(b)
		if sa != sb {
			return sa > sb
		}
		return a < b
	})

	return found, nil
}
