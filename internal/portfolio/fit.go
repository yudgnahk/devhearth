package portfolio

import (
	"fmt"
	"sort"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
)

// deepCandidates are the ecosystems with enough fixture coverage and detector
// confidence for ranked fit analysis (SPECS §7.5 depth policy). Everything else
// gets a shallow inventory assessment that says so.
var deepCandidates = map[string][]string{
	"node":   {"bun", "npm", "pnpm", "yarn"},
	"python": {"conda", "pip", "pipenv", "poetry", "uv"},
}

// Assess ranks portfolio options per ecosystem from an attributed asset graph.
// It is deterministic: identical graphs produce identical rankings.
func Assess(graph assets.Graph, input Input) []Assessment {
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	activeWithin := input.ActiveWithin
	if activeWithin <= 0 {
		activeWithin = DefaultActiveWithin
	}

	// Weights are resolved once per scan: every ecosystem must be ranked under
	// the same tradeoff, or the assessments cannot be compared to each other.
	weights := resolveWeights(input)
	mode := input.FitMode
	if _, known := WeightsForMode(mode); !known {
		mode = ModeBalanced
	}

	bySignals := gather(graph, now, activeWithin)
	ecosystems := make([]string, 0, len(bySignals))
	for ecosystem := range bySignals {
		ecosystems = append(ecosystems, ecosystem)
	}
	sort.Strings(ecosystems)

	out := make([]Assessment, 0, len(ecosystems))
	for _, ecosystem := range ecosystems {
		out = append(out, assess(bySignals[ecosystem], input.Preferred[ecosystem], mode, weights))
	}
	return out
}

func assess(current *signals, preferred, mode string, weights Weights) Assessment {
	assessment := Assessment{
		Ecosystem:                current.ecosystem,
		ProjectCount:             current.projectCount(),
		Baseline:                 current.dominantTool(),
		ProjectLocalInstallBytes: current.localInstallBytes,
		SharedStoreBytes:         current.sharedStoreBytes,
		VersionManagers:          current.versionManagers,
		FitMode:                  mode,
	}

	candidates, deep := deepCandidates[current.ecosystem]
	if !deep {
		assessment.Depth = DepthShallow
		assessment.Notes = shallowNotes(current)
		return assessment
	}
	assessment.Depth = DepthDeep

	options := make([]Option, 0, len(candidates))
	for _, tool := range candidates {
		if !relevant(current, tool) {
			continue
		}
		options = append(options, scoreOption(current, tool, preferred, weights))
	}
	if len(options) == 0 {
		assessment.Depth = DepthShallow
		assessment.Notes = append(shallowNotes(current),
			"no package-manager evidence was found for this ecosystem under the scanned roots")
		return assessment
	}

	sortOptions(options)
	for index := range options {
		options[index].Rank = index + 1
	}
	assessment.Options = options
	assessment.RecommendedTool = options[0].Tool
	assessment.StayPutWins = options[0].StayPut
	assessment.Notes = deepNotes(current, options[0])
	return assessment
}

// relevant keeps the option list to tools with evidence: in use, installed, or
// able to reduce observed duplication. Tools with no local footing are omitted
// rather than ranked from assumptions.
func relevant(current *signals, tool string) bool {
	if current.inUse[tool] > 0 {
		return true
	}
	if _, installed := current.installed[tool]; installed {
		return true
	}
	return sharedStoreCapability[tool] > 0 && current.localInstallCount >= 2
}

// sortOptions ranks by score, then prefers the stay-put baseline on a tie
// (equal evidence should not push a user into a migration), then by name.
func sortOptions(options []Option) {
	sort.Slice(options, func(i, j int) bool {
		left, right := options[i], options[j]
		if left.Score != right.Score {
			return left.Score > right.Score
		}
		if left.StayPut != right.StayPut {
			return left.StayPut
		}
		if left.Confidence != right.Confidence {
			return left.Confidence > right.Confidence
		}
		return left.Tool < right.Tool
	})
}

func shallowNotes(current *signals) []string {
	notes := []string{
		"shallow inventory only: deep fit analysis for this ecosystem is deferred until fixture coverage and confidence thresholds are met",
	}
	if current.projectCount() == 0 {
		notes = append(notes, "no projects for this ecosystem were detected under the scanned roots")
	}
	return notes
}

func deepNotes(current *signals, top Option) []string {
	notes := []string{
		"absence of a tool under the scanned roots is not proof it is unused; scope is limited to the selected roots and well-known paths",
		"no content hashing has run, so overlap between project-local installs is unknown and savings are ranges",
	}
	if top.StayPut {
		notes = append(notes, fmt.Sprintf("staying with %s ranks highest: migration cost exceeds the modelled benefit", top.Tool))
	}
	if len(current.versionManagers) > 1 {
		notes = append(notes, fmt.Sprintf("%d version managers can manage this ecosystem (%v); consolidation is assessed separately",
			len(current.versionManagers), current.versionManagers))
	}
	if current.uncertainSizes {
		notes = append(notes, "hard links were seen inside measured trees, so allocated totals are lower bounds")
	}
	return notes
}
