package recommend

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
)

// Registry holds the deterministic rule set. Rules are ordered by ID so a scan
// evaluates them in the same sequence every time.
type Registry struct {
	rules []Rule
}

// NewRegistry returns the built-in Phase 3 rules.
func NewRegistry() *Registry {
	return newRegistry(
		&runtimeConsolidationRule{},
		&sharedStoreRule{},
		&portfolioFitRule{},
		&hibernationRule{},
		&obsoleteWorktreeRule{},
		&duplicateModelRule{},
	)
}

func newRegistry(rules ...Rule) *Registry {
	ordered := make([]Rule, len(rules))
	copy(ordered, rules)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID() < ordered[j].ID() })
	return &Registry{rules: ordered}
}

// Rules returns the registered rules in evaluation order.
func (r *Registry) Rules() []Rule {
	out := make([]Rule, len(r.rules))
	copy(out, r.rules)
	return out
}

// Run evaluates every rule and returns recommendations ranked by value, risk,
// and confidence. It never mutates input.
func (r *Registry) Run(input Input) []Recommendation {
	if input.Now.IsZero() {
		input.Now = time.Now().UTC()
	}
	if input.InactiveAfter <= 0 {
		input.InactiveAfter = DefaultInactiveAfter
	}

	var out []Recommendation
	for _, rule := range r.rules {
		for _, recommendation := range rule.Evaluate(input) {
			recommendation.RuleID = rule.ID()
			recommendation.RuleVersion = rule.Version()
			if recommendation.Family == "" {
				recommendation.Family = rule.Family()
			}
			if recommendation.Risk == "" {
				recommendation.Risk = assets.RiskInformational
			}
			recommendation.Priority = priority(recommendation)
			out = append(out, recommendation)
		}
	}
	sortRecommendations(out)
	return out
}

// riskDamping keeps a large but risky saving from outranking a safe one. A
// deterministic rule owns the risk class; nothing downstream may raise these.
func riskDamping(risk assets.Risk) float64 {
	switch risk {
	case assets.RiskInformational, assets.RiskLow:
		return 1
	case assets.RiskMedium:
		return 0.8
	case assets.RiskHigh:
		return 0.4
	case assets.RiskProhibited:
		return 0
	default:
		return 0.5
	}
}

// priority is expected value: midpoint savings weighted by confidence and risk.
// Blocked recommendations are damped further so unresolved preconditions cannot
// float to the top of the inbox.
func priority(recommendation Recommendation) float64 {
	value := float64(recommendation.Savings.Midpoint())
	confidence := recommendation.Confidence
	if confidence <= 0 {
		confidence = 0.1
	}
	score := value * confidence * riskDamping(recommendation.Risk)
	if len(recommendation.Blockers) > 0 {
		score *= 0.5
	}
	return score
}

// sortRecommendations orders the inbox: highest expected value first, then by
// family and ID so equal-value advice never reshuffles between scans.
func sortRecommendations(items []Recommendation) {
	sort.Slice(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		if left.Confidence != right.Confidence {
			return left.Confidence > right.Confidence
		}
		if left.Family != right.Family {
			return left.Family < right.Family
		}
		return left.ID < right.ID
	})
}

// newID derives a stable, path-free identifier from a family and the scope keys
// that produced the recommendation. Scan-local asset UUIDs must never be used:
// the ID has to survive a rescan so Phase 4 can suppress advice by identity.
func newID(family Family, keys ...string) string {
	digest := sha256.New()
	digest.Write([]byte(family))
	for _, key := range keys {
		digest.Write([]byte{0})
		digest.Write([]byte(key))
	}
	return "rec_" + hex.EncodeToString(digest.Sum(nil))[:12]
}
