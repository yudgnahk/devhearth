package protocol

import "encoding/json"

const (
	JSONRPCVersion  = "2.0"
	ProtocolVersion = 1
	SchemaVersion   = 1
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type HelloParams struct {
	ProtocolVersions []int `json:"protocolVersions"`
	SchemaVersions   []int `json:"schemaVersions"`
}

type HelloResult struct {
	Engine          string `json:"engine"`
	ProtocolVersion int    `json:"protocolVersion"`
	SchemaVersion   int    `json:"schemaVersion"`
	ReadOnly        bool   `json:"readOnly"`
}

type ScanStartParams struct {
	Roots        []string `json:"roots"`
	PolicyID     string   `json:"policyId,omitempty"`
	Cancellation string   `json:"cancellationToken,omitempty"`
}

type ScanStarted struct {
	ScanID string `json:"scanId"`
}

type ScanProgress struct {
	ScanID         string `json:"scanId"`
	Phase          string `json:"phase"`
	EntriesVisited int64  `json:"entriesVisited"`
	AllocatedBytes int64  `json:"allocatedBytes"`
	Complete       bool   `json:"complete"`
}
