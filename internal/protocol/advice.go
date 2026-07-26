package protocol

import (
	"sort"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/portfolio"
	"github.com/yudgnahk/devhearth/internal/recommend"
)

// Wire projections for Phase 3 advice. Everything that can carry a filesystem
// path is redacted against the selected roots before it leaves the engine.

func fitList(id string, active *activeScan) FitListResult {
	out := make([]FitAssessment, 0, len(active.advice.Assessments))
	for _, assessment := range active.advice.Assessments {
		out = append(out, toWireAssessment(assessment))
	}
	return FitListResult{ScanID: id, Fit: out}
}

func findAssessment(active *activeScan, ecosystem string) (FitAssessment, bool) {
	for _, assessment := range active.advice.Assessments {
		if assessment.Ecosystem == ecosystem {
			return toWireAssessment(assessment), true
		}
	}
	return FitAssessment{}, false
}

func toWireAssessment(assessment portfolio.Assessment) FitAssessment {
	options := make([]FitOption, 0, len(assessment.Options))
	for _, option := range assessment.Options {
		factors := make([]FitFactor, 0, len(option.Factors))
		for _, factor := range option.Factors {
			factors = append(factors, FitFactor{
				Kind: string(factor.Kind), Score: factor.Score,
				Weight: factor.Weight, Detail: factor.Detail,
			})
		}
		dominant := make([]string, 0, len(option.DominantFactors))
		for _, kind := range option.DominantFactors {
			dominant = append(dominant, string(kind))
		}
		options = append(options, FitOption{
			Tool: option.Tool, StayPut: option.StayPut, Installed: option.Installed,
			ProjectsUsing: option.ProjectsUsing, Rank: option.Rank, Score: option.Score,
			Confidence: option.Confidence, Factors: factors, DominantFactors: dominant,
			Blockers: option.Blockers, WorkflowImpact: option.WorkflowImpact,
			ImmediateSavingsLowBytes:   option.ImmediateSavingsLowBytes,
			ImmediateSavingsHighBytes:  option.ImmediateSavingsHighBytes,
			FutureGrowthReductionBytes: option.FutureGrowthReductionBytes,
			SavingsUncertain:           option.SavingsUncertain,
		})
	}
	return FitAssessment{
		Ecosystem: assessment.Ecosystem, Depth: string(assessment.Depth),
		ProjectCount: assessment.ProjectCount, Baseline: assessment.Baseline,
		RecommendedTool: assessment.RecommendedTool, StayPutWins: assessment.StayPutWins,
		Options: options, Notes: assessment.Notes,
		ProjectLocalInstallBytes: assessment.ProjectLocalInstallBytes,
		SharedStoreBytes:         assessment.SharedStoreBytes,
		VersionManagers:          assessment.VersionManagers,
	}
}

func recommendationsList(id string, active *activeScan, family string) RecommendationsListResult {
	byID := assetIndex(active)
	out := make([]RecommendationSummary, 0, len(active.advice.Recommendations))
	for _, recommendation := range active.advice.Recommendations {
		if family != "" && string(recommendation.Family) != family {
			continue
		}
		out = append(out, toWireRecommendation(recommendation, byID, active.result.Roots))
	}
	return RecommendationsListResult{ScanID: id, Recommendations: out}
}

func findRecommendation(active *activeScan, recommendationID string) (RecommendationSummary, bool) {
	for _, recommendation := range active.advice.Recommendations {
		if recommendation.ID == recommendationID {
			// Resolve only this recommendation's assets, and only once it matched:
			// an unknown id should not walk the graph at all.
			byID := assetSubset(active, recommendation.AffectedAssetIDs)
			return toWireRecommendation(recommendation, byID, active.result.Roots), true
		}
	}
	return RecommendationSummary{}, false
}

// assetIndex maps every asset by id, for callers projecting the whole inbox.
func assetIndex(active *activeScan) map[string]assets.Asset {
	byID := make(map[string]assets.Asset, len(active.advice.Graph.Assets))
	for _, asset := range active.advice.Graph.Assets {
		byID[asset.ID] = asset
	}
	return byID
}

// assetSubset maps only the requested ids, for a single-recommendation lookup.
func assetSubset(active *activeScan, ids []string) map[string]assets.Asset {
	if len(ids) == 0 {
		return nil
	}
	wanted := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	byID := make(map[string]assets.Asset, len(ids))
	for _, asset := range active.advice.Graph.Assets {
		if _, ok := wanted[asset.ID]; ok {
			byID[asset.ID] = asset
		}
	}
	return byID
}

