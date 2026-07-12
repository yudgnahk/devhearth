// Package rust detects Cargo projects and build output directories.
package rust

import (
	"context"
	"path/filepath"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.rust"
	detectorVersion = 1
	ecosystem       = "rust"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID:                   detectorID,
		Version:              detectorVersion,
		Triggers:             []string{"Cargo.toml", "Cargo.lock", "target"},
		Cost:                 detect.CostMetadata,
		RequiresContentReads: false,
		Produces: []string{
			string(assets.KindProject),
			string(assets.KindPackageManager),
			string(assets.KindBuildOutput),
		},
		RiskImplications: []string{"target/ is regenerable build output, distinct from Cargo registry"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch candidate.Name {
	case "Cargo.toml", "Cargo.lock":
		return true
	case "target":
		return candidate.IsDir
	default:
		return false
	}
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	if candidate.Name == "target" && candidate.IsDir {
		projectDir := candidate.Parent
		// Only treat as Cargo build output when a sibling Cargo.toml exists.
		// Without parent children in this candidate, require Cargo.toml via parent scan.
		projectKey := detect.ProjectKey(ecosystem, projectDir)
		build := detect.StampDetector(detect.Finding{
			Key:         detect.AssetKey(assets.KindBuildOutput, candidate.Path),
			Kind:        assets.KindBuildOutput,
			DisplayName: "target",
			Path:        candidate.Path,
			Risk:        assets.RiskLow,
			Ecosystem:   ecosystem,
			Class:       assets.ClassBuildOutput,
			Attributes:  map[string]string{"output_kind": "cargo_target"},
			Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.7)},
		}, detectorID, detectorVersion)
		project := detect.StampDetector(detect.Finding{
			Key:         projectKey,
			Kind:        assets.KindProject,
			DisplayName: detect.DisplayNameFromPath(projectDir),
			Path:        projectDir,
			Risk:        assets.RiskInformational,
			Ecosystem:   ecosystem,
			Class:       assets.ClassProject,
			Attributes:  map[string]string{"ecosystem": ecosystem},
			Evidence:    []assets.Evidence{detect.Evidence("build_output", candidate.Path, 0.6)},
		}, detectorID, detectorVersion)
		return detect.Result{
			Findings: []detect.Finding{project, build},
			Links: []detect.Link{{
				SourceKey: projectKey, TargetKey: build.Key,
				Kind: assets.RelProjectOwnsBuildOutput, Confidence: 0.7,
				Evidence: detect.Evidence("path_signature", candidate.Path, 0.7),
			}},
		}, nil
	}

	projectDir := candidate.Parent
	if projectDir == "" {
		projectDir = filepath.Dir(candidate.Path)
	}
	projectKey := detect.ProjectKey(ecosystem, projectDir)
	project := detect.StampDetector(detect.Finding{
		Key:         projectKey,
		Kind:        assets.KindProject,
		DisplayName: detect.DisplayNameFromPath(projectDir),
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassProject,
		Attributes:  map[string]string{"ecosystem": ecosystem},
		Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.95)},
	}, detectorID, detectorVersion)
	pm := detect.StampDetector(detect.Finding{
		Key:         detect.AssetKey(assets.KindPackageManager, projectDir+":cargo"),
		Kind:        assets.KindPackageManager,
		DisplayName: "cargo",
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassPackageManager,
		Attributes:  map[string]string{"tool": "cargo", "scope": "project"},
		Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Name, 0.95)},
	}, detectorID, detectorVersion)
	return detect.Result{
		Findings: []detect.Finding{project, pm},
		Links: []detect.Link{{
			SourceKey: projectKey, TargetKey: pm.Key,
			Kind: assets.RelProjectUsesPackageManager, Confidence: 0.95,
			Evidence: detect.Evidence("path_signature", candidate.Name, 0.95),
		}},
	}, nil
}
