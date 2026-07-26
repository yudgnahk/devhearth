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
	Graph           assets.Graph
	Assessments     []portfolio.Assessment
	Recommendations []recommend.Recommendation
}

// Options tunes analysis. The zero value is valid and uses current time with
// the documented default windows.
type Options struct {
	// Now anchors every activity and dormancy calculation so one scan cannot
	// disagree with itself. Zero uses the current UTC time.
	Now time.Time
	// ActiveWithin and InactiveAfter override the fit and dormancy windows.
	ActiveWithin  time.Duration
	InactiveAfter time.Duration
	// Preferred maps ecosystem to a user-preferred tool (future policy input).
	Preferred map[string]string
	// Registry overrides the built-in rule set, which tests use to isolate rules.
	Registry *recommend.Registry
}

// Analyze attributes sizes onto the graph, scores portfolio fit, and runs the
// recommendation rules. It never touches the filesystem and never mutates input.
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
		Now:          now,
		ActiveWithin: options.ActiveWithin,
		Preferred:    options.Preferred,
	})
	recommendations := registry.Run(recommend.Input{
		Graph:         attributed,
		Assessments:   assessments,
		Now:           now,
		InactiveAfter: options.InactiveAfter,
	})

	return Result{
		Graph:           attributed,
		Assessments:     assessments,
		Recommendations: recommendations,
	}
}
