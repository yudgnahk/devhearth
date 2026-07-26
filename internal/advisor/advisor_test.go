package advisor

import (
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/builtin"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// analyzeFixture runs the real detector registry over the synthetic polyglot
// fixture and analyses the result, so the whole read-only pipeline is covered.
func analyzeFixture(t *testing.T, options Options) Result {
	t.Helper()
	inventory, err := scan.Inventory(t.Context(), []string{"../../testdata/filesystems/polyglot"}, scan.Options{})
	if err != nil {
		t.Fatalf("inventory fixture: %v", err)
	}
	registry, err := builtin.NewRegistry()
	if err != nil {
		t.Fatalf("build detector registry: %v", err)
	}
	graph, err := detect.Run(t.Context(), registry, inventory, detect.RunOptions{})
	if err != nil {
		t.Fatalf("run detectors: %v", err)
	}
	return Analyze(graph, scan.BuildDirectoryIndex(inventory), options)
}

func TestAnalyzeAttributesSizesOntoTheGraph(t *testing.T) {
	result := analyzeFixture(t, Options{Now: time.Now().UTC()})

	var attributed int
	for _, asset := range result.Graph.Assets {
		if asset.Size.Attributed {
			attributed++
		}
		if asset.Size.ExclusiveAllocatedBytes > asset.Size.AllocatedBytes {
			t.Fatalf("exclusive bytes exceed subtree total for %s: %#v", asset.DisplayName, asset.Size)
		}
	}
	if attributed == 0 {
		t.Fatal("no asset received attributed sizes from the fixture")
	}
}

func TestAnalyzeProducesAssessmentsForDetectedEcosystems(t *testing.T) {
	result := analyzeFixture(t, Options{Now: time.Now().UTC()})

	if len(result.Assessments) == 0 {
		t.Fatal("fixture should yield at least one ecosystem assessment")
	}
	for _, assessment := range result.Assessments {
		if assessment.Ecosystem == "" {
			t.Fatalf("assessment without an ecosystem: %#v", assessment)
		}
		if len(assessment.Notes) == 0 {
			t.Fatalf("%s assessment must state its scope limits", assessment.Ecosystem)
		}
		for _, option := range assessment.Options {
			if option.Confidence <= 0 || option.Confidence > 0.9 {
				t.Fatalf("%s/%s confidence out of range: %v", assessment.Ecosystem, option.Tool, option.Confidence)
			}
		}
	}
}

func TestAnalyzeIsDeterministicForTheSameScan(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

	first := analyzeFixture(t, Options{Now: now})
	second := analyzeFixture(t, Options{Now: now})

	if len(first.Recommendations) != len(second.Recommendations) {
		t.Fatalf("recommendation counts differ: %d vs %d", len(first.Recommendations), len(second.Recommendations))
	}
	for index := range first.Recommendations {
		if first.Recommendations[index].ID != second.Recommendations[index].ID {
			t.Fatalf("recommendation %d differs: %q vs %q",
				index, first.Recommendations[index].ID, second.Recommendations[index].ID)
		}
	}
}

func TestAnalyzeKeepsRecommendationsReadOnly(t *testing.T) {
	result := analyzeFixture(t, Options{Now: time.Now().UTC()})

	for _, recommendation := range result.Recommendations {
		if recommendation.Risk == assets.RiskProhibited {
			t.Fatalf("%s produced prohibited-risk advice", recommendation.Family)
		}
		if recommendation.Confidence <= 0 {
			t.Fatalf("%s has no confidence value", recommendation.Family)
		}
		if recommendation.Savings.HighBytes < recommendation.Savings.LowBytes {
			t.Fatalf("%s savings range is inverted: %#v", recommendation.Family, recommendation.Savings)
		}
	}
}

func TestAnalyzeHandlesAnEmptyGraph(t *testing.T) {
	result := Analyze(assets.Graph{}, map[string][]scan.DirectoryNode{}, Options{})

	if len(result.Graph.Assets) != 0 || len(result.Assessments) != 0 || len(result.Recommendations) != 0 {
		t.Fatalf("empty input produced output: %#v", result)
	}
}
