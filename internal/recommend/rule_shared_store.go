package recommend

import (
	"fmt"
	"sort"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/bytesize"
	"github.com/yudgnahk/devhearth/internal/portfolio"
)

// sharedStoreRule proposes moving duplicated project-local installs behind a
// content-addressed store that is already installed. It answers "reduce
// duplication with what this machine already has", which is a narrower question
// than the portfolio-fit ranking.
type sharedStoreRule struct{}

func (r *sharedStoreRule) ID() string     { return "rule.adopt_shared_store" }
func (r *sharedStoreRule) Version() int   { return 1 }
func (r *sharedStoreRule) Family() Family { return FamilyAdoptSharedStore }

func (r *sharedStoreRule) Evaluate(input Input) []Recommendation {
	installs := installsByEcosystem(input.Graph)

	var out []Recommendation
	for _, assessment := range input.Assessments {
		if assessment.Depth != portfolio.DepthDeep {
			continue
		}
		option, found := bestInstalledStoreOption(assessment)
		if !found {
			continue
		}
		affected := installs[assessment.Ecosystem]
		if len(affected) < 2 {
			continue
		}
		out = append(out, r.recommend(assessment, option, affected))
	}
	return out
}

func (r *sharedStoreRule) recommend(assessment portfolio.Assessment, option portfolio.Option, installs []assets.Asset) Recommendation {
	confidence := option.Confidence
	uncertain := false
	evidence := make([][]assets.Evidence, 0, len(installs))
	for _, install := range installs {
		evidence = append(evidence, install.Evidence)
		uncertain = uncertain || install.Size.Uncertain
	}
	if uncertain {
		// Hard links inside a measured install make the current footprint a lower
		// bound, so the modelled saving deserves less confidence. The penalty
		// applies once no matter how many installs are affected.
		confidence -= 0.05
	}

	return Recommendation{
		ID:        newID(FamilyAdoptSharedStore, assessment.Ecosystem, option.Tool),
		Family:    FamilyAdoptSharedStore,
		Ecosystem: assessment.Ecosystem,
		Title:     fmt.Sprintf("Share %s dependencies through the %s store", assessment.Ecosystem, option.Tool),
		Explanation: fmt.Sprintf(
			"%d project-local %s installs hold %s. %s is already present on this machine and keeps one content-addressed copy "+
				"per package, so projects reuse the same files instead of each keeping their own. "+
				"Overlap between these installs has not been measured, so the saving is a range of %s.",
			len(installs), assessment.Ecosystem, bytesize.Format(assessment.ProjectLocalInstallBytes), option.Tool,
			bytesize.FormatRange(option.ImmediateSavingsLowBytes, option.ImmediateSavingsHighBytes)),
		Risk:       assets.RiskMedium,
		Confidence: clampConfidence(confidence),
		Savings: Savings{
			LowBytes:                   option.ImmediateSavingsLowBytes,
			HighBytes:                  option.ImmediateSavingsHighBytes,
			FutureGrowthReductionBytes: option.FutureGrowthReductionBytes,
			Uncertain:                  true,
		},
		RestorationCost:     "each migrated project reinstalls from its lockfile: a download plus link step, typically under a minute per project on a warm store",
		CompatibilityImpact: option.WorkflowImpact,
		Preconditions: []string{
			"every affected project has a current lockfile",
			"native addons and postinstall scripts still resolve under the shared store layout",
			"CI uses the same package manager, or keeps working with the existing one",
		},
		ProposedActions: []string{
			fmt.Sprintf("adopt %s in one low-risk project first and confirm the build", option.Tool),
			"reinstall dependencies so the shared store is populated",
			"remove the superseded project-local installs only after each project builds",
		},
		Verification: []string{
			"each migrated project installs, builds, and tests from a clean checkout",
			"a follow-up scan shows project-local install bytes dropping and store bytes rising by less",
		},
		Rollback:         "reinstall with the previous package manager from the retained lockfile; project sources are untouched throughout",
		Blockers:         option.Blockers,
		AffectedAssetIDs: assetIDs(installs),
		Evidence:         mergeEvidence(evidence...),
		Alternatives:     alternativesFrom(assessment, option.Tool),
		DominantFactors:  factorNames(option.DominantFactors),
	}
}

// bestInstalledStoreOption picks the highest-saving option that is already
// installed. Proposing a tool the machine does not have belongs to portfolio
// fit, not to this rule.
func bestInstalledStoreOption(assessment portfolio.Assessment) (portfolio.Option, bool) {
	candidates := make([]portfolio.Option, 0, len(assessment.Options))
	for _, option := range assessment.Options {
		if option.Installed && option.ImmediateSavingsHighBytes > 0 {
			candidates = append(candidates, option)
		}
	}
	if len(candidates) == 0 {
		return portfolio.Option{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].ImmediateSavingsHighBytes == candidates[j].ImmediateSavingsHighBytes {
			return candidates[i].Tool < candidates[j].Tool
		}
		return candidates[i].ImmediateSavingsHighBytes > candidates[j].ImmediateSavingsHighBytes
	})
	return candidates[0], true
}

// installsByEcosystem groups project-local installs, ordered for stable output.
func installsByEcosystem(graph assets.Graph) map[string][]assets.Asset {
	out := map[string][]assets.Asset{}
	for _, install := range ofKind(graph, assets.KindProjectLocalInstall) {
		if install.Ecosystem == "" {
			continue
		}
		out[install.Ecosystem] = append(out[install.Ecosystem], install)
	}
	return out
}

// alternativesFrom lists every ranked option, including stay-put, so a user can
// see what was considered and why it lost.
func alternativesFrom(assessment portfolio.Assessment, chosen string) []Alternative {
	out := make([]Alternative, 0, len(assessment.Options))
	for _, option := range assessment.Options {
		if option.Tool == chosen {
			continue
		}
		out = append(out, Alternative{
			Label:            option.Tool,
			StayPut:          option.StayPut,
			Rank:             option.Rank,
			Score:            option.Score,
			SavingsLowBytes:  option.ImmediateSavingsLowBytes,
			SavingsHighBytes: option.ImmediateSavingsHighBytes,
			Blockers:         option.Blockers,
		})
	}
	return out
}

func factorNames(kinds []portfolio.FactorKind) []string {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]string, 0, len(kinds))
	for _, kind := range kinds {
		out = append(out, string(kind))
	}
	return out
}
