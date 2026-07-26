package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/advisor"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/portfolio"
	"github.com/yudgnahk/devhearth/internal/recommend"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// adviceFixture is one project with an install, one fit assessment, and one
// recommendation that names the install as an affected asset.
func adviceFixture(now time.Time) (scan.Result, advisor.Result) {
	result := scan.Result{
		Roots: []string{"/fixtures"}, StartedAt: now, CompletedAt: now,
		Entries: []scan.Entry{
			{Path: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now},
			{Path: "/fixtures/app", ParentPath: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 2, LinkCount: 1, ModifiedAt: now},
			{Path: "/fixtures/app/node_modules", ParentPath: "/fixtures/app", Kind: "directory", DeviceID: 1, Inode: 3, LinkCount: 1, ModifiedAt: now},
		},
	}
	advice := advisor.Result{
		Graph: assets.Graph{Assets: []assets.Asset{
			{
				ID: "asset-project", Kind: assets.KindProject, DisplayName: "app", Path: "/fixtures/app",
				Risk: assets.RiskInformational, Ecosystem: "node", Class: assets.ClassProject,
				DetectorID: "detect.node", DetectorVersion: 1,
				Size:           assets.Size{Attributed: true, LogicalBytes: 10, AllocatedBytes: 8192, ExclusiveAllocatedBytes: 4096},
				LastActivityAt: now,
			},
			{
				ID: "asset-install", Kind: assets.KindProjectLocalInstall, DisplayName: "node_modules",
				Path: "/fixtures/app/node_modules", Risk: assets.RiskLow, Ecosystem: "node",
				Class: assets.ClassProjectLocalInstall, DetectorID: "detect.node", DetectorVersion: 1,
				Size: assets.Size{Attributed: true, LogicalBytes: 5, AllocatedBytes: 4096, ExclusiveAllocatedBytes: 4096, Uncertain: true},
			},
		}},
		Assessments: []portfolio.Assessment{{
			Ecosystem: "node", Depth: portfolio.DepthDeep, ProjectCount: 1, Baseline: "npm",
			RecommendedTool: "pnpm", ProjectLocalInstallBytes: 4096, VersionManagers: []string{"nvm"},
			Notes: []string{"no content hashing has run"},
			Options: []portfolio.Option{
				{
					Tool: "pnpm", Rank: 1, Score: 0.71, Confidence: 0.7, Installed: true,
					ImmediateSavingsLowBytes: 1024, ImmediateSavingsHighBytes: 2048,
					FutureGrowthReductionBytes: 1536, SavingsUncertain: true,
					WorkflowImpact: "moderate workflow change",
					Factors: []portfolio.Factor{
						{Kind: portfolio.FactorInUseShare, Score: 0.5, Weight: 0.3, Detail: "1 of 2 projects"},
					},
					DominantFactors: []portfolio.FactorKind{portfolio.FactorInUseShare},
					Blockers:        []string{"pnpm is not installed"},
				},
				{Tool: "npm", StayPut: true, Rank: 2, Score: 0.62, Confidence: 0.8, ProjectsUsing: 1},
			},
		}},
		Recommendations: []recommend.Recommendation{{
			ID: "rec_abcdef123456", Family: recommend.FamilyAdoptSharedStore, Title: "Share node dependencies",
			Ecosystem: "node", Explanation: "two installs could share one store",
			Risk: assets.RiskMedium, Confidence: 0.65, Priority: 1234.5,
			Savings:         recommend.Savings{LowBytes: 1024, HighBytes: 2048, FutureGrowthReductionBytes: 1536, Uncertain: true},
			RestorationCost: "one install per project", CompatibilityImpact: "moderate",
			Preconditions:   []string{"every project has a lockfile"},
			ProposedActions: []string{"migrate one project first"},
			Verification:    []string{"each project builds"},
			Rollback:        "reinstall with the previous tool",
			Blockers:        []string{"native addons unverified"},
			Alternatives: []recommend.Alternative{
				{Label: "npm", StayPut: true, Rank: 2, Score: 0.62},
			},
			DominantFactors:  []string{"in_use_share"},
			AffectedAssetIDs: []string{"asset-install"},
			Evidence:         []assets.Evidence{{Kind: "path_signature", Value: "/fixtures/app/node_modules", Confidence: 0.95}},
			RuleID:           "rule.adopt_shared_store", RuleVersion: 1,
		}},
	}
	return result, advice
}

