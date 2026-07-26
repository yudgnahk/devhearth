package recommend

import (
	"strings"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/portfolio"
)

var dormant = testNow.Add(-400 * 24 * time.Hour)

func TestRuntimeRuleNeedsTwoDistinctManagers(t *testing.T) {
	cases := []struct {
		name     string
		managers []assets.Asset
		want     int
	}{
		{
			name:     "single manager is not duplication",
			managers: []assets.Asset{versionManager("nvm", "node", "/home/.nvm", 1<<30)},
			want:     0,
		},
		{
			name: "same tool in two places is one manager",
			managers: []assets.Asset{
				versionManager("nvm", "node", "/home/.nvm", 1<<30),
				versionManager("nvm", "node", "/opt/.nvm", 1<<30),
			},
			want: 0,
		},
		{
			name: "two node managers overlap",
			managers: []assets.Asset{
				versionManager("nvm", "node", "/home/.nvm", 2<<30),
				versionManager("volta", "node", "/home/.volta", 1<<30),
			},
			want: 1,
		},
		{
			name: "a polyglot manager overlaps an ecosystem-specific one",
			managers: []assets.Asset{
				versionManager("nvm", "node", "/home/.nvm", 2<<30),
				versionManager("mise", "polyglot", "/home/.mise", 3<<30),
			},
			want: 1,
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rule := &runtimeConsolidationRule{}
			got := rule.Evaluate(Input{Graph: assets.Graph{Assets: testCase.managers}, Now: testNow})
			if len(got) != testCase.want {
				t.Fatalf("got %d recommendations, want %d", len(got), testCase.want)
			}
		})
	}
}

func TestRuntimeRuleKeepsLargestManagerAndNeverClaimsAFloor(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		versionManager("nvm", "node", "/home/.nvm", 1<<30),
		versionManager("volta", "node", "/home/.volta", 4<<30),
	}}

	got := (&runtimeConsolidationRule{}).Evaluate(Input{Graph: graph, Now: testNow})[0]

	if !strings.Contains(got.Explanation, "Keeping volta") {
		t.Fatalf("largest manager should be the keeper: %q", got.Explanation)
	}
	if got.Savings.LowBytes != 0 {
		t.Fatalf("unverified versions must keep the low bound at zero, got %d", got.Savings.LowBytes)
	}
	if got.Savings.HighBytes != 1<<30 {
		t.Fatalf("high bound should be the smaller install, got %d", got.Savings.HighBytes)
	}
	if len(got.Blockers) == 0 {
		t.Fatal("version comparison gap must be reported as a blocker")
	}
}

func TestHibernationRuleRequiresLockfileAndOwnedInstall(t *testing.T) {
	withLock := assets.Graph{Assets: []assets.Asset{
		project("app", "node", "/roots/app", dormant),
		packageManager("npm", "node", "/roots/app"),
		install("node_modules", "node", "/roots/app/node_modules", 2<<30),
	}}
	noLock := assets.Graph{Assets: []assets.Asset{
		project("app", "node", "/roots/app", dormant),
		install("node_modules", "node", "/roots/app/node_modules", 2<<30),
	}}
	noInstall := assets.Graph{Assets: []assets.Asset{
		project("app", "node", "/roots/app", dormant),
		packageManager("npm", "node", "/roots/app"),
	}}

	rule := &hibernationRule{}
	input := func(graph assets.Graph) Input {
		return Input{Graph: graph, Now: testNow, InactiveAfter: DefaultInactiveAfter}
	}
	if got := rule.Evaluate(input(withLock)); len(got) != 1 {
		t.Fatalf("reproducible dormant project should be a candidate: %#v", got)
	}
	if got := rule.Evaluate(input(noLock)); len(got) != 0 {
		t.Fatalf("without a lockfile the project is not restorable: %#v", got)
	}
	if got := rule.Evaluate(input(noInstall)); len(got) != 0 {
		t.Fatalf("without generated material there is nothing to hibernate: %#v", got)
	}
}

func TestHibernationRuleIgnoresActiveAndUnknownActivity(t *testing.T) {
	rule := &hibernationRule{}
	for _, testCase := range []struct {
		name     string
		activity time.Time
	}{
		{name: "recently active", activity: testNow.Add(-24 * time.Hour)},
		{name: "unknown activity", activity: time.Time{}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			graph := assets.Graph{Assets: []assets.Asset{
				project("app", "node", "/roots/app", testCase.activity),
				packageManager("npm", "node", "/roots/app"),
				install("node_modules", "node", "/roots/app/node_modules", 2<<30),
			}}
			if got := rule.Evaluate(Input{Graph: graph, Now: testNow, InactiveAfter: DefaultInactiveAfter}); len(got) != 0 {
				t.Fatalf("expected no advice, got %#v", got)
			}
		})
	}
}

