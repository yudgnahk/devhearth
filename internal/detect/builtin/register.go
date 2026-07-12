// Package builtin registers the Phase 2 detector suite.
package builtin

import (
	"fmt"

	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/ai"
	"github.com/yudgnahk/devhearth/internal/detect/docker"
	"github.com/yudgnahk/devhearth/internal/detect/git"
	"github.com/yudgnahk/devhearth/internal/detect/golangmod"
	"github.com/yudgnahk/devhearth/internal/detect/java"
	"github.com/yudgnahk/devhearth/internal/detect/node"
	"github.com/yudgnahk/devhearth/internal/detect/python"
	"github.com/yudgnahk/devhearth/internal/detect/runtime"
	"github.com/yudgnahk/devhearth/internal/detect/rust"
	"github.com/yudgnahk/devhearth/internal/detect/swiftdetect"
	"github.com/yudgnahk/devhearth/internal/detect/terraform"
)

// NewRegistry returns a registry with all built-in Phase 2 detectors.
func NewRegistry() (*detect.Registry, error) {
	registry := detect.NewRegistry()
	detectors := []detect.Detector{
		git.New(),
		node.New(),
		python.New(),
		golangmod.New(),
		rust.New(),
		swiftdetect.New(),
		java.New(),
		terraform.New(),
		docker.New(),
		runtime.New(),
		ai.New(),
	}
	for _, detector := range detectors {
		if err := registry.Register(detector); err != nil {
			return nil, fmt.Errorf("register %s: %w", detector.Descriptor().ID, err)
		}
	}
	return registry, nil
}
