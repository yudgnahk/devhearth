package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/portfolio"
	"github.com/yudgnahk/devhearth/internal/recommend"
)

// insertAdvice persists fit assessments and recommendations for one scan. It
// runs inside the same transaction as the inventory so a scan is never durable
// with advice that disagrees with the graph it was derived from.
func insertAdvice(ctx context.Context, tx *sql.Tx, scanID string, advice advisor.Result) error {
	if err := insertAssessments(ctx, tx, scanID, advice.Assessments); err != nil {
		return fmt.Errorf("persist fit assessments: %w", err)
	}
	if err := insertRecommendations(ctx, tx, scanID, advice.Recommendations); err != nil {
		return fmt.Errorf("persist recommendations: %w", err)
	}
	return nil
}

func insertAssessments(ctx context.Context, tx *sql.Tx, scanID string, assessments []portfolio.Assessment) error {
	if len(assessments) == 0 {
		return nil
	}
	assessmentStmt, err := tx.PrepareContext(ctx, `INSERT INTO fit_assessments(id, scan_id, ecosystem, depth, project_count, baseline_tool, recommended_tool, stay_put_wins, project_local_install_bytes, shared_store_bytes, version_managers_json, notes_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer assessmentStmt.Close()
	optionStmt, err := tx.PrepareContext(ctx, `INSERT INTO fit_options(id, assessment_id, tool, rank, score, confidence, stay_put, installed, projects_using, immediate_savings_low_bytes, immediate_savings_high_bytes, future_growth_reduction_bytes, savings_uncertain, workflow_impact, factors_json, dominant_factors_json, blockers_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer optionStmt.Close()

	for _, assessment := range assessments {
		assessmentID := uuid.NewString()
		versionManagers, err := encodeJSON(assessment.VersionManagers)
		if err != nil {
			return err
		}
		notes, err := encodeJSON(assessment.Notes)
		if err != nil {
			return err
		}
		if _, err := assessmentStmt.ExecContext(ctx,
			assessmentID, scanID, assessment.Ecosystem, string(assessment.Depth), assessment.ProjectCount,
			assessment.Baseline, assessment.RecommendedTool, boolToInt(assessment.StayPutWins),
			assessment.ProjectLocalInstallBytes, assessment.SharedStoreBytes, versionManagers, notes,
		); err != nil {
			return err
		}
		for _, option := range assessment.Options {
			factors, err := encodeJSON(option.Factors)
			if err != nil {
				return err
			}
			dominant, err := encodeJSON(option.DominantFactors)
			if err != nil {
				return err
			}
			blockers, err := encodeJSON(option.Blockers)
			if err != nil {
				return err
			}
			if _, err := optionStmt.ExecContext(ctx,
				uuid.NewString(), assessmentID, option.Tool, option.Rank, option.Score, option.Confidence,
				boolToInt(option.StayPut), boolToInt(option.Installed), option.ProjectsUsing,
				option.ImmediateSavingsLowBytes, option.ImmediateSavingsHighBytes,
				option.FutureGrowthReductionBytes, boolToInt(option.SavingsUncertain),
				option.WorkflowImpact, factors, dominant, blockers,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertRecommendations(ctx context.Context, tx *sql.Tx, scanID string, recommendations []recommend.Recommendation) error {
	if len(recommendations) == 0 {
		return nil
	}
	recommendationStmt, err := tx.PrepareContext(ctx, `INSERT INTO recommendations(id, scan_id, family, title, ecosystem, explanation, risk, confidence, priority, savings_low_bytes, savings_high_bytes, future_growth_reduction_bytes, savings_uncertain, restoration_cost, compatibility_impact, preconditions_json, proposed_actions_json, verification_json, rollback, blockers_json, alternatives_json, dominant_factors_json, rule_id, rule_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer recommendationStmt.Close()
	assetStmt, err := tx.PrepareContext(ctx, `INSERT OR IGNORE INTO recommendation_assets(scan_id, recommendation_id, asset_id) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer assetStmt.Close()
	evidenceStmt, err := tx.PrepareContext(ctx, `INSERT INTO recommendation_evidence(id, scan_id, recommendation_id, kind, value, confidence) VALUES (?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer evidenceStmt.Close()

	for _, recommendation := range recommendations {
		lists, err := encodeLists(recommendation)
		if err != nil {
			return err
		}
		if _, err := recommendationStmt.ExecContext(ctx,
			recommendation.ID, scanID, string(recommendation.Family), recommendation.Title, recommendation.Ecosystem,
			recommendation.Explanation, string(recommendation.Risk), recommendation.Confidence, recommendation.Priority,
			recommendation.Savings.LowBytes, recommendation.Savings.HighBytes,
			recommendation.Savings.FutureGrowthReductionBytes, boolToInt(recommendation.Savings.Uncertain),
			recommendation.RestorationCost, recommendation.CompatibilityImpact,
			lists.preconditions, lists.proposedActions, lists.verification, recommendation.Rollback,
			lists.blockers, lists.alternatives, lists.dominantFactors,
			recommendation.RuleID, recommendation.RuleVersion,
		); err != nil {
			return err
		}
		for _, assetID := range recommendation.AffectedAssetIDs {
			if _, err := assetStmt.ExecContext(ctx, scanID, recommendation.ID, assetID); err != nil {
				return err
			}
		}
		for _, evidence := range recommendation.Evidence {
			if _, err := evidenceStmt.ExecContext(ctx,
				uuid.NewString(), scanID, recommendation.ID, evidence.Kind, evidence.Value, evidence.Confidence,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// encodedLists holds the JSON columns of one recommendation.
type encodedLists struct {
	preconditions   string
	proposedActions string
	verification    string
	blockers        string
	alternatives    string
	dominantFactors string
}

func encodeLists(recommendation recommend.Recommendation) (encodedLists, error) {
	var out encodedLists
	var err error
	if out.preconditions, err = encodeJSON(recommendation.Preconditions); err != nil {
		return out, err
	}
	if out.proposedActions, err = encodeJSON(recommendation.ProposedActions); err != nil {
		return out, err
	}
	if out.verification, err = encodeJSON(recommendation.Verification); err != nil {
		return out, err
	}
	if out.blockers, err = encodeJSON(recommendation.Blockers); err != nil {
		return out, err
	}
	if out.alternatives, err = encodeJSON(recommendation.Alternatives); err != nil {
		return out, err
	}
	out.dominantFactors, err = encodeJSON(recommendation.DominantFactors)
	return out, err
}

// encodeJSON renders a list column, normalizing nil to an empty JSON array so
// readers never have to distinguish null from empty.
func encodeJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if string(data) == "null" {
		return "[]", nil
	}
	return string(data), nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
