package portfolio

import (
	"maps"
	"sort"
)

// Weights assigns relative importance to each fit factor. A mode is a named
// tradeoff between disk savings and workflow stability, never a claim that one
// tool is best: every mode still ranks all options and still includes stay-put.
type Weights map[FactorKind]float64

// Named weighting modes. These strings are the policy document's vocabulary, so
// they are duplicated there as constants rather than imported: the policy
// package must not depend on scoring internals.
const (
	ModeBalanced          = "balanced"
	ModePreferDiskSavings = "prefer_disk_savings"
	ModePreferStability   = "prefer_workflow_stability"
)

// DefaultWeights is the balanced tradeoff. Disk savings deliberately never
// dominate on their own (SPECS §7.5: fit is multi-factor, not disk-first).
func DefaultWeights() Weights {
	return Weights{
		FactorInstalledPresence: weightInstalledPresence,
		FactorInUseShare:        weightInUseShare,
		FactorDuplicationCost:   weightDuplicationCost,
		FactorReproducibility:   weightReproducibility,
		FactorMigrationFriction: weightMigrationFriction,
		FactorProjectActivity:   weightProjectActivity,
		FactorPolicyPreference:  weightPolicyPreference,
	}
}

// modeWeights are the shipped presets. Each set sums to 1.0 so scores stay
// comparable between modes as well as between options.
var modeWeights = map[string]Weights{
	ModeBalanced: DefaultWeights(),
	// Duplication cost carries more of the score, but migration friction still
	// weighs 0.10: "prefer disk savings" is a preference, not permission to
	// recommend a painful migration for a small win.
	ModePreferDiskSavings: {
		FactorInstalledPresence: 0.10,
		FactorInUseShare:        0.25,
		FactorDuplicationCost:   0.35,
		FactorReproducibility:   0.10,
		FactorMigrationFriction: 0.10,
		FactorProjectActivity:   0.05,
		FactorPolicyPreference:  0.05,
	},
	// Friction and existing usage dominate, so this mode mostly ratifies what
	// the machine already does.
	ModePreferStability: {
		FactorInstalledPresence: 0.10,
		FactorInUseShare:        0.30,
		FactorDuplicationCost:   0.10,
		FactorReproducibility:   0.05,
		FactorMigrationFriction: 0.35,
		FactorProjectActivity:   0.05,
		FactorPolicyPreference:  0.05,
	},
}

// WeightsForMode returns the preset for a mode name. An unknown mode reports
// false and the caller keeps the balanced default rather than silently scoring
// with weights the user did not choose.
func WeightsForMode(mode string) (Weights, bool) {
	preset, known := modeWeights[mode]
	if !known {
		return DefaultWeights(), false
	}
	return maps.Clone(preset), true
}

// Modes lists the shipped modes in display order.
func Modes() []string { return []string{ModeBalanced, ModePreferDiskSavings, ModePreferStability} }

// WithOverrides returns a copy with per-factor overrides applied. Unknown factor
// names are ignored: a policy written against a future factor must not shift the
// weighting of the factors this build does understand.
func (w Weights) WithOverrides(overrides map[string]float64) Weights {
	if len(overrides) == 0 {
		return maps.Clone(w)
	}
	next := maps.Clone(w)
	for name, value := range overrides {
		kind := FactorKind(name)
		if _, known := next[kind]; !known {
			continue
		}
		if value < 0 {
			continue
		}
		next[kind] = value
	}
	return next
}

// normalized rescales weights to sum to 1.0 so a policy that supplies partial or
// unbalanced overrides still produces comparable scores. An all-zero set falls
// back to the default rather than making every option score zero.
func (w Weights) normalized() Weights {
	total := 0.0
	for _, value := range w {
		if value > 0 {
			total += value
		}
	}
	if total <= 0 {
		return DefaultWeights()
	}
	out := make(Weights, len(w))
	for kind, value := range w {
		if value <= 0 {
			out[kind] = 0
			continue
		}
		out[kind] = round4(value / total)
	}
	return out
}

// weightFor reads one factor, treating a missing entry as zero so a caller can
// build a partial set without every lookup needing a guard.
func (w Weights) weightFor(kind FactorKind) float64 {
	if w == nil {
		return DefaultWeights()[kind]
	}
	return w[kind]
}

// MinMigrationFrictionWeight is the floor migration friction may never fall
// below. A policy chooses a tradeoff; it may not delete the cost of a
// migration from the model. Without this floor, `fitWeights` could rank a
// disruptive migration as if it were free, which is exactly the "policy changed
// what the advice means" failure the safety model forbids.
const MinMigrationFrictionWeight = 0.05

// resolveWeights turns the fit input into the weight set scoring should use.
// Order is mode first, then explicit overrides, then the friction floor, then
// normalization.
func resolveWeights(input Input) Weights {
	base, _ := WeightsForMode(input.FitMode)
	weights := base.WithOverrides(input.WeightOverrides)
	if weights[FactorMigrationFriction] < MinMigrationFrictionWeight {
		weights[FactorMigrationFriction] = MinMigrationFrictionWeight
	}
	return weights.normalized()
}

// Ordered returns factors sorted by descending weight for display. Ties break on
// name so the UI never reorders between identical scans.
func (w Weights) Ordered() []FactorKind {
	kinds := make([]FactorKind, 0, len(w))
	for kind := range w {
		kinds = append(kinds, kind)
	}
	sort.Slice(kinds, func(i, j int) bool {
		if w[kinds[i]] != w[kinds[j]] {
			return w[kinds[i]] > w[kinds[j]]
		}
		return kinds[i] < kinds[j]
	})
	return kinds
}
