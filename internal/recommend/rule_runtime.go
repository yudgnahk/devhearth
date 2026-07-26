package recommend

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/bytesize"
)

// runtimeConsolidationRule reports ecosystems managed by more than one version
// manager. It cannot yet compare installed versions against declared project
// requirements, so it proposes a plan and records that gap as a blocker instead
// of claiming the duplicates are interchangeable.
type runtimeConsolidationRule struct{}

func (r *runtimeConsolidationRule) ID() string     { return "rule.consolidate_runtime" }
func (r *runtimeConsolidationRule) Version() int   { return 1 }
func (r *runtimeConsolidationRule) Family() Family { return FamilyConsolidateRuntime }

func (r *runtimeConsolidationRule) Evaluate(input Input) []Recommendation {
	byEcosystem := versionManagersByEcosystem(input.Graph)
	ecosystems := make([]string, 0, len(byEcosystem))
	for ecosystem := range byEcosystem {
		ecosystems = append(ecosystems, ecosystem)
	}
	sort.Strings(ecosystems)

	var out []Recommendation
	for _, ecosystem := range ecosystems {
		managers := distinctByTool(byEcosystem[ecosystem])
		if len(managers) < 2 {
			continue
		}
		out = append(out, r.recommend(ecosystem, managers))
	}
	return out
}

func (r *runtimeConsolidationRule) recommend(ecosystem string, managers []assets.Asset) Recommendation {
	// Largest install is the assumed keeper: it most likely holds the versions
	// in use. The reclaimable bytes are the smaller installs.
	sorted := make([]assets.Asset, len(managers))
	copy(sorted, managers)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Size.ExclusiveAllocatedBytes == sorted[j].Size.ExclusiveAllocatedBytes {
			return toolOf(sorted[i]) < toolOf(sorted[j])
		}
		return sorted[i].Size.ExclusiveAllocatedBytes > sorted[j].Size.ExclusiveAllocatedBytes
	})

	var reclaimable int64
	uncertain := false
	tools := make([]string, 0, len(sorted))
	evidence := make([][]assets.Evidence, 0, len(sorted))
	for index, manager := range sorted {
		tools = append(tools, toolOf(manager))
		evidence = append(evidence, manager.Evidence)
		if index > 0 {
			reclaimable += manager.Size.ExclusiveAllocatedBytes
		}
		if manager.Size.Uncertain {
			uncertain = true
		}
	}
	sortedTools := slices.Clone(tools)
	sort.Strings(sortedTools)

	return Recommendation{
		ID:        newID(FamilyConsolidateRuntime, ecosystem, strings.Join(sortedTools, "|")),
		Family:    FamilyConsolidateRuntime,
		Ecosystem: ecosystem,
		Title:     fmt.Sprintf("Consolidate %d version managers for %s", len(sortedTools), ecosystem),
		Explanation: fmt.Sprintf(
			"%s can be managed by %v on this machine. Overlapping managers each keep their own runtime copies, "+
				"and shells that pick a different manager per session are a common source of version drift. "+
				"Keeping %s and retiring the others would release up to %s once the versions each manager provides are accounted for.",
			ecosystem, sortedTools, toolOf(sorted[0]), bytesize.Format(reclaimable)),
		Risk:       assets.RiskMedium,
		Confidence: clampConfidence(runtimeConfidence(sorted, uncertain)),
		Savings: Savings{
			// Low bound stays at zero: until installed versions are compared, a
			// duplicate manager may be the only source of a version in use.
			LowBytes:  0,
			HighBytes: reclaimable,
			Uncertain: true,
		},
		RestorationCost:     "reinstalling a runtime version through the surviving manager is a download plus build, typically minutes per version",
		CompatibilityImpact: "shell initialization, PATH order, and per-project version files must move to the surviving manager before the others are removed",
		Preconditions: []string{
			"every runtime version in use is available through the surviving manager",
			"project version files (for example .nvmrc, .python-version, .tool-versions) resolve under the surviving manager",
			"shell initialization no longer references the retired managers",
		},
		ProposedActions: []string{
			fmt.Sprintf("list the versions each manager currently provides for %s", ecosystem),
			fmt.Sprintf("install any missing versions through %s", toolOf(sorted[0])),
			"remove the retired managers' shell hooks, then their install directories",
		},
		Verification: []string{
			"a new shell resolves the expected runtime version in each active project",
			"the retired managers no longer appear on PATH",
		},
		Rollback:         "reinstall the retired manager and its versions from its published installer; no project data is involved",
		Blockers:         []string{"installed runtime versions have not been compared yet, so overlap between these managers is unverified"},
		AffectedAssetIDs: assetIDs(sorted),
		Evidence:         mergeEvidence(evidence...),
		DominantFactors:  []string{"runtime_sprawl", "duplication_cost"},
	}
}

// runtimeConfidence is highest when several ecosystem-specific managers overlap,
// since a polyglot manager alongside a specific one may be intentional.
func runtimeConfidence(managers []assets.Asset, uncertain bool) float64 {
	specific := 0
	for _, manager := range managers {
		if manager.Ecosystem != "" && manager.Ecosystem != "polyglot" {
			specific++
		}
	}
	value := 0.5
	if specific >= 2 {
		value = 0.65
	}
	if uncertain {
		value -= 0.05
	}
	return value
}

// distinctByTool keeps one asset per tool: two paths for the same manager are
// one manager, not duplicate capability.
func distinctByTool(managers []assets.Asset) []assets.Asset {
	seen := map[string]struct{}{}
	out := make([]assets.Asset, 0, len(managers))
	for _, manager := range managers {
		tool := toolOf(manager)
		if tool == "" {
			continue
		}
		if _, ok := seen[tool]; ok {
			continue
		}
		seen[tool] = struct{}{}
		out = append(out, manager)
	}
	return out
}
