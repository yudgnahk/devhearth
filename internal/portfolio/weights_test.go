package portfolio

import (
	"math"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
)

// Scores are only comparable between options and between modes because every
// shipped weight set sums to 1.0.
func TestShippedModesSumToOne(t *testing.T) {
	for _, mode := range Modes() {
		t.Run(mode, func(t *testing.T) {
			weights, known := WeightsForMode(mode)
			if !known {
				t.Fatalf("%q is listed by Modes but has no preset", mode)
			}
			total := 0.0
			for _, value := range weights {
				total += value
			}
			if math.Abs(total-1.0) > 1e-9 {
				t.Fatalf("weights sum to %v, want 1.0", total)
			}
			for kind := range DefaultWeights() {
				if _, present := weights[kind]; !present {
					t.Fatalf("mode %q omits factor %q", mode, kind)
				}
			}
		})
	}
}

// Every mode keeps some migration friction: "prefer disk savings" is a
// preference, not permission to recommend a painful migration for a small win.
func TestNoModeZeroesMigrationFriction(t *testing.T) {
	for _, mode := range Modes() {
		weights, _ := WeightsForMode(mode)
		if weights[FactorMigrationFriction] <= 0 {
			t.Fatalf("mode %q dropped migration friction entirely", mode)
		}
	}
}

func TestWeightsForModeFallsBackWithoutClaimingSuccess(t *testing.T) {
	weights, known := WeightsForMode("prefer_whatever_is_newest")
	if known {
		t.Fatal("an unknown mode must not report success")
	}
	if weights[FactorInUseShare] != weightInUseShare {
		t.Fatalf("fallback weights = %v, want the balanced default", weights)
	}
}

// A preset must not be reachable for mutation through the value handed out.
func TestWeightsForModeReturnsACopy(t *testing.T) {
	first, _ := WeightsForMode(ModeBalanced)
	first[FactorInUseShare] = 0.99
	second, _ := WeightsForMode(ModeBalanced)
	if second[FactorInUseShare] != weightInUseShare {
		t.Fatal("WeightsForMode handed out the shared preset map")
	}
}

func TestWithOverrides(t *testing.T) {
	base := DefaultWeights()
	got := base.WithOverrides(map[string]float64{
		"in_use_share":       0.5,
		"not_a_real_factor":  0.9,
		"migration_friction": -1,
	})
	if got[FactorInUseShare] != 0.5 {
		t.Fatalf("known override not applied: %v", got)
	}
	if _, present := got["not_a_real_factor"]; present {
		t.Fatal("an unknown factor must not enter the weight set")
	}
	if got[FactorMigrationFriction] != weightMigrationFriction {
		t.Fatal("a negative override must be ignored, not applied")
	}
	if base[FactorInUseShare] != weightInUseShare {
		t.Fatal("WithOverrides mutated the receiver")
	}
}

func TestNormalizedRescalesPartialWeights(t *testing.T) {
	weights := Weights{
		FactorInUseShare:        1,
		FactorMigrationFriction: 1,
		FactorDuplicationCost:   2,
	}.normalized()
	total := 0.0
	for _, value := range weights {
		total += value
	}
	if math.Abs(total-1.0) > 1e-3 {
		t.Fatalf("normalized weights sum to %v, want 1.0", total)
	}
	if weights[FactorDuplicationCost] <= weights[FactorInUseShare] {
		t.Fatalf("normalization lost the relative ordering: %v", weights)
	}
}

func TestNormalizedFallsBackWhenEverythingIsZero(t *testing.T) {
	weights := Weights{FactorInUseShare: 0, FactorDuplicationCost: 0}.normalized()
	if weights[FactorInUseShare] != weightInUseShare {
		t.Fatalf("an all-zero set must fall back to the default, got %v", weights)
	}
}

// A policy picks a tradeoff; it may not delete the cost of a migration from the
// model. Without the floor, fitWeights could rank a disruptive migration as if
// it were free.
func TestPolicyCannotZeroOutMigrationFriction(t *testing.T) {
	weights := resolveWeights(Input{
		WeightOverrides: map[string]float64{"migration_friction": 0},
	})
	if weights[FactorMigrationFriction] <= 0 {
		t.Fatalf("migration friction was zeroed by policy: %v", weights)
	}

	// The floor survives normalization: it must stay a real share of the score,
	// not a rounding artefact.
	heavy := resolveWeights(Input{
		WeightOverrides: map[string]float64{
			"migration_friction": 0,
			"duplication_cost":   1,
			"in_use_share":       1,
		},
	})
	if heavy[FactorMigrationFriction] <= 0 {
		t.Fatalf("migration friction vanished under heavy overrides: %v", heavy)
	}
}

