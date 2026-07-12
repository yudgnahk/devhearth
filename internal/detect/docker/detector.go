// Package docker detects Dockerfiles and Compose projects.
package docker

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.docker"
	detectorVersion = 1
	ecosystem       = "container"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID: detectorID, Version: detectorVersion,
		Triggers: []string{
			"Dockerfile", "Dockerfile.dev", "docker-compose.yml", "docker-compose.yaml",
			"compose.yml", "compose.yaml", "Containerfile",
		},
		Cost: detect.CostMetadata, RequiresContentReads: false,
		Produces:         []string{string(assets.KindContainerResource), string(assets.KindProject)},
		RiskImplications: []string{"container volumes and images may hold persistent data"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	name := candidate.Name
	switch name {
	case "Dockerfile", "Dockerfile.dev", "Containerfile",
		"docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		return true
	default:
		return strings.HasPrefix(name, "Dockerfile.")
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
	kind := "dockerfile"
	risk := assets.RiskLow
	switch candidate.Name {
	case "docker-compose.yml", "docker-compose.yaml", "compose.yml", "compose.yaml":
		kind = "compose"
		risk = assets.RiskMedium
	}
	resource := detect.StampDetector(detect.Finding{
		Key: detect.AssetKey(assets.KindContainerResource, candidate.Path),
		Kind: assets.KindContainerResource, DisplayName: candidate.Name, Path: candidate.Path,
		Risk: risk, Ecosystem: ecosystem, Class: assets.ClassContainer,
		Attributes: map[string]string{"container_kind": kind},
		Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.9)},
	}, detectorID, detectorVersion)
	projectKey := detect.ProjectKey(ecosystem, projectDir)
	project := detect.StampDetector(detect.Finding{
		Key: projectKey, Kind: assets.KindProject,
		DisplayName: detect.DisplayNameFromPath(projectDir), Path: projectDir,
		Risk: assets.RiskInformational, Ecosystem: ecosystem, Class: assets.ClassProject,
		Attributes: map[string]string{"ecosystem": ecosystem},
		Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.75)},
	}, detectorID, detectorVersion)
	return detect.Result{
		Findings: []detect.Finding{project, resource},
		Links: []detect.Link{{
			SourceKey: resource.Key, TargetKey: projectKey,
			Kind: assets.RelComposeReferencesProject, Confidence: 0.75,
			Evidence: detect.Evidence("path_signature", candidate.Path, 0.75),
		}},
	}, nil
}
