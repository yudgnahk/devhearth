// Package portfolio scores tool-portfolio fit per ecosystem. It ranks options
// with multi-factor evidence and always includes a stay-put baseline; it never
// declares an absolute best tool and never authorizes a change.
package portfolio

import "time"

// FactorKind names a fit signal from SPECS §7.5.
type FactorKind string

const (
	FactorInstalledPresence FactorKind = "installed_presence"
	FactorInUseShare        FactorKind = "in_use_share"
	FactorDuplicationCost   FactorKind = "duplication_cost"
	FactorReproducibility   FactorKind = "reproducibility"
	FactorMigrationFriction FactorKind = "migration_friction"
	FactorProjectActivity   FactorKind = "project_activity"
	FactorPolicyPreference  FactorKind = "policy_preference"
)

// Depth records how much analysis an ecosystem received. Shallow assessments
// are inventory statements, not rankings (SPECS §7.5 depth policy).
type Depth string

const (
	DepthDeep    Depth = "deep"
	DepthShallow Depth = "shallow"
)

// Factor is one weighted signal behind an option's score. Score is normalized
// to 0..1 before weighting so the weights alone describe the tradeoff.
type Factor struct {
	Kind   FactorKind `json:"kind"`
	Score  float64    `json:"score"`
	Weight float64    `json:"weight"`
	Detail string     `json:"detail"`
}

// Option is one ranked portfolio choice for an ecosystem.
type Option struct {
	Tool          string `json:"tool"`
	StayPut       bool   `json:"stayPut"`
	Installed     bool   `json:"installed"`
	ProjectsUsing int    `json:"projectsUsing"`

	Rank       int     `json:"rank"`
	Score      float64 `json:"score"`
	Confidence float64 `json:"confidence"`

	Factors         []Factor     `json:"factors,omitempty"`
	DominantFactors []FactorKind `json:"dominantFactors,omitempty"`
	Blockers        []string     `json:"blockers,omitempty"`
	WorkflowImpact  string       `json:"workflowImpact,omitempty"`

	// Savings are ranges: no content hashing has run, so overlap between
	// project-local installs is unknown.
	ImmediateSavingsLowBytes   int64 `json:"immediateSavingsLowBytes"`
	ImmediateSavingsHighBytes  int64 `json:"immediateSavingsHighBytes"`
	FutureGrowthReductionBytes int64 `json:"futureGrowthReductionBytes"`
	SavingsUncertain           bool  `json:"savingsUncertain,omitempty"`
}

// Assessment is the ranked fit result for one ecosystem.
type Assessment struct {
	Ecosystem    string `json:"ecosystem"`
	Depth        Depth  `json:"depth"`
	ProjectCount int    `json:"projectCount"`

	// Baseline is the tool the machine already leans on (portfolio gravity).
	Baseline string `json:"baseline,omitempty"`
	// RecommendedTool is the top-ranked option, which may equal Baseline.
	RecommendedTool string `json:"recommendedTool,omitempty"`
	StayPutWins     bool   `json:"stayPutWins"`

	Options []Option `json:"options,omitempty"`
	Notes   []string `json:"notes,omitempty"`

	// FitMode names the weighting tradeoff that produced this ranking. It
	// travels with the assessment because the same portfolio ranks differently
	// under a different policy, and a user comparing two machines needs to see
	// which tradeoff each one used.
	FitMode string `json:"fitMode,omitempty"`

	ProjectLocalInstallBytes int64    `json:"projectLocalInstallBytes,omitempty"`
	SharedStoreBytes         int64    `json:"sharedStoreBytes,omitempty"`
	VersionManagers          []string `json:"versionManagers,omitempty"`
}

// Input is everything fit scoring may read. It is deliberately a value: scoring
// must stay deterministic and side-effect free.
type Input struct {
	// Now anchors project-activity math; zero falls back to time.Now.
	Now time.Time
	// ActiveWithin is how recently a project must have changed to count as
	// active. Zero selects DefaultActiveWithin.
	ActiveWithin time.Duration
	// Preferred maps ecosystem to a user-preferred tool. The Phase 4 policy
	// supplies this; an empty map leaves the policy factor neutral.
	Preferred map[string]string
	// FitMode selects a named weighting tradeoff. An empty or unknown mode uses
	// the balanced default.
	FitMode string
	// WeightOverrides adjusts individual factor weights by name. Unknown factor
	// names are ignored, and the resulting set is renormalized before use.
	WeightOverrides map[string]float64
}

// DefaultActiveWithin treats a quarter of source inactivity as dormant. It is a
// starting heuristic, not a product claim.
const DefaultActiveWithin = 90 * 24 * time.Hour
