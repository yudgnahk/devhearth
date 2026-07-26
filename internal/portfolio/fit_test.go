package portfolio

import (
	"strings"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
)

var testNow = time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)

// nodeProject builds a project plus its package manager and local install.
func nodeProject(path, tool string, installBytes int64, lastActivity time.Time) []assets.Asset {
	return []assets.Asset{
		{
			ID: "project:" + path, Kind: assets.KindProject, DisplayName: "app",
			Path: path, Ecosystem: "node", Class: assets.ClassProject,
			LastActivityAt: lastActivity,
		},
		{
			ID: "pm:" + path + ":" + tool, Kind: assets.KindPackageManager, DisplayName: tool,
			Path: path, Ecosystem: "node", Class: assets.ClassPackageManager,
			Attributes: map[string]string{"tool": tool, "scope": "project"},
			Evidence:   []assets.Evidence{{Kind: "lockfile", Value: tool + ".lock", Confidence: 0.95}},
		},
		{
			ID: "install:" + path, Kind: assets.KindProjectLocalInstall, DisplayName: "node_modules",
			Path: path + "/node_modules", Ecosystem: "node", Class: assets.ClassProjectLocalInstall,
			Size: assets.Size{Attributed: true, AllocatedBytes: installBytes, ExclusiveAllocatedBytes: installBytes},
		},
	}
}

func findAssessment(t *testing.T, list []Assessment, ecosystem string) Assessment {
	t.Helper()
	for _, item := range list {
		if item.Ecosystem == ecosystem {
			return item
		}
	}
	t.Fatalf("no assessment for %q in %#v", ecosystem, list)
	return Assessment{}
}

func findOption(t *testing.T, assessment Assessment, tool string) Option {
	t.Helper()
	for _, option := range assessment.Options {
		if option.Tool == tool {
			return option
		}
	}
	t.Fatalf("no option for %q in %#v", tool, assessment.Options)
	return Option{}
}

func TestAssessAlwaysIncludesStayPutBaseline(t *testing.T) {
	var graph assets.Graph
	graph.Assets = append(graph.Assets, nodeProject("/r/a", "npm", 1<<30, testNow)...)
	graph.Assets = append(graph.Assets, nodeProject("/r/b", "npm", 1<<30, testNow)...)

	node := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")

	if node.Baseline != "npm" {
		t.Fatalf("baseline = %q, want npm", node.Baseline)
	}
	stayPut := findOption(t, node, "npm")
	if !stayPut.StayPut {
		t.Fatal("the dominant tool must be marked as the stay-put option")
	}
	if stayPut.ImmediateSavingsLowBytes != 0 || stayPut.ImmediateSavingsHighBytes != 0 {
		t.Fatalf("stay-put must not claim savings: %#v", stayPut)
	}
}

func TestAssessRanksOptionsDeterministically(t *testing.T) {
	var graph assets.Graph
	for _, path := range []string{"/r/a", "/r/b", "/r/c"} {
		graph.Assets = append(graph.Assets, nodeProject(path, "npm", 2<<30, testNow)...)
	}

	first := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")
	second := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")

	if len(first.Options) != len(second.Options) {
		t.Fatalf("option count differs between runs: %d vs %d", len(first.Options), len(second.Options))
	}
	for index := range first.Options {
		if first.Options[index].Tool != second.Options[index].Tool || first.Options[index].Score != second.Options[index].Score {
			t.Fatalf("ranking is not deterministic at %d: %#v vs %#v", index, first.Options[index], second.Options[index])
		}
		if first.Options[index].Rank != index+1 {
			t.Fatalf("rank = %d at index %d", first.Options[index].Rank, index)
		}
	}
}