func TestHibernationRuleStatesGitAndDataPreconditions(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		project("app", "node", "/roots/app", dormant),
		packageManager("npm", "node", "/roots/app"),
		install("node_modules", "node", "/roots/app/node_modules", 2<<30),
	}}

	got := (&hibernationRule{}).Evaluate(Input{Graph: graph, Now: testNow, InactiveAfter: DefaultInactiveAfter})[0]

	joined := strings.Join(got.Preconditions, " | ")
	for _, needle := range []string{"uncommitted", "remotes", "lockfile", "user data"} {
		if !strings.Contains(joined, needle) {
			t.Fatalf("preconditions missing %q: %q", needle, joined)
		}
	}
	if got.Risk != assets.RiskLow {
		t.Fatalf("risk = %q, want low for reproducible project-local data", got.Risk)
	}
	if len(got.Blockers) == 0 {
		t.Fatal("the missing Git verification must remain a blocker")
	}
	if got.Rollback == "" || len(got.Verification) == 0 {
		t.Fatal("hibernation advice needs verification and rollback")
	}
}

func TestWorktreeRuleTreatsUnverifiedGitStateAsHighRisk(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{worktree("feature", "/roots/feature", 3<<30, dormant)}}

	got := (&obsoleteWorktreeRule{}).Evaluate(Input{Graph: graph, Now: testNow, InactiveAfter: DefaultInactiveAfter})[0]

	if got.Risk != assets.RiskHigh {
		t.Fatalf("risk = %q, want high for unverified Git state", got.Risk)
	}
	if got.Savings.LowBytes != 0 {
		t.Fatalf("nothing may be promised before verification, got low bound %d", got.Savings.LowBytes)
	}
	if !strings.Contains(strings.Join(got.ProposedActions, " "), "Git") {
		t.Fatalf("removal must go through Git: %#v", got.ProposedActions)
	}
}

func TestDuplicateModelRuleExcludesFineTunesAndSmallFiles(t *testing.T) {
	fineTune := modelFile("adapter.safetensors", "/roots/a/lora/adapter.safetensors", 4<<30)
	fineTune.Risk = assets.RiskHigh
	fineTune.Attributes = map[string]string{"ai_kind": "fine_tune"}
	otherFineTune := fineTune
	otherFineTune.ID = "asset:/roots/b/lora/adapter.safetensors"
	otherFineTune.Path = "/roots/b/lora/adapter.safetensors"

	small := modelFile("tiny.gguf", "/roots/a/tiny.gguf", 1024)
	otherSmall := small
	otherSmall.ID = "asset:/roots/b/tiny.gguf"
	otherSmall.Path = "/roots/b/tiny.gguf"

	graph := assets.Graph{Assets: []assets.Asset{fineTune, otherFineTune, small, otherSmall}}

	if got := (&duplicateModelRule{}).Evaluate(Input{Graph: graph, Now: testNow}); len(got) != 0 {
		t.Fatalf("fine-tunes and tiny files must never be duplication candidates: %#v", got)
	}
}

func TestDuplicateModelRuleRequiresMatchingSize(t *testing.T) {
	differentSize := assets.Graph{Assets: []assets.Asset{
		modelFile("llama.gguf", "/roots/a/llama.gguf", 4<<30),
		modelFile("llama.gguf", "/roots/b/llama.gguf", 5<<30),
	}}
	sameSize := assets.Graph{Assets: []assets.Asset{
		modelFile("llama.gguf", "/roots/a/llama.gguf", 4<<30),
		modelFile("llama.gguf", "/roots/b/llama.gguf", 4<<30),
	}}

	rule := &duplicateModelRule{}
	if got := rule.Evaluate(Input{Graph: differentSize, Now: testNow}); len(got) != 0 {
		t.Fatalf("same name with different size is not evidence: %#v", got)
	}
	got := rule.Evaluate(Input{Graph: sameSize, Now: testNow})
	if len(got) != 1 {
		t.Fatalf("want one candidate, got %d", len(got))
	}
	if got[0].Savings.HighBytes != 4<<30 || got[0].Savings.LowBytes != 0 {
		t.Fatalf("savings should span zero to one copy, got %#v", got[0].Savings)
	}
	if !strings.Contains(strings.Join(got[0].Blockers, " "), "content comparison") {
		t.Fatalf("missing hash comparison must be a blocker: %#v", got[0].Blockers)
	}
}

