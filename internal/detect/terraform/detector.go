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
		Triggers: []string{
			"*.tf", "*.tf.json", ".terraform", ".terraform.lock.hcl",
			"*.tfstate", "*.tfstate.backup",
		},
		Cost:                 detect.CostMetadata,
		RequiresContentReads: false,
		Produces: []string{
			string(assets.KindProject),
			string(assets.KindProjectLocalInstall),
			string(assets.KindUnknown),
		},
		RiskImplications: []string{".terraform providers are regenerable; state files are high risk"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch {
	case candidate.Name == ".terraform" && candidate.IsDir:
		return true
	case candidate.Name == ".terraform.lock.hcl":
		return true
	case isTerraformState(candidate.Name):
		return !candidate.IsDir
	case strings.HasSuffix(candidate.Name, ".tf"):
		return true
	case strings.HasSuffix(candidate.Name, ".tf.json"):
		return true
	default:
		return false
	}
}

func isTerraformState(name string) bool {
	return strings.HasSuffix(name, ".tfstate") || strings.HasSuffix(name, ".tfstate.backup")
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
			Key:  detect.AssetKey(assets.KindProjectLocalInstall, candidate.Path),
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
	if isTerraformState(candidate.Name) {
		// State is high-risk persistent data and is never treated as regenerable.
		state := detect.StampDetector(detect.Finding{
			Key:         detect.AssetKey(assets.KindUnknown, candidate.Path),
			Kind:        assets.KindUnknown,
			DisplayName: candidate.Name,
			Path:        candidate.Path,
			Risk:        assets.RiskHigh,
			Ecosystem:   ecosystem,
			Class:       assets.ClassOther,
			Attributes: map[string]string{
				"data_kind":   "terraform_state",
				"regenerable": "false",
			},
			Evidence: []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.95)},
		}, detectorID, detectorVersion)
		result.Findings = append(result.Findings, state)
		result.Links = append(result.Links, detect.Link{
			SourceKey: projectKey, TargetKey: state.Key,
			Kind: assets.RelProjectOwnsEnvironment, Confidence: 0.9,
			Evidence: detect.Evidence("path_signature", candidate.Path, 0.9),
		})
	}
	return result, nil
}
