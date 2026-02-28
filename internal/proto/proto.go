// Package proto defines the NDJSON wire protocol for the Digestron stdio server.
//
// Protocol version: digestron.proto.v0.25
// Each request and response is a single JSON line (NDJSON).
package proto

// Version is the current protocol version string.
const Version = "digestron.proto.v0.25"

// Request is a single NDJSON request from a client.
type Request struct {
	V      string                 `json:"v"`
	ID     string                 `json:"id"`
	Op     string                 `json:"op"`
	Params map[string]interface{} `json:"params"`
}

// Response is a single NDJSON response sent to a client.
type Response struct {
	V      string      `json:"v"`
	ID     string      `json:"id"`
	Ok     bool        `json:"ok"`
	Result interface{} `json:"result,omitempty"`
	Error  *ErrorObj   `json:"error,omitempty"`
}

// ErrorObj carries a machine-readable code and a human-readable message.
type ErrorObj struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
