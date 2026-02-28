package proto

import (
	"encoding/json"
	"testing"
)

func TestRequest_MarshalRoundTrip(t *testing.T) {
	req := Request{
		V:      Version,
		ID:     "req-1",
		Op:     "impact",
		Params: map[string]interface{}{"ref": "Foo", "radius": float64(2)},
	}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Request
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.V != Version || got.ID != "req-1" || got.Op != "impact" {
		t.Errorf("round-trip mismatch: %+v", got)
	}
	if got.Params["ref"] != "Foo" {
		t.Errorf("params[ref] = %v, want Foo", got.Params["ref"])
	}
}

func TestResponse_OkTrue(t *testing.T) {
	resp := Response{V: Version, ID: "1", Ok: true, Result: map[string]int{"count": 3}}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["ok"] != true {
		t.Errorf("ok = %v, want true", m["ok"])
	}
	if _, hasError := m["error"]; hasError {
		t.Errorf("error field should be omitted on success")
	}
}

func TestResponse_OkFalse(t *testing.T) {
	resp := Response{
		V:     Version,
		ID:    "2",
		Ok:    false,
		Error: &ErrorObj{Code: "NOT_INDEXED", Message: "run index first"},
	}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Response
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Ok {
		t.Error("ok should be false")
	}
	if got.Error == nil || got.Error.Code != "NOT_INDEXED" {
		t.Errorf("error mismatch: %+v", got.Error)
	}
	if got.Result != nil {
		t.Errorf("result should be nil on error, got: %v", got.Result)
	}
}
