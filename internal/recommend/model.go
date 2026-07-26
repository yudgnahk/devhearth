// Package recommend turns an attributed asset graph and portfolio fit
// assessments into deterministic, evidence-backed optimization advice. Rules are
// read-only: they describe plans and non-executing commands, and nothing here
// may authorize or perform a mutation.
package recommend

import (
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/portfolio"
)

// Family groups recommendations by the optimization they propose
// (SPECS §7.8 initial recommendation families).
type Family string

const (
	FamilyConsolidateRuntime Family = "consolidate_runtime"
	FamilyAdoptSharedStore   Family = "adopt_shared_store"
	FamilyPortfolioFit       Family = "standardize_portfolio_fit"
	FamilyHibernateProject   Family = "hibernate_inactive_project"
	FamilyObsoleteWorktree   Family = "remove_obsolete_worktree"
	FamilyDuplicateAIModel   Family = "unify_duplicate_ai_models"
)

// Savings is an estimated storage outcome. Bounds are always a range unless the
// underlying bytes are directly measured, and Uncertain marks totals affected by
// hard links, clones, or missing content comparison.
type Savings struct {
	LowBytes                   int64 `json:"lowBytes"`
	HighBytes                  int64 `json:"highBytes"`
	FutureGrowthReductionBytes int64 `json:"futureGrowthReductionBytes,omitempty"`
	Uncertain                  bool  `json:"uncertain,omitempty"`
}

// Midpoint is the ranking value for a savings range.
func (s Savings) Midpoint() int64 { return (s.LowBytes + s.HighBytes) / 2 }

// Alternative is one option that was considered and not chosen, including the
// stay-put baseline. Portfolio-fit recommendations must always list these.
type Alternative struct {
	Label            string   `json:"label"`
	StayPut          bool     `json:"stayPut,omitempty"`
	Rank             int      `json:"rank"`
	Score            float64  `json:"score"`
	SavingsLowBytes  int64    `json:"savingsLowBytes,omitempty"`
	SavingsHighBytes int64    `json:"savingsHighBytes,omitempty"`
	Blockers         []string `json:"blockers,omitempty"`
}

// Recommendation is one evidence-backed proposal (SPECS §7.8). Every field that
// a user needs in order to accept or reject the advice is carried here; nothing
// is left to be inferred by the UI.
type Recommendation struct {
	ID        string `json:"id"`
	Family    Family `json:"family"`
	Title     string `json:"title"`
	Ecosystem string `json:"ecosystem,omitempty"`

	Explanation         string      `json:"explanation"`
	Risk                assets.Risk `json:"risk"`
	Confidence          float64     `json:"confidence"`
	Savings             Savings     `json:"savings"`
	RestorationCost     string      `json:"restorationCost,omitempty"`
	CompatibilityImpact string      `json:"compatibilityImpact,omitempty"`

	Preconditions   []string `json:"preconditions,omitempty"`
	ProposedActions []string `json:"proposedActions,omitempty"`
	Verification    []string `json:"verification,omitempty"`
	Rollback        string   `json:"rollback,omitempty"`
	Blockers        []string `json:"blockers,omitempty"`

	AffectedAssetIDs []string          `json:"affectedAssetIds,omitempty"`
	Evidence         []assets.Evidence `json:"evidence,omitempty"`
	Alternatives     []Alternative     `json:"alternatives,omitempty"`
	DominantFactors  []string          `json:"dominantFactors,omitempty"`

	RuleID      string `json:"ruleId"`
	RuleVersion int    `json:"ruleVersion"`
	// Priority ranks the inbox by value, risk, and confidence. It is derived,
	// never supplied by a rule.
	Priority float64 `json:"priority"`
}

// Input is the read-only evidence a rule may consult.
type Input struct {
	Graph       assets.Graph
	Assessments []portfolio.Assessment
	// Now anchors inactivity math; zero falls back to time.Now.
	Now time.Time
	// InactiveAfter is how long a project must be untouched before hibernation
	// or worktree removal is even considered. Zero selects DefaultInactiveAfter.
	InactiveAfter time.Duration
}

// DefaultInactiveAfter is deliberately long: proposing removal for a project
// somebody touched last month would be worse than proposing nothing.
const DefaultInactiveAfter = 180 * 24 * time.Hour

// Rule is a deterministic recommendation producer. Implementations must not
// touch the filesystem, must not mutate Input, and must return the same
// recommendations for the same input.
type Rule interface {
	ID() string
	Version() int
	Family() Family
	Evaluate(Input) []Recommendation
}
