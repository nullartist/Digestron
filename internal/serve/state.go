package serve

import (
	"sync"
	"time"

	"github.com/nullartist/digestron/internal/usg"
)

// RepoState holds the in-memory state for a single repository.
type RepoState struct {
	RepoRoot   string
	USG        *usg.USG
	View       *usg.View
	LastUsedAt time.Time
}

// State holds the in-memory server state shared across requests.
type State struct {
	mu          sync.RWMutex
	defaultRepo string
	repos       map[string]*RepoState
	maxRepos    int
}

// NewState creates a new State with the given default repo root.
func NewState(defaultRepo string) *State {
	return &State{
		defaultRepo: defaultRepo,
		repos:       map[string]*RepoState{},
		maxRepos:    5,
	}
}
