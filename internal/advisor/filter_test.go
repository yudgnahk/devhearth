package advisor

import (
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/policy"
)

// policyFilter builds the real policy filter so the wiring, not a stub, is
// under test: a filter that silently failed to match would be invisible here
// otherwise.
func policyFilter(t *testing.T, mutate func(policy.Document) policy.Document) policy.Effective {
	t.Helper()
	document, err := policy.Validate(mutate(policy.DefaultDocument("test-policy")))
	if err != nil {
		t.Fatalf("build policy: %v", err)
	}
	return policy.Resolve(document, policy.Machine{})
}

func TestAnalyzeWithoutAFilterShowsEverything(t *testing.T) {
	result := analyzeFixture(t, Options{Now: time.Now().UTC()})
	if len(result.Suppressed) != 0 || len(result.HiddenByRisk) != 0 {
		t.Fatalf("no filter should withhold nothing, got %d suppressed and %d risk-hidden",
			len(result.Suppressed), len(result.HiddenByRisk))
	}
	if len(result.Recommendations) == 0 {
		t.Fatal("the fixture should produce recommendations to filter")
	}
}

func TestPolicySuppressionMovesAdviceOutOfTheInbox(t *testing.T) {
	unfiltered := analyzeFixture(t, Options{Now: time.Now().UTC()})
	if len(unfiltered.Recommendations) == 0 {
		t.Fatal("the fixture should produce recommendations")
	}
	target := unfiltered.Recommendations[0]

	filtered := analyzeFixture(t, Options{
		Now: time.Now().UTC(),
		Filter: policyFilter(t, func(document policy.Document) policy.Document {
			return document.WithSuppression(policy.Suppression{
				RecommendationID: target.ID,
				Reason:           "handled by hand",
			})
		}),
	})

	for _, recommendation := range filtered.Recommendations {
		if recommendation.ID == target.ID {
			t.Fatalf("suppressed recommendation %q still reached the inbox", target.ID)
		}
	}
	if len(filtered.Suppressed) != 1 || filtered.Suppressed[0].ID != target.ID {
		t.Fatalf("suppressed = %+v, want exactly the dismissed recommendation", filtered.Suppressed)
	}
	if len(filtered.Recommendations)+len(filtered.Suppressed) != len(unfiltered.Recommendations) {
		t.Fatalf("filtering lost or invented advice: %d visible + %d suppressed != %d produced",
			len(filtered.Recommendations), len(filtered.Suppressed), len(unfiltered.Recommendations))
	}
}

func TestFamilySuppressionHidesTheWholeFamily(t *testing.T) {
	unfiltered := analyzeFixture(t, Options{Now: time.Now().UTC()})
	family := string(unfiltered.Recommendations[0].Family)
	expected := 0
	for _, recommendation := range unfiltered.Recommendations {
		if string(recommendation.Family) == family {
			expected++
		}
	}

	filtered := analyzeFixture(t, Options{
		Now: time.Now().UTC(),
		Filter: policyFilter(t, func(document policy.Document) policy.Document {
			return document.WithSuppression(policy.Suppression{Family: family})
		}),
	})
	if len(filtered.Suppressed) != expected {
		t.Fatalf("suppressed %d of family %q, want %d", len(filtered.Suppressed), family, expected)
	}
	for _, recommendation := range filtered.Recommendations {
		if string(recommendation.Family) == family {
			t.Fatalf("family %q survived suppression", family)
		}
	}
}

// A risk threshold hides advice from the inbox. It must never rewrite the risk
// class of the advice it hides.
func TestRiskThresholdHidesWithoutRewritingRisk(t *testing.T) {
	unfiltered := analyzeFixture(t, Options{Now: time.Now().UTC()})
	filtered := analyzeFixture(t, Options{
		Now: time.Now().UTC(),
		Filter: policyFilter(t, func(document policy.Document) policy.Document {
			document.RiskThreshold = policy.RiskInformational
			return document
		}),
	})

	byID := make(map[string]string, len(unfiltered.Recommendations))
	for _, recommendation := range unfiltered.Recommendations {
		byID[recommendation.ID] = string(recommendation.Risk)
	}
	for _, recommendation := range filtered.Recommendations {
		if string(recommendation.Risk) != "informational" {
			t.Fatalf("recommendation %q with risk %q passed an informational threshold", recommendation.ID, recommendation.Risk)
		}
	}
	for _, recommendation := range filtered.HiddenByRisk {
		if was := byID[recommendation.ID]; was != string(recommendation.Risk) {
			t.Fatalf("risk for %q changed from %q to %q while being filtered", recommendation.ID, was, recommendation.Risk)
		}
	}
	if len(filtered.Recommendations)+len(filtered.HiddenByRisk)+len(filtered.Suppressed) != len(unfiltered.Recommendations) {
		t.Fatal("risk filtering lost or invented advice")
	}
}

// Savings and confidence come from the rules. A policy chooses what to show,
// never what a recommendation claims.
func TestFilteringDoesNotAlterSavingsOrConfidence(t *testing.T) {
	unfiltered := analyzeFixture(t, Options{Now: time.Now().UTC()})
	filtered := analyzeFixture(t, Options{
		Now: time.Now().UTC(),
		Filter: policyFilter(t, func(document policy.Document) policy.Document {
			return document.WithSuppression(policy.Suppression{Family: "unify_duplicate_ai_models"})
		}),
	})

	before := make(map[string]struct {
		low, high  int64
		confidence float64
	}, len(unfiltered.Recommendations))
	for _, recommendation := range unfiltered.Recommendations {
		before[recommendation.ID] = struct {
			low, high  int64
			confidence float64
		}{recommendation.Savings.LowBytes, recommendation.Savings.HighBytes, recommendation.Confidence}
	}
	for _, recommendation := range filtered.Recommendations {
		was, known := before[recommendation.ID]
		if !known {
			t.Fatalf("filtering invented recommendation %q", recommendation.ID)
		}
		if was.low != recommendation.Savings.LowBytes || was.high != recommendation.Savings.HighBytes {
			t.Fatalf("savings for %q changed under a policy filter", recommendation.ID)
		}
		if was.confidence != recommendation.Confidence {
			t.Fatalf("confidence for %q changed under a policy filter", recommendation.ID)
		}
	}
}

func TestPolicyFitModeReachesAssessments(t *testing.T) {
	result := analyzeFixture(t, Options{
		Now:     time.Now().UTC(),
		FitMode: policy.FitModePreferDiskSavings,
	})
	for _, assessment := range result.Assessments {
		if assessment.FitMode != policy.FitModePreferDiskSavings {
			t.Fatalf("%s was scored under %q, want the policy mode", assessment.Ecosystem, assessment.FitMode)
		}
	}
}
