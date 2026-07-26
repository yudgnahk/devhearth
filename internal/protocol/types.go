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

// InventoryChildrenParams lists direct children under a directory for drill-down.
// PathKey is the absolute session-local path from a previous response (empty for roots).
// Paths are never logged by the server.
type InventoryChildrenParams struct {
	ScanID  string `json:"scanId"`
	PathKey string `json:"pathKey,omitempty"`
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

type FitListParams struct {
	ScanID string `json:"scanId"`
}

type FitGetParams struct {
	ScanID    string `json:"scanId"`
	Ecosystem string `json:"ecosystem"`
}

type RecommendationsListParams struct {
	ScanID string `json:"scanId"`
	// Family optionally narrows the inbox to one recommendation family.
	Family string `json:"family,omitempty"`
}

type RecommendationsGetParams struct {
	ScanID           string `json:"scanId"`
	RecommendationID string `json:"recommendationId"`
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
	// RecommendationsFound reports advice produced after the "advice" phase.
	RecommendationsFound int64 `json:"recommendationsFound,omitempty"`
	// RowsWritten/RowsTotal report durable inventory rows during phase "persist".
	RowsWritten int64 `json:"rowsWritten,omitempty"`
	RowsTotal   int64 `json:"rowsTotal,omitempty"`
	Complete    bool  `json:"complete"`
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
	Advice         *AdviceSummary     `json:"advice,omitempty"`
}

// AdviceSummary is the report-level roll-up of Phase 3 analysis. Savings stay
// ranges, and the total is a sum of ranges rather than a single figure.
type AdviceSummary struct {
	RecommendationCount    int            `json:"recommendationCount"`
	ByFamily               map[string]int `json:"byFamily,omitempty"`
	ByRisk                 map[string]int `json:"byRisk,omitempty"`
	BlockedCount           int            `json:"blockedCount"`
	SavingsLowBytes        int64          `json:"savingsLowBytes"`
	SavingsHighBytes       int64          `json:"savingsHighBytes"`
	SavingsUncertain       bool           `json:"savingsUncertain,omitempty"`
	Fit                    []FitHeadline  `json:"fit,omitempty"`
	TopRecommendationTitle string         `json:"topRecommendationTitle,omitempty"`
}

// FitHeadline is the one-line fit outcome per ecosystem for reports.
type FitHeadline struct {
	Ecosystem       string  `json:"ecosystem"`
	Depth           string  `json:"depth"`
	Baseline        string  `json:"baseline,omitempty"`
	RecommendedTool string  `json:"recommendedTool,omitempty"`
	StayPutWins     bool    `json:"stayPutWins"`
	Confidence      float64 `json:"confidence,omitempty"`
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
	Size            AssetSize         `json:"size"`
	// LastActivityAt is RFC 3339 or empty when activity is unknown.
	LastActivityAt string `json:"lastActivityAt,omitempty"`
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
	Ecosystem                string         `json:"ecosystem"`
	ProjectCount             int            `json:"projectCount"`
	PackageManagers          map[string]int `json:"packageManagers,omitempty"`
	VersionManagers          []string       `json:"versionManagers,omitempty"`
	ProjectLocalInstallCount int            `json:"projectLocalInstallCount,omitempty"`
	SharedStoreCount         int            `json:"sharedStoreCount,omitempty"`
	DownloadCacheCount       int            `json:"downloadCacheCount,omitempty"`
	BuildOutputCount         int            `json:"buildOutputCount,omitempty"`
	DominantPackageTool      string         `json:"dominantPackageTool,omitempty"`
	ProjectLocalInstallBytes int64          `json:"projectLocalInstallBytes,omitempty"`
	SharedStoreBytes         int64          `json:"sharedStoreBytes,omitempty"`
	DownloadCacheBytes       int64          `json:"downloadCacheBytes,omitempty"`
	BuildOutputBytes         int64          `json:"buildOutputBytes,omitempty"`
	SizesUncertain           bool           `json:"sizesUncertain,omitempty"`
}

type PortfolioListResult struct {
	ScanID    string             `json:"scanId"`
	Portfolio []PortfolioSummary `json:"portfolio"`
}

// AssetSize is attributed storage for an asset, mirrored onto the wire so the
// UI can show project-local versus shared bytes without recomputing anything.
type AssetSize struct {
	Attributed              bool  `json:"attributed"`
	LogicalBytes            int64 `json:"logicalBytes"`
	AllocatedBytes          int64 `json:"allocatedBytes"`
	ExclusiveAllocatedBytes int64 `json:"exclusiveAllocatedBytes"`
	Shared                  bool  `json:"shared,omitempty"`
	Uncertain               bool  `json:"uncertain,omitempty"`
}

// FitFactor is one weighted signal behind a fit option.
type FitFactor struct {
	Kind   string  `json:"kind"`
	Score  float64 `json:"score"`
	Weight float64 `json:"weight"`
	Detail string  `json:"detail"`
}

// FitOption is one ranked portfolio option.
type FitOption struct {
	Tool          string `json:"tool"`
	StayPut       bool   `json:"stayPut"`
	Installed     bool   `json:"installed"`
	ProjectsUsing int    `json:"projectsUsing"`

	Rank       int     `json:"rank"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`

	Factors         []FitFactor `json:"factors,omitempty"`
	DominantFactors []string    `json:"dominantFactors,omitempty"`
	Blockers        []string    `json:"blockers,omitempty"`
	WorkflowImpact  string      `json:"workflowImpact,omitempty"`

	ImmediateSavingsLowBytes   int64 `json:"immediateSavingsLowBytes"`
	ImmediateSavingsHighBytes  int64 `json:"immediateSavingsHighBytes"`
	FutureGrowthReductionBytes int64 `json:"futureGrowthReductionBytes,omitempty"`
	SavingsUncertain           bool  `json:"savingsUncertain,omitempty"`
}

// FitAssessment is the ranked fit result for one ecosystem.
type FitAssessment struct {
	Ecosystem                string      `json:"ecosystem"`
	Depth                    string      `json:"depth"`
	ProjectCount             int         `json:"projectCount"`
	Baseline                 string      `json:"baseline,omitempty"`
	RecommendedTool          string      `json:"recommendedTool,omitempty"`
	StayPutWins              bool        `json:"stayPutWins"`
	Options                  []FitOption `json:"options,omitempty"`
	Notes                    []string    `json:"notes,omitempty"`
	ProjectLocalInstallBytes int64       `json:"projectLocalInstallBytes,omitempty"`
	SharedStoreBytes         int64       `json:"sharedStoreBytes,omitempty"`
	VersionManagers          []string    `json:"versionManagers,omitempty"`
}

type FitListResult struct {
	ScanID string          `json:"scanId"`
	Fit    []FitAssessment `json:"fit"`
}

type FitGetResult struct {
	ScanID string        `json:"scanId"`
	Fit    FitAssessment `json:"fit"`
}

// RecommendationSavings is an estimated storage outcome as a range.
type RecommendationSavings struct {
	LowBytes                   int64 `json:"lowBytes"`
	HighBytes                  int64 `json:"highBytes"`
	FutureGrowthReductionBytes int64 `json:"futureGrowthReductionBytes,omitempty"`
	Uncertain                  bool  `json:"uncertain,omitempty"`
}

// RecommendationAlternative is an option that was considered and not chosen.
type RecommendationAlternative struct {
	Label            string   `json:"label"`
	StayPut          bool     `json:"stayPut,omitempty"`
	Rank             int      `json:"rank"`
	Score            float64  `json:"score"`
	SavingsLowBytes  int64    `json:"savingsLowBytes,omitempty"`
	SavingsHighBytes int64    `json:"savingsHighBytes,omitempty"`
	Blockers         []string `json:"blockers,omitempty"`
}

// AffectedAsset identifies an asset a recommendation would touch, with a
// redacted display path.
type AffectedAsset struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
	Path        string `json:"path"`
}

