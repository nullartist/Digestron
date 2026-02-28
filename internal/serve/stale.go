package serve

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RepoFreshness holds the result of comparing the USG on-disk mod time
// against the latest relevant source-file modification time in the repo.
type RepoFreshness struct {
	USGPath        string    `json:"usgPath"`
	USGModTime     time.Time `json:"usgModTime"`
	RepoLatestTime time.Time `json:"repoLatestTime"`
	RepoLatestFile string    `json:"repoLatestFile"`
	IsStale        bool      `json:"isStale"`
}

// FreshnessOptions controls which files are considered when scanning the repo.
type FreshnessOptions struct {
	IncludeJS    bool
	IncludeTests bool
	MaxFiles     int
}

// CheckRepoFreshness walks repoRoot and determines whether the on-disk USG is
// stale relative to the most-recently-modified relevant source file.
func CheckRepoFreshness(repoRoot string, opt FreshnessOptions) (RepoFreshness, error) {
	if opt.MaxFiles <= 0 {
		opt.MaxFiles = 300000
	}

	usgPath := filepath.Join(repoRoot, ".digestron", "usg.v0.1.json")
	var usgTime time.Time
	if st, err := os.Stat(usgPath); err == nil {
		usgTime = st.ModTime()
	}

	ignoreDir := func(name string) bool {
		switch name {
		case "node_modules", "dist", "build", "out", ".next", ".turbo", ".git", ".digestron":
			return true
		default:
			return false
		}
	}

	isRelevant := func(path string) bool {
		l := strings.ToLower(path)
		base := strings.ToLower(filepath.Base(path))

		// ---- Config & lock files (always relevant, regardless of test filters) ----
		switch base {
		case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "bun.lockb",
			"tsconfig.json", "turbo.json", "nx.json", ".nvmrc":
			return true
		}
		// tsconfig.*.json
		if strings.HasPrefix(base, "tsconfig.") && strings.HasSuffix(base, ".json") {
			return true
		}
		// common build configs
		for _, p := range []string{
			"vite.config.", "webpack.config.", "rollup.config.", "esbuild.config.", "next.config.",
		} {
			if strings.HasPrefix(base, p) {
				return true
			}
		}

		// ---- Code files (subject to test filters) ----
		if !opt.IncludeTests {
			if strings.Contains(l, "__tests__") || strings.Contains(l, "/tests/") || strings.Contains(l, "\\tests\\") {
				if strings.HasSuffix(l, ".ts") || strings.HasSuffix(l, ".tsx") || strings.HasSuffix(l, ".js") || strings.HasSuffix(l, ".jsx") {
					return false
				}
			}
			if strings.Contains(l, ".spec.") || strings.Contains(l, ".test.") {
				return false
			}
		}
		if strings.HasSuffix(l, ".ts") || strings.HasSuffix(l, ".tsx") {
			return true
		}
		if opt.IncludeJS && (strings.HasSuffix(l, ".js") || strings.HasSuffix(l, ".jsx") || strings.HasSuffix(l, ".mjs") || strings.HasSuffix(l, ".cjs")) {
			return true
		}
		return false
	}

	var latest time.Time
	var latestFile string
	seen := 0

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
		seen++
		if seen > opt.MaxFiles {
			return fs.SkipAll
		}
		if !isRelevant(path) {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		mt := info.ModTime()
		if mt.After(latest) {
			latest = mt
			latestFile = path
		}
		return nil
	})
	if err != nil {
		return RepoFreshness{}, err
	}

	// Stale only when USG already exists and a source file is newer than it.
	isStale := !usgTime.IsZero() && latest.After(usgTime)

	return RepoFreshness{
		USGPath:        usgPath,
		USGModTime:     usgTime,
		RepoLatestTime: latest,
		RepoLatestFile: latestFile,
		IsStale:        isStale,
	}, nil
}
