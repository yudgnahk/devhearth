package attribute

import (
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/scan"
)

// projectInventory builds a project with a node_modules install and one source
// file, so exclusive attribution and activity have something to separate.
func projectInventory(t *testing.T, now time.Time) scan.Result {
	t.Helper()
	old := now.Add(-400 * 24 * time.Hour)
	return scan.Result{
		Roots: []string{"/roots"},
		Entries: []scan.Entry{
			{Path: "/roots", Kind: "directory", ModifiedAt: old},
			{Path: "/roots/app", ParentPath: "/roots", Kind: "directory", ModifiedAt: old},
			{Path: "/roots/app/main.ts", ParentPath: "/roots/app", Kind: "file", LogicalBytes: 100, AllocatedBytes: 4096, ModifiedAt: old},
			{Path: "/roots/app/.git", ParentPath: "/roots/app", Kind: "directory", ModifiedAt: now},
			{Path: "/roots/app/.git/HEAD", ParentPath: "/roots/app/.git", Kind: "file", LogicalBytes: 20, AllocatedBytes: 4096, ModifiedAt: now},
			{Path: "/roots/app/node_modules", ParentPath: "/roots/app", Kind: "directory", ModifiedAt: now},
			{Path: "/roots/app/node_modules/dep.js", ParentPath: "/roots/app/node_modules", Kind: "file", LogicalBytes: 500, AllocatedBytes: 8192, ModifiedAt: now},
		},
	}
}

func projectGraph() assets.Graph {
	return assets.Graph{Assets: []assets.Asset{
		{ID: "p1", Kind: assets.KindProject, Path: "/roots/app", Ecosystem: "node"},
		{ID: "i1", Kind: assets.KindProjectLocalInstall, Path: "/roots/app/node_modules", Ecosystem: "node"},
		{ID: "m1", Kind: assets.KindPackageManager, Path: "/roots/app", Ecosystem: "node"},
	}}
}

func findAsset(t *testing.T, graph assets.Graph, id string) assets.Asset {
	t.Helper()
	for _, asset := range graph.Assets {
		if asset.ID == id {
			return asset
		}
	}
	t.Fatalf("asset %q missing from graph", id)
	return assets.Asset{}
}

func TestApplyAttributesExclusiveBytes(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	index := scan.BuildDirectoryIndex(projectInventory(t, now))

	result := Apply(projectGraph(), index)

	project := findAsset(t, result, "p1")
	if !project.Size.Attributed {
		t.Fatal("project size should be attributed")
	}
	// 4096 (main.ts) + 4096 (.git dir file) + 8192 (node_modules) + directories.
	if project.Size.AllocatedBytes != 16384 {
		t.Fatalf("project allocated = %d, want 16384", project.Size.AllocatedBytes)
	}
	if project.Size.ExclusiveAllocatedBytes != 8192 {
		t.Fatalf("project exclusive = %d, want 8192 (node_modules excluded)", project.Size.ExclusiveAllocatedBytes)
	}
	install := findAsset(t, result, "i1")
	if install.Size.ExclusiveAllocatedBytes != 8192 {
		t.Fatalf("install exclusive = %d, want 8192", install.Size.ExclusiveAllocatedBytes)
	}
}

func TestApplySkipsAssetsThatDoNotOwnStorage(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	index := scan.BuildDirectoryIndex(projectInventory(t, now))

	manager := findAsset(t, Apply(projectGraph(), index), "m1")
	if manager.Size.Attributed || manager.Size.AllocatedBytes != 0 {
		t.Fatalf("package manager must not be charged project bytes: %#v", manager.Size)
	}
}

func TestApplyActivityIgnoresGeneratedAndGitSubtrees(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	old := now.Add(-400 * 24 * time.Hour)
	index := scan.BuildDirectoryIndex(projectInventory(t, now))

	result := Apply(projectGraph(), index)

	project := findAsset(t, result, "p1")
	if !project.LastActivityAt.Equal(old) {
		t.Fatalf("project activity = %s, want source time %s", project.LastActivityAt, old)
	}
	install := findAsset(t, result, "i1")
	if !install.LastActivityAt.Equal(now) {
		t.Fatalf("install activity = %s, want its own %s", install.LastActivityAt, now)
	}
}

