package indexer

import (
	"encoding/json"

	"github.com/nullartist/digestron/internal/usg"
)

// ExtractStats reads the usg.Stats embedded in the raw JSON returned by
// RunTSExtract without unmarshalling the full USG.
func ExtractStats(raw json.RawMessage) usg.Stats {
	var tmp struct {
		Stats usg.Stats `json:"stats"`
	}
	_ = json.Unmarshal(raw, &tmp)
	return tmp.Stats
}
