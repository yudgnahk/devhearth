// Package assets defines the development asset graph: logical objects,
// locations, typed relationships, and evidence produced by detectors.
package assets

// Kind identifies the logical role of an asset in the inventory graph.
type Kind string

const (
	KindProject             Kind = "project"
	KindGitRepository       Kind = "git_repository"
	KindGitWorktree         Kind = "git_worktree"
	KindVersionManager      Kind = "version_manager"
	KindRuntime             Kind = "runtime"
	KindPackageManager      Kind = "package_manager"
	KindDependencyStore     Kind = "dependency_store"
	KindDownloadCache       Kind = "download_cache"
	KindProjectLocalInstall Kind = "project_local_install"
	KindBuildOutput         Kind = "build_output"
	KindContainerResource   Kind = "container_resource"
	KindAIAsset             Kind = "ai_asset"
	KindUnknown             Kind = "unknown"
)

// Class is the product taxonomy class for tools and storage (SPECS §7.3).
type Class string

const (
	ClassVersionManager      Class = "version_manager"
	ClassPackageManager      Class = "package_manager"
	ClassDependencyStore     Class = "dependency_store"
	ClassDownloadCache       Class = "download_cache"
	ClassProjectLocalInstall Class = "project_local_install"
	ClassBuildOutput         Class = "build_output"
	ClassProject             Class = "project"
	ClassGit                 Class = "git"
	ClassContainer           Class = "container"
	ClassAI                  Class = "ai"
	ClassOther               Class = "other"
)

// Risk is the conservative risk class for future optimization advice.
type Risk string

const (
	RiskInformational Risk = "informational"
	RiskLow           Risk = "low"
	RiskMedium        Risk = "medium"
	RiskHigh          Risk = "high"
	RiskProhibited    Risk = "prohibited"
)

// Evidence is a single factual observation supporting an asset or relationship.
type Evidence struct {
	Kind       string  `json:"kind"`
	Value      string  `json:"value"`
	Confidence float64 `json:"confidence"`
}

// Asset is a logical development object discovered during a scan.
type Asset struct {
	ID              string            `json:"id"`
	Kind            Kind              `json:"kind"`
	DisplayName     string            `json:"displayName"`
	Path            string            `json:"path"`
	Risk            Risk              `json:"risk"`
	Ecosystem       string            `json:"ecosystem,omitempty"`
	Class           Class             `json:"class,omitempty"`
	DetectorID      string            `json:"detectorId"`
	DetectorVersion int               `json:"detectorVersion"`
	Attributes      map[string]string `json:"attributes,omitempty"`
	Evidence        []Evidence        `json:"evidence,omitempty"`
}

// Relationship is a typed edge between two assets.
type Relationship struct {
	ID              string   `json:"id"`
	SourceID        string   `json:"sourceId"`
	TargetID        string   `json:"targetId"`
	Kind            string   `json:"kind"`
	Confidence      float64  `json:"confidence"`
	DetectorID      string   `json:"detectorId"`
	DetectorVersion int      `json:"detectorVersion"`
	Evidence        Evidence `json:"evidence"`
}

// Graph is the full asset inventory for one scan.
type Graph struct {
	Assets        []Asset        `json:"assets"`
	Relationships []Relationship `json:"relationships"`
}

// Relationship kinds used by Phase 2 detectors.
const (
	RelWorktreeBelongsToRepo     = "worktree_belongs_to_repository"
	RelProjectUsesPackageManager = "project_uses_package_manager"
	RelProjectOwnsEnvironment    = "project_owns_environment"
	RelProjectOwnsBuildOutput    = "project_owns_build_output"
	RelProjectHasGit             = "project_has_git"
	RelComposeReferencesProject  = "compose_references_project"
)

// PortfolioSummary aggregates installed tools and usage for an ecosystem.
type PortfolioSummary struct {
	Ecosystem            string         `json:"ecosystem"`
	ProjectCount         int            `json:"projectCount"`
	PackageManagers      map[string]int `json:"packageManagers,omitempty"`
	VersionManagers      []string       `json:"versionManagers,omitempty"`
	ProjectLocalBytes    int64          `json:"projectLocalBytes,omitempty"`
	SharedStoreCount     int            `json:"sharedStoreCount,omitempty"`
	DownloadCacheCount   int            `json:"downloadCacheCount,omitempty"`
	BuildOutputCount     int            `json:"buildOutputCount,omitempty"`
	DominantPackageTool  string         `json:"dominantPackageTool,omitempty"`
}