func openStore(t *testing.T) *Store {
	t.Helper()
	database, err := Open(filepath.Join(t.TempDir(), "inventory.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { database.Close() })
	return database
}

func TestSavePersistsAttributedAssetSizes(t *testing.T) {
	database := openStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	result, advice := adviceFixture(now)

	scanID, err := database.Save(context.Background(), result, advice, "complete")
	if err != nil {
		t.Fatal(err)
	}

	stored, err := database.ListAssets(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]assets.Asset{}
	for _, asset := range stored {
		byID[asset.ID] = asset
	}
	project := byID["asset-project"]
	if !project.Size.Attributed || project.Size.AllocatedBytes != 8192 || project.Size.ExclusiveAllocatedBytes != 4096 {
		t.Fatalf("project size round-trip failed: %#v", project.Size)
	}
	if project.LastActivityAt.IsZero() || !project.LastActivityAt.Equal(now) {
		t.Fatalf("last activity = %s, want %s", project.LastActivityAt, now)
	}
	install := byID["asset-install"]
	if !install.Size.Uncertain {
		t.Fatalf("uncertainty must survive persistence: %#v", install.Size)
	}
}

func TestSavePersistsFitAssessments(t *testing.T) {
	database := openStore(t)
	result, advice := adviceFixture(time.Now().UTC())

	scanID, err := database.Save(context.Background(), result, advice, "complete")
	if err != nil {
		t.Fatal(err)
	}

	stored, err := database.ListFitAssessments(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("want one assessment, got %d", len(stored))
	}
	assessment := stored[0]
	if assessment.Ecosystem != "node" || assessment.Depth != portfolio.DepthDeep {
		t.Fatalf("assessment header round-trip failed: %#v", assessment)
	}
	if len(assessment.Options) != 2 || assessment.Options[0].Tool != "pnpm" || assessment.Options[0].Rank != 1 {
		t.Fatalf("options should round-trip in rank order: %#v", assessment.Options)
	}
	if len(assessment.Options[0].Factors) != 1 || assessment.Options[0].Factors[0].Kind != portfolio.FactorInUseShare {
		t.Fatalf("factor evidence lost: %#v", assessment.Options[0].Factors)
	}
	if !assessment.Options[1].StayPut {
		t.Fatal("the stay-put baseline must survive persistence")
	}
	if len(assessment.Notes) != 1 {
		t.Fatalf("scope-limit notes lost: %#v", assessment.Notes)
	}
}

func TestSavePersistsRecommendationsWithEvidenceAndAssets(t *testing.T) {
	database := openStore(t)
	result, advice := adviceFixture(time.Now().UTC())

	scanID, err := database.Save(context.Background(), result, advice, "complete")
	if err != nil {
		t.Fatal(err)
	}

	stored, err := database.ListRecommendations(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 {
		t.Fatalf("want one recommendation, got %d", len(stored))
	}
	recommendation := stored[0]
	if recommendation.ID != "rec_abcdef123456" || recommendation.Family != recommend.FamilyAdoptSharedStore {
		t.Fatalf("identity round-trip failed: %#v", recommendation)
	}
	if recommendation.Savings.LowBytes != 1024 || recommendation.Savings.HighBytes != 2048 || !recommendation.Savings.Uncertain {
		t.Fatalf("savings range round-trip failed: %#v", recommendation.Savings)
	}
	if len(recommendation.Preconditions) != 1 || len(recommendation.Verification) != 1 || recommendation.Rollback == "" {
		t.Fatalf("safety fields lost: %#v", recommendation)
	}
	if len(recommendation.Blockers) != 1 {
		t.Fatalf("blockers lost: %#v", recommendation.Blockers)
	}
	if len(recommendation.Alternatives) != 1 || !recommendation.Alternatives[0].StayPut {
		t.Fatalf("alternatives lost: %#v", recommendation.Alternatives)
	}
	if len(recommendation.AffectedAssetIDs) != 1 || recommendation.AffectedAssetIDs[0] != "asset-install" {
		t.Fatalf("affected assets lost: %#v", recommendation.AffectedAssetIDs)
	}
	if len(recommendation.Evidence) != 1 {
		t.Fatalf("evidence lost: %#v", recommendation.Evidence)
	}
	if recommendation.RuleID != "rule.adopt_shared_store" || recommendation.RuleVersion != 1 {
		t.Fatalf("rule identity lost: %#v", recommendation)
	}
}

func TestSaveWithoutAdviceLeavesAdviceTablesEmpty(t *testing.T) {
	database := openStore(t)
	result, advice := adviceFixture(time.Now().UTC())
	advice.Assessments = nil
	advice.Recommendations = nil

	scanID, err := database.Save(context.Background(), result, advice, "complete")
	if err != nil {
		t.Fatal(err)
	}

	assessments, err := database.ListFitAssessments(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	recommendations, err := database.ListRecommendations(context.Background(), scanID)
	if err != nil {
		t.Fatal(err)
	}
	if len(assessments) != 0 || len(recommendations) != 0 {
		t.Fatalf("expected no advice rows, got %d assessments and %d recommendations", len(assessments), len(recommendations))
	}
}

func TestSavePersistsRollupModificationTimeAndAliasCounts(t *testing.T) {
	database := openStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	result := scan.Result{
		Roots: []string{"/fixtures"}, StartedAt: now, CompletedAt: now,
		Entries: []scan.Entry{
			{Path: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now},
			{Path: "/fixtures/linked", ParentPath: "/fixtures", Kind: "file", DeviceID: 1, Inode: 2,
				LinkCount: 2, HardLinkAlias: true, ModifiedAt: now},
		},
	}

	scanID, err := database.Save(context.Background(), result, advisor.Result{}, "complete")
	if err != nil {
		t.Fatal(err)
	}

	var aliasCount int64
	var modifiedAt string
	row := database.db.QueryRowContext(context.Background(),
		`SELECT hard_link_alias_count, modified_at FROM directory_aggregates WHERE scan_id = ? AND path = ?`,
		scanID, "/fixtures")
	if err := row.Scan(&aliasCount, &modifiedAt); err != nil {
		t.Fatal(err)
	}
	if aliasCount != 1 {
		t.Fatalf("hard-link alias rollup = %d, want 1", aliasCount)
	}
	if modifiedAt == "" {
		t.Fatal("rollup modification time was not persisted")
	}
}
