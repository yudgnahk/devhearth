// Package advisor composes the read-only analysis stages that follow detection:
// size and activity attribution, portfolio fit scoring, and deterministic
// recommendation rules. It exists so the protocol server and the CLI share one
// analysis path rather than assembling the stages themselves.
package advisor

import (
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/attribute"
	"github.com/yudgnahk/devhearth/internal/portfolio"
	"github.com/yudgnahk/devhearth/internal/recommend"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// Result is the analysed view of one scan. Graph is the attributed graph and
// replaces the detector output for persistence and queries.
type Result struct {
	Graph       assets.Graph
	Assessments []portfolio.Assessment

	// All is the complete, ordered rule output before any policy filtering. It
	// is retained so a policy change can re-sort advice into visible and hidden
	// without re-running the rules: hiding is a display decision, and re-running
	// analysis to apply one would risk producing different advice.
	All []recommend.Recommendation
	// Recommendations is what the inbox shows.
	Recommendations []recommend.Recommendation
	// Suppressed holds advice the user has already dismissed through policy.
	// It is kept rather than dropped so the UI can say "3 hidden by your policy"
	// instead of quietly showing less than the rules produced.
	Suppressed []recommend.Recommendation
	// HiddenByRisk holds advice above the policy's risk display threshold.
	HiddenByRisk []recommend.Recommendation
}

// Refilter re-partitions an existing result under a different policy. The rules
// are not re-run and no field of any recommendation changes; only which list
// each one lands in.
func (r Result) Refilter(filter Filter) Result {
	next := r
	next.Recommendations, next.Suppressed, next.HiddenByRisk = partition(r.All, filter)
	return next
}

// Filter decides which recommendations reach the inbox. The policy package
// implements it; advisor states only what it needs so analysis does not depend
// on the policy document format.
//
// A filter may hide advice. It may never change a savings figure, a confidence,
// or a risk class: those come from the rules and stay as the rules computed them.
type Filter interface {
	// Suppresses reports advice the user has already decided about.
	Suppresses(recommendationID, family, ecosystem string) bool
	// AllowsRisk reports whether a risk class is at or below the display threshold.
	AllowsRisk(risk string) bool
}

// Options tunes analysis. The zero value is valid and uses current time with
// the documented default windows and balanced fit weighting.
type Options struct {
	// Now anchors every activity and dormancy calculation so one scan cannot
	// disagree with itself. Zero uses the current UTC time.
	Now time.Time
	// ActiveWithin and InactiveAfter override the fit and dormancy windows.
	ActiveWithin  time.Duration
	InactiveAfter time.Duration
	// Preferred maps ecosystem to a user-preferred tool (policy input).
	Preferred map[string]string
	// FitMode names the weighting tradeoff for fit scoring (policy input).
	FitMode string
	// WeightOverrides adjusts individual fit factor weights by name.
	WeightOverrides map[string]float64
	// Filter hides advice the policy has already ruled out. Nil shows everything.
	Filter Filter
	// Registry overrides the built-in rule set, which tests use to isolate rules.
	Registry *recommend.Registry
}

// Analyze attributes sizes onto the graph, scores portfolio fit, runs the
// recommendation rules, and applies the policy filter. It never touches the
// filesystem and never mutates input.
func Analyze(graph assets.Graph, index map[string][]scan.DirectoryNode, options Options) Result {
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	registry := options.Registry
	if registry == nil {
		registry = recommend.NewRegistry()
	}

	attributed := attribute.Apply(graph, index)
	assessments := portfolio.Assess(attributed, portfolio.Input{
		Now:             now,
		ActiveWithin:    options.ActiveWithin,
		Preferred:       options.Preferred,
		FitMode:         options.FitMode,
		WeightOverrides: options.WeightOverrides,
	})
	recommendations := registry.Run(recommend.Input{
		Graph:         attributed,
		Assessments:   assessments,
		Now:           now,
		InactiveAfter: options.InactiveAfter,
	})

	result := Result{
		Graph:       attributed,
		Assessments: assessments,
		All:         recommendations,
	}
	result.Recommendations, result.Suppressed, result.HiddenByRisk = partition(recommendations, options.Filter)
	return result
}

// partition splits rule output into what the inbox shows and what the policy
// withheld. Suppression is checked first so an item the user explicitly
// dismissed is reported as dismissed rather than as risk-filtered.
func partition(recommendations []recommend.Recommendation, filter Filter) (visible, suppressed, hiddenByRisk []recommend.Recommendation) {
	if filter == nil {
		return recommendations, nil, nil
	}
	visible = make([]recommend.Recommendation, 0, len(recommendations))
	for _, recommendation := range recommendations {
		switch {
		case filter.Suppresses(recommendation.ID, string(recommendation.Family), recommendation.Ecosystem):
			suppressed = append(suppressed, recommendation)
		case !filter.AllowsRisk(string(recommendation.Risk)):
			hiddenByRisk = append(hiddenByRisk, recommendation)
		default:
			visible = append(visible, recommendation)
		}
	}
	return visible, suppressed, hiddenByRisk
}
