package detect

import (
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
)

func TestMergeFindingsEscalatesRiskAndDedupesEvidence(t *testing.T) {
	a := Finding{
		Key:  "k",
		Risk: assets.RiskLow,
		Evidence: []assets.Evidence{
			{Kind: "path_signature", Value: "/a", Confidence: 0.5},
		},
	}
	b := Finding{
		Key:  "k",
		Risk: assets.RiskHigh,
		Evidence: []assets.Evidence{
			{Kind: "path_signature", Value: "/a", Confidence: 0.9},
			{Kind: "manifest", Value: "Cargo.toml", Confidence: 0.9},
		},
	}
	merged := mergeFindings(a, b)
	if merged.Risk != assets.RiskHigh {
		t.Fatalf("risk = %q, want high", merged.Risk)
	}
	if len(merged.Evidence) != 2 {
		t.Fatalf("evidence = %#v, want 2 unique items", merged.Evidence)
	}
}
