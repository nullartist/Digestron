package serve

import (
	"time"

	"github.com/nullartist/digestron/internal/usg"
)

// getOrWarm returns the RepoState for repoAbs, loading from disk if available.
// It evicts the least-recently-used repo entry if the LRU limit is exceeded.
func (st *State) getOrWarm(repoAbs string) (*RepoState, error) {
	st.mu.Lock()
	defer st.mu.Unlock()

	if rs, ok := st.repos[repoAbs]; ok {
		rs.LastUsedAt = time.Now()
		return rs, nil
	}

	if len(st.repos) >= st.maxRepos {
		st.evictOneLocked()
	}

	rs := &RepoState{RepoRoot: repoAbs, LastUsedAt: time.Now()}
	// Warm-load from disk if a previously indexed USG exists.
	if g, err := usg.Load(repoAbs); err == nil && g != nil {
		rs.USG = g
		rs.View = usg.BuildView(g)
	}
	st.repos[repoAbs] = rs
	return rs, nil
}

// evictOneLocked removes the least-recently-used repo from the map.
// Must be called with st.mu held for writing.
func (st *State) evictOneLocked() {
	var oldestKey string
	var oldest time.Time
	first := true
	for k, v := range st.repos {
		if first || v.LastUsedAt.Before(oldest) {
			first = false
			oldest = v.LastUsedAt
			oldestKey = k
		}
	}
	if oldestKey != "" {
		delete(st.repos, oldestKey)
	}
}