func TestSharedStoreRuleOnlyProposesInstalledTools(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		install("node_modules", "node", "/roots/a/node_modules", 3<<30),
		install("node_modules", "node", "/roots/b/node_modules", 3<<30),
	}}
	notInstalled := portfolio.Assessment{
		Ecosystem: "node", Depth: portfolio.DepthDeep, ProjectLocalInstallBytes: 6 << 30,
		Options: []portfolio.Option{{
			Tool: "pnpm", Rank: 1, Confidence: 0.5, Installed: false,
			ImmediateSavingsLowBytes: 1 << 30, ImmediateSavingsHighBytes: 2 << 30,
		}},
	}
	installed := notInstalled
	installed.Options = []portfolio.Option{{
		Tool: "pnpm", Rank: 1, Confidence: 0.7, Installed: true,
		ImmediateSavingsLowBytes: 1 << 30, ImmediateSavingsHighBytes: 2 << 30,
	}}

	rule := &sharedStoreRule{}
	if got := rule.Evaluate(Input{Graph: graph, Assessments: []portfolio.Assessment{notInstalled}, Now: testNow}); len(got) != 0 {
		t.Fatalf("a tool that is not installed belongs to fit advice, not this rule: %#v", got)
	}
	if got := rule.Evaluate(Input{Graph: graph, Assessments: []portfolio.Assessment{installed}, Now: testNow}); len(got) != 1 {
		t.Fatalf("want one shared-store recommendation, got %#v", got)
	}
}

func TestSharedStoreRuleSkipsShallowEcosystems(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		install("target", "rust", "/roots/a/target", 3<<30),
		install("target", "rust", "/roots/b/target", 3<<30),
	}}
	shallow := portfolio.Assessment{
		Ecosystem: "rust", Depth: portfolio.DepthShallow, ProjectLocalInstallBytes: 6 << 30,
		Options: []portfolio.Option{{Tool: "cargo", Installed: true, ImmediateSavingsHighBytes: 2 << 30}},
	}

	if got := (&sharedStoreRule{}).Evaluate(Input{Graph: graph, Assessments: []portfolio.Assessment{shallow}, Now: testNow}); len(got) != 0 {
		t.Fatalf("shallow ecosystems must not receive migration advice: %#v", got)
	}
}

func TestFitRuleReportsStayPutAsAnOutcome(t *testing.T) {
	assessment := portfolio.Assessment{
		Ecosystem: "node", Depth: portfolio.DepthDeep, ProjectCount: 4, Baseline: "npm",
		StayPutWins: true,
		Options: []portfolio.Option{
			{Tool: "npm", StayPut: true, Rank: 1, Score: 0.72, Confidence: 0.8, ProjectsUsing: 4},
			{Tool: "pnpm", Rank: 2, Score: 0.61, Confidence: 0.6, ImmediateSavingsHighBytes: 2 << 30},
		},
	}

	got := (&portfolioFitRule{}).Evaluate(Input{Assessments: []portfolio.Assessment{assessment}, Now: testNow})

	if len(got) != 1 {
		t.Fatalf("want one fit recommendation, got %d", len(got))
	}
	if got[0].Risk != assets.RiskInformational {
		t.Fatalf("a stay-put outcome is informational, got %q", got[0].Risk)
	}
	if got[0].Savings.HighBytes != 0 {
		t.Fatalf("staying put saves nothing today: %#v", got[0].Savings)
	}
	if len(got[0].Alternatives) != 1 || got[0].Alternatives[0].Label != "pnpm" {
		t.Fatalf("alternatives must stay visible: %#v", got[0].Alternatives)
	}
}

func TestFitRuleCarriesFactorEvidence(t *testing.T) {
	assessment := portfolio.Assessment{
		Ecosystem: "node", Depth: portfolio.DepthDeep, ProjectCount: 3, Baseline: "npm",
		Notes: []string{"no content hashing has run"},
		Options: []portfolio.Option{
			{Tool: "pnpm", Rank: 1, Score: 0.8, Confidence: 0.7, Installed: true,
				Factors:         []portfolio.Factor{{Kind: portfolio.FactorInUseShare, Score: 0.66, Weight: 0.3, Detail: "2 of 3 projects"}},
				DominantFactors: []portfolio.FactorKind{portfolio.FactorInUseShare}},
			{Tool: "npm", StayPut: true, Rank: 2, Score: 0.5, Confidence: 0.8},
		},
	}

	got := (&portfolioFitRule{}).Evaluate(Input{Assessments: []portfolio.Assessment{assessment}, Now: testNow})[0]

	var sawFactor, sawScope bool
	for _, evidence := range got.Evidence {
		if strings.HasPrefix(evidence.Kind, "fit_factor:") {
			sawFactor = true
		}
		if evidence.Kind == "scope_limit" {
			sawScope = true
		}
	}
	if !sawFactor || !sawScope {
		t.Fatalf("fit evidence must include factors and scope limits: %#v", got.Evidence)
	}
	if len(got.DominantFactors) == 0 {
		t.Fatal("the signals that decided the ranking must be reported")
	}
}

