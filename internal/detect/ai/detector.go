// Package ai detects common local model and dataset store layouts.
package ai

import (
	"context"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.ai"
	detectorVersion = 1
	ecosystem       = "ai"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID: detectorID, Version: detectorVersion,
		Triggers: []string{
			".ollama", "ollama", "huggingface", "transformers",
			"models", "checkpoints", "lora", "gguf",
		},
		Cost: detect.CostMetadata, RequiresContentReads: false,
		Produces:         []string{string(assets.KindAIAsset)},
		RiskImplications: []string{"models may be downloadable but expensive; fine-tunes are high risk"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	name := strings.ToLower(candidate.Name)
	switch name {
	case ".ollama", "ollama", "huggingface", "transformers", "checkpoints":
		return candidate.IsDir
	case "models":
		// Prefer known parent markers to reduce false positives.
		parent := strings.ToLower(detect.DisplayNameFromPath(candidate.Parent))
		return candidate.IsDir && (parent == ".cache" || parent == "huggingface" || parent == "lmstudio" || parent == "ollama")
	default:
		return strings.HasSuffix(name, ".gguf") || strings.HasSuffix(name, ".safetensors")
	}
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	risk := assets.RiskMedium
	assetKind := "model_store"
	name := strings.ToLower(candidate.Name)
	if strings.HasSuffix(name, ".gguf") || strings.HasSuffix(name, ".safetensors") {
		assetKind = "model_file"
	}
	if name == "lora" || strings.Contains(candidate.Path, "lora") {
		risk = assets.RiskHigh
		assetKind = "fine_tune"
	}
	finding := detect.StampDetector(detect.Finding{
		Key: detect.AssetKey(assets.KindAIAsset, candidate.Path),
		Kind: assets.KindAIAsset, DisplayName: candidate.Name, Path: candidate.Path,
		Risk: risk, Ecosystem: ecosystem, Class: assets.ClassAI,
		Attributes: map[string]string{"ai_kind": assetKind},
		Evidence:   []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.75)},
	}, detectorID, detectorVersion)
	return detect.Result{Findings: []detect.Finding{finding}}, nil
}
