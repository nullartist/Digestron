package serve

import "path/filepath"

// resolveRepo returns the absolute repo path for a request.
// If params contains a non-empty "repoRoot" string, that is used;
// otherwise the server default is returned.
func resolveRepo(st *State, params map[string]interface{}) (string, error) {
	if params != nil {
		if v, ok := params["repoRoot"].(string); ok && v != "" {
			return filepath.Abs(v)
		}
	}
	return st.defaultRepo, nil
}