func toWireRecommendation(
	recommendation recommend.Recommendation,
	byID map[string]assets.Asset,
	roots []string,
) RecommendationSummary {
	evidence := make([]EvidenceSummary, 0, len(recommendation.Evidence))
	for _, item := range recommendation.Evidence {
		evidence = append(evidence, EvidenceSummary{
			Kind: item.Kind, Value: redactEvidenceValue(item.Value, roots), Confidence: item.Confidence,
		})
	}
	affected := make([]AffectedAsset, 0, len(recommendation.AffectedAssetIDs))
	for _, assetID := range recommendation.AffectedAssetIDs {
		asset, found := byID[assetID]
		if !found {
			continue
		}
		affected = append(affected, AffectedAsset{
			ID: asset.ID, Kind: string(asset.Kind), DisplayName: asset.DisplayName,
			Path: redactPath(asset.Path, roots),
		})
	}
	alternatives := make([]RecommendationAlternative, 0, len(recommendation.Alternatives))
	for _, alternative := range recommendation.Alternatives {
		alternatives = append(alternatives, RecommendationAlternative{
			Label: alternative.Label, StayPut: alternative.StayPut, Rank: alternative.Rank,
			Score: alternative.Score, SavingsLowBytes: alternative.SavingsLowBytes,
			SavingsHighBytes: alternative.SavingsHighBytes, Blockers: alternative.Blockers,
		})
	}

	return RecommendationSummary{
		ID: recommendation.ID, Family: string(recommendation.Family), Title: recommendation.Title,
		Ecosystem: recommendation.Ecosystem, Explanation: recommendation.Explanation,
		Risk: string(recommendation.Risk), Confidence: recommendation.Confidence,
		Priority: recommendation.Priority,
		Savings: RecommendationSavings{
			LowBytes:                   recommendation.Savings.LowBytes,
			HighBytes:                  recommendation.Savings.HighBytes,
			FutureGrowthReductionBytes: recommendation.Savings.FutureGrowthReductionBytes,
			Uncertain:                  recommendation.Savings.Uncertain,
		},
		RestorationCost: recommendation.RestorationCost, CompatibilityImpact: recommendation.CompatibilityImpact,
		Preconditions: recommendation.Preconditions, ProposedActions: recommendation.ProposedActions,
		Verification: recommendation.Verification, Rollback: recommendation.Rollback,
		Blockers: recommendation.Blockers, AffectedAssets: affected, Evidence: evidence,
		Alternatives: alternatives, DominantFactors: recommendation.DominantFactors,
		RuleID: recommendation.RuleID, RuleVersion: recommendation.RuleVersion,
		AdviceOnly: true,
	}
}

// adviceSummary rolls recommendations up for report.export. Savings bounds are
// summed separately so the report never collapses a range into one number.
func adviceSummary(active *activeScan) *AdviceSummary {
	if len(active.advice.Recommendations) == 0 && len(active.advice.Assessments) == 0 {
		return nil
	}
	summary := &AdviceSummary{
		RecommendationCount: len(active.advice.Recommendations),
		ByFamily:            map[string]int{},
		ByRisk:              map[string]int{},
	}
	for _, recommendation := range active.advice.Recommendations {
		summary.ByFamily[string(recommendation.Family)]++
		summary.ByRisk[string(recommendation.Risk)]++
		summary.SavingsLowBytes += recommendation.Savings.LowBytes
		summary.SavingsHighBytes += recommendation.Savings.HighBytes
		if recommendation.Savings.Uncertain {
			summary.SavingsUncertain = true
		}
		if len(recommendation.Blockers) > 0 {
			summary.BlockedCount++
		}
	}
	if len(active.advice.Recommendations) > 0 {
		summary.TopRecommendationTitle = active.advice.Recommendations[0].Title
	}
	for _, assessment := range active.advice.Assessments {
		headline := FitHeadline{
			Ecosystem: assessment.Ecosystem, Depth: string(assessment.Depth),
			Baseline: assessment.Baseline, RecommendedTool: assessment.RecommendedTool,
			StayPutWins: assessment.StayPutWins,
		}
		if len(assessment.Options) > 0 {
			headline.Confidence = assessment.Options[0].Confidence
		}
		summary.Fit = append(summary.Fit, headline)
	}
	sort.Slice(summary.Fit, func(i, j int) bool { return summary.Fit[i].Ecosystem < summary.Fit[j].Ecosystem })
	return summary
}
