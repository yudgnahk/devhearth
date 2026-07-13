package assets_test

import (
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
)

func TestSummarizePortfolio(t *testing.T) {
	graph := assets.Graph{Assets: []assets.Asset{
		{Kind: assets.KindProject, Ecosystem: "node"},
		{Kind: assets.KindProject, Ecosystem: "node"},
		{Kind: assets.KindPackageManager, Ecosystem: "node", DisplayName: "pnpm", Attributes: map[string]string{"tool": "pnpm"}},
		{Kind: assets.KindPackageManager, Ecosystem: "node", DisplayName: "npm", Attributes: map[string]string{"tool": "npm"}},
		{Kind: assets.KindPackageManager, Ecosystem: "node", DisplayName: "pnpm", Attributes: map[string]string{"tool": "pnpm"}},
		{Kind: assets.KindVersionManager, Ecosystem: "node", Attributes: map[string]string{"tool": "fnm"}},
		{Kind: assets.KindProjectLocalInstall, Ecosystem: "node"},
		{Kind: assets.KindProject, Ecosystem: "python"},
	}}
	summaries := assets.SummarizePortfolio(graph)
	if len(summaries) != 2 {
		t.Fatalf("summaries = %d", len(summaries))
	}
	var node assets.PortfolioSummary
	for _, summary := range summaries {
		if summary.Ecosystem == "node" {
			node = summary
		}
	}
	if node.ProjectCount != 2 || node.DominantPackageTool != "pnpm" {
		t.Fatalf("node portfolio = %#v", node)
	}
	if len(node.VersionManagers) != 1 || node.VersionManagers[0] != "fnm" {
		t.Fatalf("version managers = %#v", node.VersionManagers)
	}
	if node.ProjectLocalInstallCount != 1 {
		t.Fatalf("project local install count = %d", node.ProjectLocalInstallCount)
	}
}
