package store

import (
	"context"
	"encoding/json"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/portfolio"
	"github.com/yudgnahk/devhearth/internal/recommend"
)

// ListFitAssessments returns persisted fit assessments for a scan, ordered by
// ecosystem, with their ranked options.
func (s *Store) ListFitAssessments(ctx context.Context, scanID string) ([]portfolio.Assessment, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, ecosystem, depth, project_count, baseline_tool, recommended_tool, stay_put_wins, project_local_install_bytes, shared_store_bytes, version_managers_json, notes_json FROM fit_assessments WHERE scan_id = ? ORDER BY ecosystem`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []string
	var out []portfolio.Assessment
	for rows.Next() {
		var assessment portfolio.Assessment
		var id, depth, versionManagers, notes string
		var stayPut int
		if err := rows.Scan(&id, &assessment.Ecosystem, &depth, &assessment.ProjectCount,
			&assessment.Baseline, &assessment.RecommendedTool, &stayPut,
			&assessment.ProjectLocalInstallBytes, &assessment.SharedStoreBytes,
			&versionManagers, &notes); err != nil {
			return nil, err
		}
		assessment.Depth = portfolio.Depth(depth)
		assessment.StayPutWins = stayPut == 1
		if err := json.Unmarshal([]byte(versionManagers), &assessment.VersionManagers); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(notes), &assessment.Notes); err != nil {
			return nil, err
		}
		ids = append(ids, id)
		out = append(out, assessment)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for index, id := range ids {
		options, err := s.listFitOptions(ctx, id)
		if err != nil {
			return nil, err
		}
		out[index].Options = options
	}
	return out, nil
}

func (s *Store) listFitOptions(ctx context.Context, assessmentID string) ([]portfolio.Option, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT tool, rank, score, confidence, stay_put, installed, projects_using, immediate_savings_low_bytes, immediate_savings_high_bytes, future_growth_reduction_bytes, savings_uncertain, workflow_impact, factors_json, dominant_factors_json, blockers_json FROM fit_options WHERE assessment_id = ? ORDER BY rank`, assessmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []portfolio.Option
	for rows.Next() {
		var option portfolio.Option
		var stayPut, installed, uncertain int
		var factors, dominant, blockers string
		if err := rows.Scan(&option.Tool, &option.Rank, &option.Score, &option.Confidence,
			&stayPut, &installed, &option.ProjectsUsing,
			&option.ImmediateSavingsLowBytes, &option.ImmediateSavingsHighBytes,
			&option.FutureGrowthReductionBytes, &uncertain, &option.WorkflowImpact,
			&factors, &dominant, &blockers); err != nil {
			return nil, err
		}
		option.StayPut = stayPut == 1
		option.Installed = installed == 1
		option.SavingsUncertain = uncertain == 1
		if err := json.Unmarshal([]byte(factors), &option.Factors); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(dominant), &option.DominantFactors); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(blockers), &option.Blockers); err != nil {
			return nil, err
		}
		out = append(out, option)
	}
	return out, rows.Err()
}

// ListRecommendations returns persisted recommendations for a scan, highest
// priority first, with their affected assets and evidence.
func (s *Store) ListRecommendations(ctx context.Context, scanID string) ([]recommend.Recommendation, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, family, title, ecosystem, explanation, risk, confidence, priority, savings_low_bytes, savings_high_bytes, future_growth_reduction_bytes, savings_uncertain, restoration_cost, compatibility_impact, preconditions_json, proposed_actions_json, verification_json, rollback, blockers_json, alternatives_json, dominant_factors_json, rule_id, rule_version FROM recommendations WHERE scan_id = ? ORDER BY priority DESC, family, id`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []recommend.Recommendation
	for rows.Next() {
		recommendation, err := scanRecommendation(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, recommendation)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for index := range out {
		affected, err := s.listRecommendationAssets(ctx, scanID, out[index].ID)
		if err != nil {
			return nil, err
		}
		out[index].AffectedAssetIDs = affected
		evidence, err := s.listRecommendationEvidence(ctx, scanID, out[index].ID)
		if err != nil {
			return nil, err
		}
		out[index].Evidence = evidence
	}
	return out, nil
}

// rowScanner is the subset of *sql.Rows that scanRecommendation needs.
type rowScanner interface {
	Scan(dest ...any) error
}

func scanRecommendation(rows rowScanner) (recommend.Recommendation, error) {
	var recommendation recommend.Recommendation
	var family, risk string
	var uncertain int
	var preconditions, proposedActions, verification, blockers, alternatives, dominant string
	if err := rows.Scan(&recommendation.ID, &family, &recommendation.Title, &recommendation.Ecosystem,
		&recommendation.Explanation, &risk, &recommendation.Confidence, &recommendation.Priority,
		&recommendation.Savings.LowBytes, &recommendation.Savings.HighBytes,
		&recommendation.Savings.FutureGrowthReductionBytes, &uncertain,
		&recommendation.RestorationCost, &recommendation.CompatibilityImpact,
		&preconditions, &proposedActions, &verification, &recommendation.Rollback,
		&blockers, &alternatives, &dominant,
		&recommendation.RuleID, &recommendation.RuleVersion); err != nil {
		return recommendation, err
	}
	recommendation.Family = recommend.Family(family)
	recommendation.Risk = assets.Risk(risk)
	recommendation.Savings.Uncertain = uncertain == 1
	for _, decode := range []struct {
		raw    string
		target any
	}{
		{raw: preconditions, target: &recommendation.Preconditions},
		{raw: proposedActions, target: &recommendation.ProposedActions},
		{raw: verification, target: &recommendation.Verification},
		{raw: blockers, target: &recommendation.Blockers},
		{raw: alternatives, target: &recommendation.Alternatives},
		{raw: dominant, target: &recommendation.DominantFactors},
	} {
		if err := json.Unmarshal([]byte(decode.raw), decode.target); err != nil {
			return recommendation, err
		}
	}
	return recommendation, nil
}

func (s *Store) listRecommendationAssets(ctx context.Context, scanID, recommendationID string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT asset_id FROM recommendation_assets WHERE scan_id = ? AND recommendation_id = ? ORDER BY asset_id`, scanID, recommendationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var assetID string
		if err := rows.Scan(&assetID); err != nil {
			return nil, err
		}
		out = append(out, assetID)
	}
	return out, rows.Err()
}

func (s *Store) listRecommendationEvidence(ctx context.Context, scanID, recommendationID string) ([]assets.Evidence, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT kind, value, confidence FROM recommendation_evidence WHERE scan_id = ? AND recommendation_id = ? ORDER BY kind, value`, scanID, recommendationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []assets.Evidence
	for rows.Next() {
		var evidence assets.Evidence
		if err := rows.Scan(&evidence.Kind, &evidence.Value, &evidence.Confidence); err != nil {
			return nil, err
		}
		out = append(out, evidence)
	}
	return out, rows.Err()
}
