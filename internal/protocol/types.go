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

type ScanCancelParams struct {
	ScanID string `json:"scanId"`
}
type ScanStatusParams struct {
	ScanID string `json:"scanId"`
}
type ReportExportParams struct {
	ScanID string `json:"scanId"`
}

type AssetsListParams struct {
	ScanID string `json:"scanId"`
}

type AssetsGetParams struct {
	ScanID  string `json:"scanId"`
	AssetID string `json:"assetId"`
}

type PortfolioListParams struct {
	ScanID string `json:"scanId"`
}

type ScanStatus struct {
	ScanID string `json:"scanId"`
	Status string `json:"status"`
}

type ScanProgress struct {
	ScanID         string `json:"scanId"`
	Phase          string `json:"phase"`
	EntriesVisited int64  `json:"entriesVisited"`
	AllocatedBytes int64  `json:"allocatedBytes"`
	AssetsFound    int64  `json:"assetsFound,omitempty"`
	Complete       bool   `json:"complete"`
}

type ScanReport struct {
	ScanID         string             `json:"scanId"`
	Status         string             `json:"status"`
	Roots          []string           `json:"roots"`
	EntriesVisited int64              `json:"entriesVisited"`
	LogicalBytes   int64              `json:"logicalBytes"`
	AllocatedBytes int64              `json:"allocatedBytes"`
	Inaccessible   []InaccessiblePath `json:"inaccessible"`
	AssetCount     int                `json:"assetCount"`
	AssetsByKind   map[string]int     `json:"assetsByKind,omitempty"`
	Portfolio      []PortfolioSummary `json:"portfolio,omitempty"`
}

type InaccessiblePath struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type AssetSummary struct {
	ID              string            `json:"id"`
	Kind            string            `json:"kind"`
	DisplayName     string            `json:"displayName"`
	Path            string            `json:"path"`
	Risk            string            `json:"risk"`
	Ecosystem       string            `json:"ecosystem,omitempty"`
	Class           string            `json:"class,omitempty"`
	DetectorID      string            `json:"detectorId"`
	DetectorVersion int               `json:"detectorVersion"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	Evidence        []EvidenceSummary `json:"evidence,omitempty"`
}

type EvidenceSummary struct {
	Kind       string  `json:"kind"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

type RelationshipSummary struct {
	ID         string  `json:"id"`
	SourceID   string  `json:"sourceId"`
	TargetID   string  `json:"targetId"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
	DetectorID string  `json:"detectorId,omitempty"`
}

type AssetsListResult struct {
	ScanID        string                `json:"scanId"`
	Assets        []AssetSummary        `json:"assets"`
	Relationships []RelationshipSummary `json:"relationships"`
}

type AssetsGetResult struct {
	ScanID string       `json:"scanId"`
	Asset  AssetSummary `json:"asset"`
}

type PortfolioSummary struct {
	Ecosystem           string         `json:"ecosystem"`
	ProjectCount        int            `json:"projectCount"`
	PackageManagers     map[string]int `json:"packageManagers,omitempty"`
	VersionManagers     []string       `json:"versionManagers,omitempty"`
	SharedStoreCount    int            `json:"sharedStoreCount,omitempty"`
	DownloadCacheCount  int            `json:"downloadCacheCount,omitempty"`
	BuildOutputCount    int            `json:"buildOutputCount,omitempty"`
	DominantPackageTool string         `json:"dominantPackageTool,omitempty"`
}

type PortfolioListResult struct {
	ScanID    string             `json:"scanId"`
	Portfolio []PortfolioSummary `json:"portfolio"`
}