func TestAssessOffersSharedStoreSavingsRangeForDuplicatedInstalls(t *testing.T) {
	var graph assets.Graph
	for _, path := range []string{"/r/a", "/r/b", "/r/c", "/r/d"} {
		graph.Assets = append(graph.Assets, nodeProject(path, "npm", 1<<30, testNow)...)
	}
	graph.Assets = append(graph.Assets, assets.Asset{
		ID: "store", Kind: assets.KindDependencyStore, DisplayName: "pnpm_store",
		Path: "/home/.pnpm-store", Ecosystem: "node", Class: assets.ClassDependencyStore,
		Attributes: map[string]string{"tool": "pnpm_store", "scope": "machine"},
	})

	node := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")
	pnpm := findOption(t, node, "pnpm")

	if !pnpm.Installed {
		t.Fatal("a detected pnpm store is evidence that pnpm is installed")
	}
	if pnpm.ImmediateSavingsLowBytes <= 0 || pnpm.ImmediateSavingsHighBytes <= pnpm.ImmediateSavingsLowBytes {
		t.Fatalf("expected a savings range, got %d..%d", pnpm.ImmediateSavingsLowBytes, pnpm.ImmediateSavingsHighBytes)
	}
	if pnpm.ImmediateSavingsHighBytes >= 4<<30 {
		t.Fatalf("savings must stay below total project-local bytes, got %d", pnpm.ImmediateSavingsHighBytes)
	}
	if !pnpm.SavingsUncertain {
		t.Fatal("savings without content hashing must be marked uncertain")
	}
	npm := findOption(t, node, "npm")
	if npm.ImmediateSavingsHighBytes != 0 {
		t.Fatalf("npm installs a per-project copy and cannot claim shared-store savings: %#v", npm)
	}
}

func TestAssessRecordsPnPBlockerForNonYarnOptions(t *testing.T) {
	var graph assets.Graph
	graph.Assets = append(graph.Assets, nodeProject("/r/a", "yarn", 1<<30, testNow)...)
	graph.Assets = append(graph.Assets, nodeProject("/r/b", "yarn", 1<<30, testNow)...)
	for index, asset := range graph.Assets {
		if asset.Kind == assets.KindPackageManager {
			graph.Assets[index].Attributes["yarn_mode"] = "pnp"
		}
	}

	node := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")
	pnpm := findOption(t, node, "pnpm")

	var found bool
	for _, blocker := range pnpm.Blockers {
		if blocker != "" && strings.Contains(blocker, "Plug'n'Play") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a PnP blocker on non-yarn options, got %#v", pnpm.Blockers)
	}
}

func TestAssessMarksNonDeepEcosystemsShallow(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		{ID: "p", Kind: assets.KindProject, Path: "/r/go-app", Ecosystem: "go", DisplayName: "go-app"},
	}}

	goEco := findAssessment(t, Assess(graph, Input{Now: testNow}), "go")

	if goEco.Depth != DepthShallow {
		t.Fatalf("depth = %q, want shallow", goEco.Depth)
	}
	if len(goEco.Options) != 0 {
		t.Fatalf("shallow assessments must not rank options: %#v", goEco.Options)
	}
	if len(goEco.Notes) == 0 {
		t.Fatal("shallow assessments must state their scope limit")
	}
}

func TestAssessPolicyPreferenceReweightsRanking(t *testing.T) {
	var graph assets.Graph
	for _, path := range []string{"/r/a", "/r/b", "/r/c"} {
		graph.Assets = append(graph.Assets, nodeProject(path, "npm", 4<<30, testNow)...)
	}

	neutral := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")
	preferred := findAssessment(t, Assess(graph, Input{Now: testNow, Preferred: map[string]string{"node": "pnpm"}}), "node")

	if findOption(t, preferred, "pnpm").Score <= findOption(t, neutral, "pnpm").Score {
		t.Fatal("a policy preference must raise the preferred tool's score")
	}
}

func TestAssessConfidenceStaysConservative(t *testing.T) {
	var graph assets.Graph
	graph.Assets = append(graph.Assets, nodeProject("/r/a", "npm", 1<<30, testNow)...)

	node := findAssessment(t, Assess(graph, Input{Now: testNow}), "node")
	for _, option := range node.Options {
		if option.Confidence > 0.9 || option.Confidence < 0.1 {
			t.Fatalf("confidence out of range for %s: %v", option.Tool, option.Confidence)
		}
	}
}

func TestAssessTreatsDormantProjectsAsFavouringStayPut(t *testing.T) {
	dormant := testNow.Add(-365 * 24 * time.Hour)
	var hot, cold assets.Graph
	for _, path := range []string{"/r/a", "/r/b", "/r/c"} {
		hot.Assets = append(hot.Assets, nodeProject(path, "npm", 1<<30, testNow)...)
		cold.Assets = append(cold.Assets, nodeProject(path, "npm", 1<<30, dormant)...)
	}

	hotNode := findAssessment(t, Assess(hot, Input{Now: testNow}), "node")
	coldNode := findAssessment(t, Assess(cold, Input{Now: testNow}), "node")

	if findOption(t, coldNode, "npm").Score <= findOption(t, hotNode, "npm").Score {
		t.Fatal("dormant portfolios should favour the stay-put option more than active ones")
	}
}
