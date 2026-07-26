package recommend

import (
	"fmt"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/bytesize"
	"github.com/yudgnahk/devhearth/internal/portfolio"
)

// portfolioFitRule reports the ranked fit assessment for each deeply analysed
// ecosystem. It reports the stay-put outcome as well as a proposed change: "the
// tool you have still fits" is a useful answer, and the product must never claim
// a single absolute best tool.
type portfolioFitRule struct{}

func (r *portfolioFitRule) ID() string     { return "rule.standardize_portfolio_fit" }
func (r *portfolioFitRule) Version() int   { return 1 }
func (r *portfolioFitRule) Family() Family { return FamilyPortfolioFit }

func (r *portfolioFitRule) Evaluate(input Input) []Recommendation {
	var out []Recommendation
	for _, assessment := range input.Assessments {
		if assessment.Depth != portfolio.DepthDeep || len(assessment.Options) == 0 {
			continue
		}
		top := assessment.Options[0]
		if top.StayPut {
			out = append(out, r.stayPut(assessment, top))
			continue
		}
		out = append(out, r.standardize(assessment, top))
	}
	return out
}

func (r *portfolioFitRule) standardize(assessment portfolio.Assessment, top portfolio.Option) Recommendation {
	baseline := assessment.Baseline
	if baseline == "" {
		baseline = "no dominant tool"
	}
	return Recommendation{
		ID:        newID(FamilyPortfolioFit, assessment.Ecosystem, top.Tool),
		Family:    FamilyPortfolioFit,
		Ecosystem: assessment.Ecosystem,
		Title:     fmt.Sprintf("Standardize %s on %s", assessment.Ecosystem, top.Tool),
		Explanation: fmt.Sprintf(
			"Across %d detected %s projects, %s ranks first on installed presence, current usage, duplication cost, "+
				"reproducibility, migration friction, and project activity (score %.2f against a %s baseline). "+
				"This is a fit ranking for this machine, not a claim that %s is the best tool in general.",
			assessment.ProjectCount, assessment.Ecosystem, top.Tool, top.Score, baseline, top.Tool),
		Risk:       assets.RiskMedium,
		Confidence: clampConfidence(top.Confidence),
		Savings: Savings{
			LowBytes:                   top.ImmediateSavingsLowBytes,
			HighBytes:                  top.ImmediateSavingsHighBytes,
			FutureGrowthReductionBytes: top.FutureGrowthReductionBytes,
			Uncertain:                  top.SavingsUncertain,
		},
		RestorationCost:     "reverting means reinstalling from the retained previous lockfile in each migrated project",
		CompatibilityImpact: top.WorkflowImpact,
		Preconditions: []string{
			"the ranked blockers are resolved or accepted",
			"one project is migrated and verified before the rest follow",
			"CI pipelines and any private registry configuration are updated alongside each migrated project",
		},
		ProposedActions: []string{
			fmt.Sprintf("migrate one representative project to %s and keep the previous lockfile until it is verified", top.Tool),
			"record the outcome, then stage the remaining projects",
		},
		Verification: []string{
			"each migrated project installs, builds, and tests from a clean checkout",
			"a follow-up scan shows the expected package-manager distribution",
		},
		Rollback:         "restore the previous lockfile and reinstall with the previous tool",
		Blockers:         top.Blockers,
		Evidence:         fitEvidence(assessment, top),
		Alternatives:     alternativesFrom(assessment, top.Tool),
		DominantFactors:  factorNames(top.DominantFactors),
		AffectedAssetIDs: nil,
	}
}

func (r *portfolioFitRule) stayPut(assessment portfolio.Assessment, top portfolio.Option) Recommendation {
	return Recommendation{
		ID:        newID(FamilyPortfolioFit, assessment.Ecosystem, "stay_put", top.Tool),
		Family:    FamilyPortfolioFit,
		Ecosystem: assessment.Ecosystem,
		Title:     fmt.Sprintf("Keep %s for %s", top.Tool, assessment.Ecosystem),
		Explanation: fmt.Sprintf(
			"%s already carries %d of %d detected %s projects, and every alternative scored lower once migration friction "+
				"and workflow impact were weighed. Staying put is the recommended option; the ranked alternatives and the "+
				"%s of project-local installs are listed so the tradeoff stays visible.",
			top.Tool, top.ProjectsUsing, assessment.ProjectCount, assessment.Ecosystem,
			bytesize.Format(assessment.ProjectLocalInstallBytes)),
		Risk:       assets.RiskInformational,
		Confidence: clampConfidence(top.Confidence),
		// A stay-put outcome saves nothing today; that is the point of ranking it.
		Savings:             Savings{},
		CompatibilityImpact: top.WorkflowImpact,
		ProposedActions:     []string{"no change proposed; revisit if project counts or duplication grow"},
		Evidence:            fitEvidence(assessment, top),
		Alternatives:        alternativesFrom(assessment, top.Tool),
		DominantFactors:     factorNames(top.DominantFactors),
	}
}

// fitEvidence turns the winning option's factors and the assessment notes into
// evidence rows, so the inspector shows what produced the ranking.
func fitEvidence(assessment portfolio.Assessment, top portfolio.Option) []assets.Evidence {
	out := make([]assets.Evidence, 0, len(top.Factors)+len(assessment.Notes))
	for _, factor := range top.Factors {
		out = append(out, assets.Evidence{
			Kind:       "fit_factor:" + string(factor.Kind),
			Value:      factor.Detail,
			Confidence: factor.Score,
		})
	}
	for _, note := range assessment.Notes {
		out = append(out, assets.Evidence{Kind: "scope_limit", Value: note, Confidence: 1})
	}
	return mergeEvidence(out)
}
