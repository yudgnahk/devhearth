package recommend

import (
	"strings"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/portfolio"
)

var testNow = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

func byFamily(items []Recommendation, family Family) []Recommendation {
	var out []Recommendation
	for _, item := range items {
		if item.Family == family {
			out = append(out, item)
		}
	}
	return out
}

func requireOne(t *testing.T, items []Recommendation, family Family) Recommendation {
	t.Helper()
	matches := byFamily(items, family)
	if len(matches) != 1 {
		t.Fatalf("want exactly one %s recommendation, got %d: %#v", family, len(matches), families(items))
	}
	return matches[0]
}

func families(items []Recommendation) []Family {
	out := make([]Family, 0, len(items))
	for _, item := range items {
		out = append(out, item.Family)
	}
	return out
}

func TestRegistryOrdersRulesByID(t *testing.T) {
	rules := NewRegistry().Rules()
	if len(rules) == 0 {
		t.Fatal("no built-in rules registered")
	}
	for index := 1; index < len(rules); index++ {
		if rules[index-1].ID() >= rules[index].ID() {
			t.Fatalf("rules are not ordered by id: %q then %q", rules[index-1].ID(), rules[index].ID())
		}
	}
}

func TestRunStampsRuleIdentityAndPriority(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		versionManager("nvm", "node", "/home/.nvm", 2<<30),
		versionManager("fnm", "node", "/home/.fnm", 1<<30),
	}}

	results := NewRegistry().Run(Input{Graph: graph, Now: testNow})

	recommendation := requireOne(t, results, FamilyConsolidateRuntime)
	if recommendation.RuleID != "rule.consolidate_runtime" || recommendation.RuleVersion != 1 {
		t.Fatalf("rule identity not stamped: %#v", recommendation)
	}
	if recommendation.Priority <= 0 {
		t.Fatalf("priority = %v, want a positive expected value", recommendation.Priority)
	}
	if recommendation.ID == "" || !strings.HasPrefix(recommendation.ID, "rec_") {
		t.Fatalf("id = %q", recommendation.ID)
	}
}

func TestRunRanksHigherExpectedValueFirst(t *testing.T) {
	// A high-risk blocked worktree must not outrank a confident, safe saving of
	// comparable size.
	graph := assets.Graph{Assets: []assets.Asset{
		worktree("stale", "/roots/stale", 4<<30, testNow.Add(-400*24*time.Hour)),
		project("app", "node", "/roots/app", testNow.Add(-400*24*time.Hour)),
		packageManager("npm", "node", "/roots/app"),
		install("node_modules", "node", "/roots/app/node_modules", 4<<30),
	}}
	graph.Relationships = []assets.Relationship{{
		ID: "r1", SourceID: "asset:/roots/app", TargetID: "asset:/roots/app/node_modules",
		Kind: assets.RelProjectOwnsEnvironment, Confidence: 0.9,
	}}

	results := NewRegistry().Run(Input{Graph: graph, Now: testNow})

	hibernate := requireOne(t, results, FamilyHibernateProject)
	stale := requireOne(t, results, FamilyObsoleteWorktree)
	if hibernate.Priority <= stale.Priority {
		t.Fatalf("safe measured saving (%v) should outrank blocked high-risk advice (%v)", hibernate.Priority, stale.Priority)
	}
}

func TestRunIsDeterministic(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		versionManager("nvm", "node", "/home/.nvm", 2<<30),
		versionManager("fnm", "node", "/home/.fnm", 1<<30),
		modelFile("llama.gguf", "/roots/a/llama.gguf", 4<<30),
		modelFile("llama.gguf", "/roots/b/llama.gguf", 4<<30),
	}}

	first := NewRegistry().Run(Input{Graph: graph, Now: testNow})
	second := NewRegistry().Run(Input{Graph: graph, Now: testNow})

	if len(first) != len(second) {
		t.Fatalf("recommendation count differs: %d vs %d", len(first), len(second))
	}
	for index := range first {
		if first[index].ID != second[index].ID || first[index].Priority != second[index].Priority {
			t.Fatalf("run %d differs: %#v vs %#v", index, first[index], second[index])
		}
	}
}