// The floor must not silently rewrite a weight a user legitimately set.
func TestFrictionFloorLeavesLegitimateWeightsAlone(t *testing.T) {
	weights := resolveWeights(Input{
		WeightOverrides: map[string]float64{"migration_friction": 0.4},
	})
	other := resolveWeights(Input{})
	if weights[FactorMigrationFriction] <= other[FactorMigrationFriction] {
		t.Fatalf("a raised friction weight was not honoured: %v vs %v", weights, other)
	}
}

func TestResolveWeightsAppliesModeThenOverrides(t *testing.T) {
	weights := resolveWeights(Input{
		FitMode:         ModePreferDiskSavings,
		WeightOverrides: map[string]float64{"policy_preference": 0.5},
	})
	if weights[FactorPolicyPreference] <= weightPolicyPreference {
		t.Fatalf("override did not raise the policy factor: %v", weights)
	}
	total := 0.0
	for _, value := range weights {
		total += value
	}
	if math.Abs(total-1.0) > 1e-3 {
		t.Fatalf("resolved weights sum to %v, want 1.0", total)
	}
}

// The point of a mode is that it changes the ranking. A portfolio with heavy
// duplication and real migration friction must rank differently under the two
// opposed modes, and stay-put must survive in both.
func TestFitModeChangesRankingButNeverRemovesStayPut(t *testing.T) {
	var graph assets.Graph
	for _, path := range []string{"/r/a", "/r/b", "/r/c", "/r/d"} {
		graph.Assets = append(graph.Assets, nodeProject(path, "npm", 3<<30, testNow)...)
	}
	graph.Assets = append(graph.Assets, assets.Asset{
		ID: "store:pnpm", Kind: assets.KindDependencyStore, DisplayName: "pnpm store",
		Path: "/home/.pnpm-store", Ecosystem: "node", Class: assets.ClassDependencyStore,
		Attributes: map[string]string{"tool": "pnpm", "store": "pnpm_store"},
		Size:       assets.Size{Attributed: true, AllocatedBytes: 1 << 28, Shared: true},
	})

	savings := findAssessment(t, Assess(graph, Input{Now: testNow, FitMode: ModePreferDiskSavings}), "node")
	stability := findAssessment(t, Assess(graph, Input{Now: testNow, FitMode: ModePreferStability}), "node")

	if savings.FitMode != ModePreferDiskSavings || stability.FitMode != ModePreferStability {
		t.Fatalf("assessments did not record their mode: %q / %q", savings.FitMode, stability.FitMode)
	}
	savingsPnpm := findOption(t, savings, "pnpm")
	stabilityPnpm := findOption(t, stability, "pnpm")
	if savingsPnpm.Score <= stabilityPnpm.Score {
		t.Fatalf("pnpm scored %v under prefer_disk_savings and %v under prefer_workflow_stability; the savings mode should rank a deduplicating store higher",
			savingsPnpm.Score, stabilityPnpm.Score)
	}
	if !stability.StayPutWins {
		t.Fatalf("prefer_workflow_stability recommended %q over the npm baseline", stability.RecommendedTool)
	}
	for _, assessment := range []Assessment{savings, stability} {
		stayPut := findOption(t, assessment, "npm")
		if !stayPut.StayPut {
			t.Fatalf("mode %q lost the stay-put baseline", assessment.FitMode)
		}
	}
}

// A weight override must move scores; if policy weights were quietly ignored the
// two rankings would be identical.
func TestWeightOverridesReachScoring(t *testing.T) {
	var graph assets.Graph
	graph.Assets = append(graph.Assets, nodeProject("/r/a", "npm", 1<<30, testNow)...)
	graph.Assets = append(graph.Assets, nodeProject("/r/b", "pnpm", 1<<30, testNow)...)

	neutral := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")
	weighted := findAssessment(t, Assess(graph, Input{
		Now:             testNow,
		Preferred:       map[string]string{"node": "pnpm"},
		WeightOverrides: map[string]float64{"policy_preference": 0.6},
	}), "node")

	if findOption(t, weighted, "pnpm").Score <= findOption(t, neutral, "pnpm").Score {
		t.Fatal("raising the policy-preference weight did not raise the preferred tool's score")
	}
	factor := findFactor(t, findOption(t, weighted, "pnpm"), FactorPolicyPreference)
	if factor.Weight <= weightPolicyPreference {
		t.Fatalf("factor weight = %v, want the policy override to be visible in the evidence", factor.Weight)
	}
}

func findFactor(t *testing.T, option Option, kind FactorKind) Factor {
	t.Helper()
	for _, factor := range option.Factors {
		if factor.Kind == kind {
			return factor
		}
	}
	t.Fatalf("no %q factor in %#v", kind, option.Factors)
	return Factor{}
}