// RecommendationSummary is one entry in the recommendation inbox. Every field a
// user needs to accept or reject the advice travels with it.
type RecommendationSummary struct {
	ID        string `json:"id"`
	Family    string `json:"family"`
	Title     string `json:"title"`
	Ecosystem string `json:"ecosystem,omitempty"`

	Explanation         string                `json:"explanation"`
	Risk                string                `json:"risk"`
	Confidence          float64               `json:"confidence"`
	Priority            float64               `json:"priority"`
	Savings             RecommendationSavings `json:"savings"`
	RestorationCost     string                `json:"restorationCost,omitempty"`
	CompatibilityImpact string                `json:"compatibilityImpact,omitempty"`

	Preconditions   []string `json:"preconditions,omitempty"`
	ProposedActions []string `json:"proposedActions,omitempty"`
	Verification    []string `json:"verification,omitempty"`
	Rollback        string   `json:"rollback,omitempty"`
	Blockers        []string `json:"blockers,omitempty"`

	AffectedAssets  []AffectedAsset             `json:"affectedAssets,omitempty"`
	Evidence        []EvidenceSummary           `json:"evidence,omitempty"`
	Alternatives    []RecommendationAlternative `json:"alternatives,omitempty"`
	DominantFactors []string                    `json:"dominantFactors,omitempty"`

	RuleID      string `json:"ruleId"`
	RuleVersion int    `json:"ruleVersion"`
	// AdviceOnly restates the trust model on the wire: this protocol version has
	// no method that can execute a recommendation. Mutation methods must arrive
	// in a later protocol version, and a client must never infer permission to
	// act from the presence of proposed actions.
	AdviceOnly bool `json:"adviceOnly"`
}

type RecommendationsListResult struct {
	ScanID          string                  `json:"scanId"`
	Recommendations []RecommendationSummary `json:"recommendations"`
}

type RecommendationsGetResult struct {
	ScanID         string                `json:"scanId"`
	Recommendation RecommendationSummary `json:"recommendation"`
}

// DirectoryChild is one inventory row for directory drill-down (redacted display path).
type DirectoryChild struct {
	Name                string `json:"name"`
	Path                string `json:"path"`    // redacted for display / export
	PathKey             string `json:"pathKey"` // absolute path for subsequent inventory.children calls
	Kind                string `json:"kind"`
	LogicalBytes        int64  `json:"logicalBytes"`
	AllocatedBytes      int64  `json:"allocatedBytes"`
	TotalLogicalBytes   int64  `json:"totalLogicalBytes"`
	TotalAllocatedBytes int64  `json:"totalAllocatedBytes"`
	DirectChildCount    int    `json:"directChildCount,omitempty"`
	IsSymlink           bool   `json:"isSymlink,omitempty"`
}

type InventoryChildrenResult struct {
	ScanID    string           `json:"scanId"`
	PathKey   string           `json:"pathKey,omitempty"`
	Path      string           `json:"path"` // redacted parent path
	ParentKey string           `json:"parentKey,omitempty"`
	Children  []DirectoryChild `json:"children"`
}