func TestFitRuleSkipsEcosystemsWithoutRankedOptions(t *testing.T) {
	shallow := portfolio.Assessment{Ecosystem: "go", Depth: portfolio.DepthShallow, ProjectCount: 2}
	empty := portfolio.Assessment{Ecosystem: "node", Depth: portfolio.DepthDeep}

	got := (&portfolioFitRule{}).Evaluate(Input{Assessments: []portfolio.Assessment{shallow, empty}, Now: testNow})

	if len(got) != 0 {
		t.Fatalf("no ranked options means no fit advice: %#v", got)
	}
}

func TestRulesNeverProposeExecutableMutations(t *testing.T) {
	// Proposed actions are descriptions for a human, not commands to run. A
	// literal shell invocation would be a mutation escaping the read-only phase.
	graph := assets.Graph{Assets: []assets.Asset{
		project("app", "node", "/roots/app", dormant),
		packageManager("npm", "node", "/roots/app"),
		install("node_modules", "node", "/roots/app/node_modules", 2<<30),
		worktree("feature", "/roots/feature", 1<<30, dormant),
		versionManager("nvm", "node", "/home/.nvm", 1<<30),
		versionManager("fnm", "node", "/home/.fnm", 1<<30),
		modelFile("llama.gguf", "/roots/a/llama.gguf", 4<<30),
		modelFile("llama.gguf", "/roots/b/llama.gguf", 4<<30),
	}}

	for _, recommendation := range NewRegistry().Run(Input{Graph: graph, Now: testNow}) {
		for _, action := range recommendation.ProposedActions {
			for _, forbidden := range []string{"rm -rf", "sudo ", "&&", "| sh"} {
				if strings.Contains(action, forbidden) {
					t.Fatalf("%s proposes an executable mutation: %q", recommendation.Family, action)
				}
			}
		}
		if recommendation.Risk == assets.RiskProhibited {
			t.Fatalf("%s produced prohibited-risk advice", recommendation.Family)
		}
	}
}

func TestSharedStoreRuleKeepsEvidenceFromEveryInstall(t *testing.T) {
	// An uncertain install early in the list must not stop later installs from
	// contributing evidence, and the confidence penalty applies only once.
	uncertain := install("node_modules", "node", "/roots/a/node_modules", 3<<30)
	uncertain.Size.Uncertain = true
	alsoUncertain := install("node_modules", "node", "/roots/b/node_modules", 3<<30)
	alsoUncertain.Size.Uncertain = true
	certain := install("node_modules", "node", "/roots/c/node_modules", 3<<30)
	certain.Evidence = []assets.Evidence{{Kind: "path_signature", Value: "/roots/c/node_modules", Confidence: 0.95}}

	graph := assets.Graph{Assets: []assets.Asset{uncertain, alsoUncertain, certain}}
	assessment := portfolio.Assessment{
		Ecosystem: "node", Depth: portfolio.DepthDeep, ProjectLocalInstallBytes: 9 << 30,
		Options: []portfolio.Option{{
			Tool: "pnpm", Rank: 1, Confidence: 0.7, Installed: true,
			ImmediateSavingsLowBytes: 1 << 30, ImmediateSavingsHighBytes: 4 << 30,
		}},
	}

	got := (&sharedStoreRule{}).Evaluate(Input{Graph: graph, Assessments: []portfolio.Assessment{assessment}, Now: testNow})
	if len(got) != 1 {
		t.Fatalf("want one recommendation, got %d", len(got))
	}
	if len(got[0].AffectedAssetIDs) != 3 {
		t.Fatalf("every install must be named: %#v", got[0].AffectedAssetIDs)
	}
	var sawLast bool
	for _, evidence := range got[0].Evidence {
		if evidence.Value == "/roots/c/node_modules" {
			sawLast = true
		}
	}
	if !sawLast {
		t.Fatalf("evidence after the first uncertain install was dropped: %#v", got[0].Evidence)
	}
	// 0.7 base minus a single 0.05 uncertainty penalty, not one per install.
	if got[0].Confidence != 0.65 {
		t.Fatalf("confidence = %v, want 0.65 (one penalty only)", got[0].Confidence)
	}
}
