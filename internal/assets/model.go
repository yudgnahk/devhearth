// Package assets defines the development asset graph: logical objects,
// locations, typed relationships, and evidence produced by detectors.
package assets

import "time"

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

// Size is attributed storage for one asset. Allocated bytes come from
// filesystem metadata and already exclude hard-link aliases seen earlier in the
// same scan, so a subtree containing aliases is a lower bound (Uncertain).
type Size struct {
	// Attributed is false for assets that do not own storage of their own, such
	// as a package manager whose path is the project directory it configures.
	Attributed bool `json:"attributed"`
	// LogicalBytes and AllocatedBytes are subtree totals at the asset path.
	LogicalBytes   int64 `json:"logicalBytes"`
	AllocatedBytes int64 `json:"allocatedBytes"`
	// ExclusiveAllocatedBytes subtracts nested storage-owning assets so a
	// project total is not counted again for its own node_modules.
	ExclusiveAllocatedBytes int64 `json:"exclusiveAllocatedBytes"`
	// Shared marks stores and caches that serve many projects; their bytes must
	// never be attributed to a single project.
	Shared bool `json:"shared,omitempty"`
	// Uncertain marks totals affected by hard links or clones.
	Uncertain bool `json:"uncertain,omitempty"`
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
	// Size and LastActivityAt are filled by size attribution after detection.
	Size Size `json:"size"`
	// LastActivityAt is the most recent modification under the asset, ignoring
	// generated subtrees and `.git`, so it approximates source activity.
	LastActivityAt time.Time `json:"lastActivityAt,omitzero"`
}

// OwnsStorage reports whether an asset kind owns the bytes under its path.
// Several assets may legitimately share one path — a repository, a worktree, and
// a project can all be rooted at the same directory — and each then reports the
// same attributed bytes. Attribution counts a path once, so this does not double
// count. Logical tools do not own storage: a package manager's path is the
// project directory it configures, whose bytes the project already accounts for.
func (k Kind) OwnsStorage() bool {
	switch k {
	case KindProject, KindProjectLocalInstall, KindBuildOutput, KindDependencyStore,
		KindDownloadCache, KindAIAsset, KindVersionManager, KindRuntime, KindContainerResource,
		KindGitRepository, KindGitWorktree:
		return true
	default:
		return false
	}
}

// IsShared reports whether a kind serves multiple projects, meaning its bytes
// must not be charged to any single project.
func (k Kind) IsShared() bool {
	switch k {
	case KindDependencyStore, KindDownloadCache, KindVersionManager, KindRuntime:
		return true
	default:
		return false
	}
}

// IsGenerated reports whether a kind holds regenerable material. Activity
// attribution skips these subtrees so an install does not look like source work.
func (k Kind) IsGenerated() bool {
	switch k {
	case KindProjectLocalInstall, KindBuildOutput, KindDependencyStore, KindDownloadCache:
		return true
	default:
		return false
	}
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

// PortfolioSummary aggregates installed tools, usage, and storage by class for
// an ecosystem. Byte totals require size attribution to have run.
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
	// Storage by class, kept separate so shared-store bytes are never presented
	// as project-local duplication.
	ProjectLocalInstallBytes int64 `json:"projectLocalInstallBytes,omitempty"`
	SharedStoreBytes         int64 `json:"sharedStoreBytes,omitempty"`
	DownloadCacheBytes       int64 `json:"downloadCacheBytes,omitempty"`
	BuildOutputBytes         int64 `json:"buildOutputBytes,omitempty"`
	// SizesUncertain marks totals affected by hard links or clones.
	SizesUncertain bool `json:"sizesUncertain,omitempty"`
}
