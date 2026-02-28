package serve

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/nullartist/digestron/internal/proto"
	"github.com/nullartist/digestron/internal/usg"
)

// helpers

func makeState(g *usg.USG) *State {
	st := &State{repoRoot: mockTempDir()}
	if g != nil {
		st.graph = g
		st.view = usg.BuildView(g)
	}
	return st
}

// mockTempDir returns a placeholder directory path used as repoRoot in tests
// that do not perform real file I/O.
func mockTempDir() string { return "/tmp/digestron-serve-test" }

func req(op string, params map[string]interface{}) proto.Request {
	return proto.Request{V: proto.Version, ID: "test-1", Op: op, Params: params}
}

func sym(id, qname, name, kind, file string, line int) usg.Symbol {
	return usg.Symbol{
		ID:    id,
		QName: qname,
		Name:  name,
		Kind:  kind,
		Loc:   usg.Location{File: file, StartLine: line},
	}
}

func strPtr(s string) *string { return &s }

// ---- handle() tests ----

func TestHandle_Doctor(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("doctor", nil))
	if !resp.Ok {
		t.Errorf("doctor: expected ok=true, got %+v", resp.Error)
	}
}

func TestHandle_Health_NotIndexed(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("health", nil))
	if !resp.Ok {
		t.Errorf("health: expected ok=true")
	}
	m, _ := resp.Result.(map[string]any)
	if m["indexed"] != false {
		t.Errorf("health.indexed should be false when no USG loaded")
	}
}

func TestHandle_Health_Indexed(t *testing.T) {
	g := &usg.USG{Symbols: []usg.Symbol{sym("s1", "a::Foo", "Foo", "function", "a.ts", 1)}}
	st := makeState(g)
	resp := handle(st, req("health", nil))
	m, _ := resp.Result.(map[string]any)
	if m["indexed"] != true {
		t.Errorf("health.indexed should be true when USG loaded")
	}
}

func TestHandle_UnknownOp(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("bogus", nil))
	if resp.Ok {
		t.Error("expected ok=false for unknown op")
	}
	if resp.Error == nil || resp.Error.Code != "UNKNOWN_OP" {
		t.Errorf("expected UNKNOWN_OP error, got %+v", resp.Error)
	}
}

func TestHandle_Search_NotIndexed(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("search", map[string]interface{}{"query": "Foo"}))
	if resp.Ok {
		t.Error("expected ok=false when not indexed")
	}
	if resp.Error.Code != "NOT_INDEXED" {
		t.Errorf("expected NOT_INDEXED, got %s", resp.Error.Code)
	}
}

func TestHandle_Search_Found(t *testing.T) {
	g := &usg.USG{Symbols: []usg.Symbol{sym("s1", "src/a.ts::Foo", "Foo", "function", "src/a.ts", 5)}}
	st := makeState(g)
	resp := handle(st, req("search", map[string]interface{}{"query": "Foo"}))
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp.Error)
	}
	m := resp.Result.(map[string]any)
	b, _ := json.Marshal(m)
	var out map[string]interface{}
	json.Unmarshal(b, &out)
	hitsRaw, _ := out["hits"].([]interface{})
	if len(hitsRaw) == 0 {
		t.Errorf("expected at least 1 hit")
	}
}

func TestHandle_Search_NoQuery(t *testing.T) {
	g := &usg.USG{}
	st := makeState(g)
	resp := handle(st, req("search", map[string]interface{}{}))
	if resp.Ok {
		t.Error("expected ok=false for empty query")
	}
	if resp.Error.Code != "BAD_PARAMS" {
		t.Errorf("expected BAD_PARAMS, got %s", resp.Error.Code)
	}
}

func TestHandle_Impact_NotIndexed(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("impact", map[string]interface{}{"ref": "Foo"}))
	if resp.Ok || resp.Error.Code != "NOT_INDEXED" {
		t.Errorf("expected NOT_INDEXED, got %+v", resp)
	}
}

func TestHandle_Impact_Found(t *testing.T) {
	g := &usg.USG{
		Symbols: []usg.Symbol{sym("s1", "src/a.ts::Foo", "Foo", "function", "src/a.ts", 5)},
		Stats:   usg.Stats{StructuralConfidence: 0.9},
	}
	st := makeState(g)
	resp := handle(st, req("impact", map[string]interface{}{"ref": "Foo"}))
	if !resp.Ok {
		t.Fatalf("expected ok=true, got %+v", resp.Error)
	}
	m := resp.Result.(map[string]any)
	if _, ok := m["focusText"]; !ok {
		t.Error("expected focusText in result")
	}
	if _, ok := m["focus"]; !ok {
		t.Error("expected focus in result")
	}
}

func TestHandle_Snippets_BadParams(t *testing.T) {
	st := makeState(nil)
	resp := handle(st, req("snippets", map[string]interface{}{}))
	if resp.Ok {
		t.Error("expected ok=false for missing locs")
	}
	if resp.Error.Code != "BAD_PARAMS" {
		t.Errorf("expected BAD_PARAMS, got %s", resp.Error.Code)
	}
}

func TestHandle_Snippets_OK(t *testing.T) {
	st := makeState(nil)
	// Provide locs pointing to a file that doesn't exist — Build will skip it
	// gracefully and return an empty but successful result.
	locs := []interface{}{
		map[string]interface{}{"file": "nonexistent.ts", "line": float64(1)},
	}
	resp := handle(st, req("snippets", map[string]interface{}{"locs": locs}))
	if !resp.Ok {
		t.Fatalf("expected ok=true even for missing file, got %+v", resp.Error)
	}
}

// ---- writeResp + Run integration test ----

func TestWriteResp_NDJSON(t *testing.T) {
	var buf bytes.Buffer
	w := bufio.NewWriter(&buf)
	writeResp(w, proto.Response{ID: "x", Ok: true, Result: map[string]string{"a": "b"}})

	line := strings.TrimRight(buf.String(), "\n")
	var got proto.Response
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("invalid NDJSON: %v\n%s", err, line)
	}
	if got.V != proto.Version {
		t.Errorf("V = %q, want %q", got.V, proto.Version)
	}
	if !got.Ok {
		t.Error("ok should be true")
	}
}
