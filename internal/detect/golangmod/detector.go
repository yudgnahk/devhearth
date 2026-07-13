// Package golangmod detects Go module and workspace projects.
package golangmod

import (
	"context"
	"path/filepath"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.go"
	detectorVersion = 1
	ecosystem       = "go"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID:                   detectorID,
		Version:              detectorVersion,
		Triggers:             []string{"go.mod", "go.work", "go.sum"},
		Cost:                 detect.CostMetadata,
		RequiresContentReads: false,
		Produces:             []string{string(assets.KindProject), string(assets.KindPackageManager)},
		RiskImplications:     []string{"module cache is shared and not project-local"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch candidate.Name {
	case "go.mod", "go.work", "go.sum":
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
		Key:         detect.AssetKey(assets.KindPackageManager, projectDir+":go_modules"),
		Kind:        assets.KindPackageManager,
		DisplayName: "go modules",
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassPackageManager,
		Attributes:  map[string]string{"tool": "go_modules", "scope": "project"},
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
