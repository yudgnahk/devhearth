// Package java detects Maven and Gradle projects.
package java

import (
	"context"
	"path/filepath"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.java"
	detectorVersion = 1
	ecosystem       = "java"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID: detectorID, Version: detectorVersion,
		Triggers: []string{
			"pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
			"settings.gradle.kts", "gradlew", "gradlew.bat", "mvnw",
		},
		Cost: detect.CostMetadata, RequiresContentReads: false,
		Produces:         []string{string(assets.KindProject), string(assets.KindPackageManager)},
		RiskImplications: []string{"build/ and target/ outputs are distinct from Maven/Gradle caches"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch candidate.Name {
	case "pom.xml", "build.gradle", "build.gradle.kts", "settings.gradle",
		"settings.gradle.kts", "gradlew", "gradlew.bat", "mvnw":
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
	tool := "gradle"
	if candidate.Name == "pom.xml" || candidate.Name == "mvnw" {
		tool = "maven"
	}
	projectKey := detect.ProjectKey(ecosystem, projectDir)
	project := detect.StampDetector(detect.Finding{
		Key: projectKey, Kind: assets.KindProject,
		DisplayName: detect.DisplayNameFromPath(projectDir), Path: projectDir,
		Risk: assets.RiskInformational, Ecosystem: ecosystem, Class: assets.ClassProject,
		Attributes: map[string]string{"ecosystem": ecosystem},
		Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.9)},
	}, detectorID, detectorVersion)
	pm := detect.StampDetector(detect.Finding{
		Key: detect.AssetKey(assets.KindPackageManager, projectDir+":"+tool),
		Kind: assets.KindPackageManager, DisplayName: tool, Path: projectDir,
		Risk: assets.RiskInformational, Ecosystem: ecosystem, Class: assets.ClassPackageManager,
		Attributes: map[string]string{"tool": tool, "scope": "project"},
		Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Name, 0.9)},
	}, detectorID, detectorVersion)
	return detect.Result{
		Findings: []detect.Finding{project, pm},
		Links: []detect.Link{{
			SourceKey: projectKey, TargetKey: pm.Key,
			Kind: assets.RelProjectUsesPackageManager, Confidence: 0.9,
			Evidence: detect.Evidence("path_signature", candidate.Name, 0.9),
		}},
	}, nil
}
