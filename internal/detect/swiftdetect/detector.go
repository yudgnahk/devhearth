// Package swiftdetect detects SwiftPM and Xcode project layouts.
package swiftdetect

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.swift"
	detectorVersion = 1
	ecosystem       = "swift"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID:                   detectorID,
		Version:              detectorVersion,
		Triggers:             []string{"Package.swift", "Package.resolved", "Podfile", "Podfile.lock"},
		Cost:                 detect.CostMetadata,
		RequiresContentReads: false,
		Produces:             []string{string(assets.KindProject), string(assets.KindPackageManager)},
		RiskImplications:     []string{"DerivedData is separate from SwiftPM package caches"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch candidate.Name {
	case "Package.swift", "Package.resolved", "Podfile", "Podfile.lock":
		return true
	default:
		return strings.HasSuffix(candidate.Name, ".xcodeproj") ||
			strings.HasSuffix(candidate.Name, ".xcworkspace")
	}
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	projectDir := candidate.Path
	if !candidate.IsDir {
		projectDir = candidate.Parent
		if projectDir == "" {
			projectDir = filepath.Dir(candidate.Path)
		}
	} else if strings.HasSuffix(candidate.Name, ".xcodeproj") || strings.HasSuffix(candidate.Name, ".xcworkspace") {
		projectDir = candidate.Parent
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
		Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.9)},
	}, detectorID, detectorVersion)

	tool := "swiftpm"
	switch {
	case candidate.Name == "Podfile" || candidate.Name == "Podfile.lock":
		tool = "cocoapods"
	case strings.HasSuffix(candidate.Name, ".xcodeproj") || strings.HasSuffix(candidate.Name, ".xcworkspace"):
		tool = "xcode"
	}

	pm := detect.StampDetector(detect.Finding{
		Key:         detect.AssetKey(assets.KindPackageManager, projectDir+":"+tool),
		Kind:        assets.KindPackageManager,
		DisplayName: tool,
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassPackageManager,
		Attributes:  map[string]string{"tool": tool, "scope": "project"},
		Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Name, 0.9)},
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