func TestApplyMarksHardLinkedSubtreesUncertain(t *testing.T) {
	inventory := scan.Result{
		Roots: []string{"/roots"},
		Entries: []scan.Entry{
			{Path: "/roots", Kind: "directory"},
			{Path: "/roots/store", ParentPath: "/roots", Kind: "directory"},
			{Path: "/roots/store/blob", ParentPath: "/roots/store", Kind: "file", LogicalBytes: 10, AllocatedBytes: 0, LinkCount: 2, HardLinkAlias: true},
		},
	}
	graph := assets.Graph{Assets: []assets.Asset{
		{ID: "s1", Kind: assets.KindDependencyStore, Path: "/roots/store", Ecosystem: "node"},
	}}

	store := findAsset(t, Apply(graph, scan.BuildDirectoryIndex(inventory)), "s1")
	if !store.Size.Uncertain {
		t.Fatal("hard-linked subtree totals must be marked uncertain")
	}
	if !store.Size.Shared {
		t.Fatal("dependency stores must be marked shared")
	}
}

func TestApplyLeavesUnknownPathsUnattributed(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		{ID: "x1", Kind: assets.KindProject, Path: "/not/in/inventory"},
	}}

	asset := findAsset(t, Apply(graph, map[string][]scan.DirectoryNode{}), "x1")
	if asset.Size.Attributed {
		t.Fatalf("missing inventory node must stay unattributed: %#v", asset.Size)
	}
}

func TestApplyDoesNotMutateInput(t *testing.T) {
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	index := scan.BuildDirectoryIndex(projectInventory(t, now))
	graph := projectGraph()

	_ = Apply(graph, index)

	for _, asset := range graph.Assets {
		if asset.Size.Attributed || !asset.LastActivityAt.IsZero() {
			t.Fatalf("Apply mutated its input: %#v", asset)
		}
	}
}

func TestApplyNestsThroughIntermediateDirectories(t *testing.T) {
	// A build output two directories below the project must still be excluded
	// from the project's exclusive bytes.
	inventory := scan.Result{
		Roots: []string{"/roots"},
		Entries: []scan.Entry{
			{Path: "/roots", Kind: "directory"},
			{Path: "/roots/app", ParentPath: "/roots", Kind: "directory"},
			{Path: "/roots/app/sub", ParentPath: "/roots/app", Kind: "directory"},
			{Path: "/roots/app/sub/target", ParentPath: "/roots/app/sub", Kind: "directory"},
			{Path: "/roots/app/sub/target/out.bin", ParentPath: "/roots/app/sub/target", Kind: "file", AllocatedBytes: 12288},
			{Path: "/roots/app/src.rs", ParentPath: "/roots/app", Kind: "file", AllocatedBytes: 4096},
		},
	}
	graph := assets.Graph{Assets: []assets.Asset{
		{ID: "p1", Kind: assets.KindProject, Path: "/roots/app", Ecosystem: "rust"},
		{ID: "b1", Kind: assets.KindBuildOutput, Path: "/roots/app/sub/target", Ecosystem: "rust"},
	}}

	result := Apply(graph, scan.BuildDirectoryIndex(inventory))

	if got := findAsset(t, result, "p1").Size.ExclusiveAllocatedBytes; got != 4096 {
		t.Fatalf("project exclusive = %d, want 4096", got)
	}
}

func TestApplyAttributesGitWorktreesSoAdviceCanMeasureThem(t *testing.T) {
	// A worktree shares its path with the project rooted there. Both must report
	// the same bytes and activity, or worktree advice has nothing to measure.
	now := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	old := now.Add(-400 * 24 * time.Hour)
	inventory := scan.Result{
		Roots: []string{"/roots"},
		Entries: []scan.Entry{
			{Path: "/roots", Kind: "directory", ModifiedAt: old},
			{Path: "/roots/feature", ParentPath: "/roots", Kind: "directory", ModifiedAt: old},
			{Path: "/roots/feature/.git", ParentPath: "/roots/feature", Kind: "file", LogicalBytes: 40, AllocatedBytes: 4096, ModifiedAt: now},
			{Path: "/roots/feature/main.ts", ParentPath: "/roots/feature", Kind: "file", LogicalBytes: 80, AllocatedBytes: 4096, ModifiedAt: old},
		},
	}
	graph := assets.Graph{Assets: []assets.Asset{
		{ID: "w1", Kind: assets.KindGitWorktree, Path: "/roots/feature", DisplayName: "feature"},
	}}

	worktree := findAsset(t, Apply(graph, scan.BuildDirectoryIndex(inventory)), "w1")

	if !worktree.Size.Attributed || worktree.Size.ExclusiveAllocatedBytes != 8192 {
		t.Fatalf("worktree bytes must be attributed: %#v", worktree.Size)
	}
	if !worktree.LastActivityAt.Equal(old) {
		t.Fatalf("worktree activity = %s, want source time %s (a .git write is not source work)", worktree.LastActivityAt, old)
	}
}
