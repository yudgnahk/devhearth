// Package terraform detects Terraform project layouts.
package terraform

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.terraform"
	detectorVersion = 1
	ecosystem       = "terraform"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID: detectorID, Version: detectorVersion,
		Triggers:             []string{"*.tf", "*.tf.json", ".terraform", ".terraform.lock.hcl"},
		Cost:                 detect.CostMetadata,
		RequiresContentReads: false,
		Produces:             []string{string(assets.KindProject), string(assets.KindProjectLocalInstall)},
		RiskImplications:     []string{".terraform providers are regenerable; state files are high risk"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch {
	case candidate.Name == ".terraform" && candidate.IsDir:
		return true
	case candidate.Name == ".terraform.lock.hcl":
		return true
	case strings.HasSuffix(candidate.Name, ".tf"):
		return true
	case strings.HasSuffix(candidate.Name, ".tf.json"):
		return true
	default:
		return false
	}
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	projectDir := candidate.Parent
	if projectDir == "" {
		projectDir = filepath.Dir(candidate.Path)
	}
	if candidate.Name == ".terraform" {
		projectDir = candidate.Parent
	}
	projectKey := detect.ProjectKey(ecosystem, projectDir)
	project := detect.StampDetector(detect.Finding{
		Key: projectKey, Kind: assets.KindProject,
		DisplayName: detect.DisplayNameFromPath(projectDir), Path: projectDir,
		Risk: assets.RiskInformational, Ecosystem: ecosystem, Class: assets.ClassProject,
		Attributes: map[string]string{"ecosystem": ecosystem},
		Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.9)},
	}, detectorID, detectorVersion)
	result := detect.Result{Findings: []detect.Finding{project}}
	if candidate.Name == ".terraform" && candidate.IsDir {
		install := detect.StampDetector(detect.Finding{
			Key: detect.AssetKey(assets.KindProjectLocalInstall, candidate.Path),
			Kind: assets.KindProjectLocalInstall, DisplayName: ".terraform", Path: candidate.Path,
			Risk: assets.RiskLow, Ecosystem: ecosystem, Class: assets.ClassProjectLocalInstall,
			Attributes: map[string]string{"install_kind": "terraform_providers"},
			Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.85)},
		}, detectorID, detectorVersion)
		result.Findings = append(result.Findings, install)
		result.Links = append(result.Links, detect.Link{
			SourceKey: projectKey, TargetKey: install.Key,
			Kind: assets.RelProjectOwnsEnvironment, Confidence: 0.85,
			Evidence: detect.Evidence("path_signature", candidate.Path, 0.85),
		})
	}
	return result, nil
}
