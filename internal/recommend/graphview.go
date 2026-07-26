package recommend

import (
	"sort"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
)

// graphview offers the small read-only queries rules need. Keeping them here
// stops each rule from re-walking the graph in its own way.

// ofKind returns assets of one kind ordered by path so rule output is stable.
func ofKind(graph assets.Graph, kind assets.Kind) []assets.Asset {
	out := make([]assets.Asset, 0, 8)
	for _, asset := range graph.Assets {
		if asset.Kind == kind {
			out = append(out, asset)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path == out[j].Path {
			return out[i].DisplayName < out[j].DisplayName
		}
		return out[i].Path < out[j].Path
	})
	return out
}

func toolOf(asset assets.Asset) string {
	if tool, ok := asset.Attributes["tool"]; ok && tool != "" {
		return tool
	}
	return asset.DisplayName
}

// installsByProject maps a project path to the project-local installs it owns.
// Ownership comes from detector relationships when present, and otherwise from
// path containment, which is how the install was detected in the first place.
func installsByProject(graph assets.Graph) map[string][]assets.Asset {
	byID := make(map[string]assets.Asset, len(graph.Assets))
	for _, asset := range graph.Assets {
		byID[asset.ID] = asset
	}

	out := map[string][]assets.Asset{}
	linked := map[string]struct{}{}
	for _, relationship := range graph.Relationships {
		if relationship.Kind != assets.RelProjectOwnsEnvironment {
			continue
		}
		project, okProject := byID[relationship.SourceID]
		install, okInstall := byID[relationship.TargetID]
		if !okProject || !okInstall || install.Kind != assets.KindProjectLocalInstall {
			continue
		}
		out[project.Path] = append(out[project.Path], install)
		linked[install.ID] = struct{}{}
	}

	projects := ofKind(graph, assets.KindProject)
	for _, install := range ofKind(graph, assets.KindProjectLocalInstall) {
		if _, ok := linked[install.ID]; ok {
			continue
		}
		if owner, found := nearestProject(install.Path, projects); found {
			out[owner] = append(out[owner], install)
		}
	}
	for path := range out {
		sort.Slice(out[path], func(i, j int) bool { return out[path][i].Path < out[path][j].Path })
	}
	return out
}

// nearestProject returns the deepest project path containing child.
func nearestProject(child string, projects []assets.Asset) (string, bool) {
	best := ""
	for _, project := range projects {
		if !isUnder(child, project.Path) {
			continue
		}
		if len(project.Path) > len(best) {
			best = project.Path
		}
	}
	return best, best != ""
}

func isUnder(child, parent string) bool {
	if parent == "" || child == parent {
		return false
	}
	return strings.HasPrefix(child, strings.TrimSuffix(parent, "/")+"/")
}

// reproducibleProjects returns project paths whose package-manager evidence
// includes a lockfile, which is the precondition for treating project-local
// dependencies as restorable.
func reproducibleProjects(graph assets.Graph) map[string][]assets.Asset {
	out := map[string][]assets.Asset{}
	for _, manager := range ofKind(graph, assets.KindPackageManager) {
		if manager.Attributes["scope"] == "machine" {
			continue
		}
		for _, evidence := range manager.Evidence {
			switch evidence.Kind {
			case "lockfile", "lockfile_or_manifest", "yarn_pnp":
				out[manager.Path] = append(out[manager.Path], manager)
			}
		}
	}
	return out
}

// versionManagersByEcosystem groups version managers, adding polyglot managers
// to every ecosystem they can serve. Overlapping capability is the signal the
// consolidation rule needs (for example mise alongside leftover nvm).
func versionManagersByEcosystem(graph assets.Graph) map[string][]assets.Asset {
	managers := ofKind(graph, assets.KindVersionManager)
	specific := map[string][]assets.Asset{}
	var polyglot []assets.Asset
	for _, manager := range managers {
		if manager.Ecosystem == "" || manager.Ecosystem == "polyglot" {
			polyglot = append(polyglot, manager)
			continue
		}
		specific[manager.Ecosystem] = append(specific[manager.Ecosystem], manager)
	}
	if len(polyglot) == 0 {
		return specific
	}
	for ecosystem := range specific {
		specific[ecosystem] = append(specific[ecosystem], polyglot...)
	}
	if len(specific) == 0 && len(polyglot) > 1 {
		specific["polyglot"] = polyglot
	}
	return specific
}

// assetIDs returns sorted, deduplicated asset IDs for the affected-assets list.
// Several assets can share a path, so the same id may arrive from two groups.
func assetIDs(items ...[]assets.Asset) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range items {
		for _, asset := range group {
			if _, ok := seen[asset.ID]; ok {
				continue
			}
			seen[asset.ID] = struct{}{}
			out = append(out, asset.ID)
		}
	}
	sort.Strings(out)
	return out
}

// mergeEvidence keeps evidence deterministic and free of duplicates.
func mergeEvidence(items ...[]assets.Evidence) []assets.Evidence {
	seen := map[string]struct{}{}
	var out []assets.Evidence
	for _, group := range items {
		for _, evidence := range group {
			key := evidence.Kind + "\x00" + evidence.Value
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, evidence)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			return out[i].Value < out[j].Value
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func clampConfidence(value float64) float64 {
	if value < 0.1 {
		return 0.1
	}
	if value > 0.9 {
		return 0.9
	}
	// Trim float noise so identical evidence yields byte-identical reports.
	return float64(int64(value*100+0.5)) / 100
}