func TestRunDefaultsInactivityWindow(t *testing.T) {
	// 100 days is dormant under a 90-day window but not under the 180-day default.
	lastActivity := testNow.Add(-100 * 24 * time.Hour)
	graph := assets.Graph{Assets: []assets.Asset{
		worktree("recent", "/roots/recent", 1<<30, lastActivity),
	}}

	if got := byFamily(NewRegistry().Run(Input{Graph: graph, Now: testNow}), FamilyObsoleteWorktree); len(got) != 0 {
		t.Fatalf("default window should not flag a 100-day-old worktree: %#v", got)
	}
	shorter := Input{Graph: graph, Now: testNow, InactiveAfter: 90 * 24 * time.Hour}
	if got := byFamily(NewRegistry().Run(shorter), FamilyObsoleteWorktree); len(got) != 1 {
		t.Fatalf("explicit 90-day window should flag it: %#v", got)
	}
}

func TestRunNeverProducesRecommendationsForAnEmptyGraph(t *testing.T) {
	if results := NewRegistry().Run(Input{Now: testNow}); len(results) != 0 {
		t.Fatalf("empty graph produced advice: %#v", families(results))
	}
}

func TestPriorityDampsRiskAndBlockers(t *testing.T) {
	base := Recommendation{Confidence: 0.8, Savings: Savings{LowBytes: 1 << 30, HighBytes: 1 << 30}}
	low := base
	low.Risk = assets.RiskLow
	high := base
	high.Risk = assets.RiskHigh
	blocked := low
	blocked.Blockers = []string{"unverified"}

	if priority(high) >= priority(low) {
		t.Fatal("high risk must rank below low risk for the same value")
	}
	if priority(blocked) >= priority(low) {
		t.Fatal("blockers must dampen priority")
	}
	prohibited := base
	prohibited.Risk = assets.RiskProhibited
	if priority(prohibited) != 0 {
		t.Fatalf("prohibited advice must carry no priority, got %v", priority(prohibited))
	}
}

func TestNewIDIsStableAndPathFree(t *testing.T) {
	first := newID(FamilyHibernateProject, "node", "/Users/example/Projects/app")
	second := newID(FamilyHibernateProject, "node", "/Users/example/Projects/app")
	other := newID(FamilyHibernateProject, "node", "/Users/example/Projects/other")

	if first != second {
		t.Fatal("identical scope keys must produce identical ids")
	}
	if first == other {
		t.Fatal("different scope keys must produce different ids")
	}
	if strings.Contains(first, "Users") || strings.Contains(first, "app") {
		t.Fatalf("id leaks its scope key: %q", first)
	}
}

func TestRunPassesAssessmentsToFitRules(t *testing.T) {
	assessment := portfolio.Assessment{
		Ecosystem:                "node",
		Depth:                    portfolio.DepthDeep,
		ProjectCount:             3,
		Baseline:                 "npm",
		ProjectLocalInstallBytes: 6 << 30,
		Options: []portfolio.Option{
			{Tool: "pnpm", Rank: 1, Score: 0.7, Confidence: 0.7, Installed: true,
				ImmediateSavingsLowBytes: 1 << 30, ImmediateSavingsHighBytes: 3 << 30,
				FutureGrowthReductionBytes: 2 << 30, SavingsUncertain: true},
			{Tool: "npm", StayPut: true, Rank: 2, Score: 0.6, Confidence: 0.8},
		},
	}
	graph := assets.Graph{Assets: []assets.Asset{
		install("node_modules", "node", "/roots/a/node_modules", 3<<30),
		install("node_modules", "node", "/roots/b/node_modules", 3<<30),
	}}

	results := NewRegistry().Run(Input{Graph: graph, Assessments: []portfolio.Assessment{assessment}, Now: testNow})

	fit := requireOne(t, results, FamilyPortfolioFit)
	if fit.Savings.HighBytes != 3<<30 {
		t.Fatalf("fit savings not carried through: %#v", fit.Savings)
	}
	if len(fit.Alternatives) == 0 {
		t.Fatal("fit advice must list the alternatives considered")
	}
	store := requireOne(t, results, FamilyAdoptSharedStore)
	if len(store.AffectedAssetIDs) != 2 {
		t.Fatalf("shared-store advice should name both installs: %#v", store.AffectedAssetIDs)
	}
}

