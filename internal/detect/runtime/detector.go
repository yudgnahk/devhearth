// Package runtime detects version managers, shared dependency stores, and
// download caches from well-known directory names under scan roots.
package runtime

import (
	"context"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.runtime"
	detectorVersion = 1
)

type signature struct {
	name       string
	kind       assets.Kind
	class      assets.Class
	ecosystem  string
	tool       string
	risk       assets.Risk
	confidence float64
}

var signatures = []signature{
	// Version managers
	{name: ".nvm", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "node", tool: "nvm", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".fnm", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "node", tool: "fnm", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".volta", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "node", tool: "volta", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".pyenv", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "python", tool: "pyenv", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".asdf", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "polyglot", tool: "asdf", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".mise", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "polyglot", tool: "mise", risk: assets.RiskMedium, confidence: 0.9},
	{name: "mise", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "polyglot", tool: "mise", risk: assets.RiskMedium, confidence: 0.7},
	{name: ".sdkman", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "java", tool: "sdkman", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".rustup", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "rust", tool: "rustup", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".local", kind: assets.KindUnknown, class: assets.ClassOther, ecosystem: "", tool: "", risk: assets.RiskInformational, confidence: 0}, // skipped via tool==""
	// Shared stores / caches under home-like trees
	{name: ".npm", kind: assets.KindDownloadCache, class: assets.ClassDownloadCache, ecosystem: "node", tool: "npm_cache", risk: assets.RiskLow, confidence: 0.8},
	{name: ".pnpm-store", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "node", tool: "pnpm_store", risk: assets.RiskMedium, confidence: 0.9},
	{name: "pnpm-store", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "node", tool: "pnpm_store", risk: assets.RiskMedium, confidence: 0.85},
	{name: ".yarn", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "node", tool: "yarn_cache", risk: assets.RiskMedium, confidence: 0.8},
	{name: ".bun", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "node", tool: "bun_install", risk: assets.RiskMedium, confidence: 0.8},
	{name: ".cargo", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "rust", tool: "cargo_home", risk: assets.RiskMedium, confidence: 0.85},
	{name: "go", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "go", tool: "gopath_or_modcache", risk: assets.RiskMedium, confidence: 0.5},
	{name: ".gradle", kind: assets.KindDownloadCache, class: assets.ClassDownloadCache, ecosystem: "java", tool: "gradle_cache", risk: assets.RiskLow, confidence: 0.85},
	{name: ".m2", kind: assets.KindDependencyStore, class: assets.ClassDependencyStore, ecosystem: "java", tool: "maven_local", risk: assets.RiskMedium, confidence: 0.9},
	{name: ".cache", kind: assets.KindDownloadCache, class: assets.ClassDownloadCache, ecosystem: "polyglot", tool: "user_cache", risk: assets.RiskLow, confidence: 0.5},
	{name: "Homebrew", kind: assets.KindVersionManager, class: assets.ClassVersionManager, ecosystem: "polyglot", tool: "homebrew", risk: assets.RiskMedium, confidence: 0.7},
	{name: "Cellar", kind: assets.KindRuntime, class: assets.ClassVersionManager, ecosystem: "polyglot", tool: "homebrew_cellar", risk: assets.RiskMedium, confidence: 0.8},
}

var byName map[string]signature

func init() {
	byName = make(map[string]signature, len(signatures))
	for _, sig := range signatures {
		if sig.tool == "" {
			continue
		}
		byName[sig.name] = sig
	}
}

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	triggers := make([]string, 0, len(byName))
	for name := range byName {
		triggers = append(triggers, name)
	}
	return detect.Descriptor{
		ID: detectorID, Version: detectorVersion, Triggers: triggers,
		Cost: detect.CostMetadata, RequiresContentReads: false,
		Produces: []string{
			string(assets.KindVersionManager), string(assets.KindRuntime),
			string(assets.KindDependencyStore), string(assets.KindDownloadCache),
		},
		RiskImplications: []string{"shared stores may serve many projects; do not double-count hard links"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	if !candidate.IsDir {
		return false
	}
	_, ok := byName[candidate.Name]
	// Avoid labeling every "go" directory: require parent suggests module cache/home.
	if candidate.Name == "go" {
		base := strings.ToLower(detect.DisplayNameFromPath(candidate.Parent))
		return base == "pkg" || strings.Contains(candidate.Path, "GOPATH") || strings.HasSuffix(candidate.Parent, "go")
	}
	if candidate.Name == "mise" {
		// Prefer ~/.local/share/mise style paths over random project folders named mise.
		return strings.Contains(candidate.Path, "share") || strings.Contains(candidate.Path, ".local")
	}
	return ok
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	sig, ok := byName[candidate.Name]
	if !ok {
		return detect.Result{}, nil
	}
	finding := detect.StampDetector(detect.Finding{
		Key:         detect.AssetKey(sig.kind, candidate.Path),
		Kind:        sig.kind,
		DisplayName: sig.tool,
		Path:        candidate.Path,
		Risk:        sig.risk,
		Ecosystem:   sig.ecosystem,
		Class:       sig.class,
		Attributes:  map[string]string{"tool": sig.tool, "scope": "machine"},
		Evidence:    []assets.Evidence{detect.Evidence("well_known_path", candidate.Path, sig.confidence)},
	}, detectorID, detectorVersion)
	return detect.Result{Findings: []detect.Finding{finding}}, nil
}
