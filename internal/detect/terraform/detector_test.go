package terraform_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/terraform"
)

func TestTerraformStateIsHighRisk(t *testing.T) {
	detector := terraform.New()
	projectDir := filepath.Join("/tmp", "infra")
	statePath := filepath.Join(projectDir, "terraform.tfstate")
	candidate := detect.Candidate{
		Path: statePath, Name: "terraform.tfstate", IsDir: false, Parent: projectDir,
	}
	if !detector.Match(candidate) {
		t.Fatal("expected match on terraform.tfstate")
	}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	var foundState bool
	for _, finding := range result.Findings {
		if finding.Path == statePath {
			foundState = true
			if finding.Risk != assets.RiskHigh {
				t.Fatalf("state risk = %q, want high", finding.Risk)
			}
			if finding.Attributes["regenerable"] != "false" {
				t.Fatalf("state must not be regenerable: %#v", finding.Attributes)
			}
		}
	}
	if !foundState {
		t.Fatalf("expected state finding, got %#v", result.Findings)
	}
}