// Test asset builders. IDs mirror the "asset:<path>" shape so relationship
// wiring in tests stays readable.

func project(name, ecosystem, path string, lastActivity time.Time) assets.Asset {
	return assets.Asset{
		ID: "asset:" + path, Kind: assets.KindProject, DisplayName: name, Path: path,
		Ecosystem: ecosystem, Class: assets.ClassProject, Risk: assets.RiskInformational,
		LastActivityAt: lastActivity,
		Evidence:       []assets.Evidence{{Kind: "path_signature", Value: path, Confidence: 0.9}},
	}
}

func packageManager(tool, ecosystem, path string) assets.Asset {
	return assets.Asset{
		ID: "asset:" + path + ":" + tool, Kind: assets.KindPackageManager, DisplayName: tool, Path: path,
		Ecosystem: ecosystem, Class: assets.ClassPackageManager, Risk: assets.RiskInformational,
		Attributes: map[string]string{"tool": tool, "scope": "project"},
		Evidence:   []assets.Evidence{{Kind: "lockfile", Value: tool + ".lock", Confidence: 0.95}},
	}
}

func install(name, ecosystem, path string, bytes int64) assets.Asset {
	return assets.Asset{
		ID: "asset:" + path, Kind: assets.KindProjectLocalInstall, DisplayName: name, Path: path,
		Ecosystem: ecosystem, Class: assets.ClassProjectLocalInstall, Risk: assets.RiskLow,
		Size:     assets.Size{Attributed: true, AllocatedBytes: bytes, ExclusiveAllocatedBytes: bytes},
		Evidence: []assets.Evidence{{Kind: "path_signature", Value: path, Confidence: 0.95}},
	}
}

func versionManager(tool, ecosystem, path string, bytes int64) assets.Asset {
	return assets.Asset{
		ID: "asset:" + path, Kind: assets.KindVersionManager, DisplayName: tool, Path: path,
		Ecosystem: ecosystem, Class: assets.ClassVersionManager, Risk: assets.RiskMedium,
		Attributes: map[string]string{"tool": tool, "scope": "machine"},
		Size:       assets.Size{Attributed: true, AllocatedBytes: bytes, ExclusiveAllocatedBytes: bytes, Shared: true},
		Evidence:   []assets.Evidence{{Kind: "well_known_path", Value: path, Confidence: 0.9}},
	}
}

func worktree(name, path string, bytes int64, lastActivity time.Time) assets.Asset {
	return assets.Asset{
		ID: "asset:" + path, Kind: assets.KindGitWorktree, DisplayName: name, Path: path,
		Class: assets.ClassGit, Risk: assets.RiskInformational, LastActivityAt: lastActivity,
		Attributes: map[string]string{"git_kind": "worktree"},
		Size:       assets.Size{Attributed: true, AllocatedBytes: bytes, ExclusiveAllocatedBytes: bytes},
		Evidence:   []assets.Evidence{{Kind: "path_signature", Value: path + "/.git", Confidence: 0.9}},
	}
}

func modelFile(name, path string, bytes int64) assets.Asset {
	return assets.Asset{
		ID: "asset:" + path, Kind: assets.KindAIAsset, DisplayName: name, Path: path,
		Ecosystem: "ai", Class: assets.ClassAI, Risk: assets.RiskMedium,
		Attributes: map[string]string{"ai_kind": "model_file"},
		Size:       assets.Size{Attributed: true, LogicalBytes: bytes, AllocatedBytes: bytes, ExclusiveAllocatedBytes: bytes},
		Evidence:   []assets.Evidence{{Kind: "path_signature", Value: path, Confidence: 0.75}},
	}
}
