package portfolio

import (
	"sort"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
)

// storeToTool maps a detected machine-wide store or cache to the package
// manager that owns it, so "already installed" is evidence rather than a guess.
var storeToTool = map[string]string{
	"pnpm_store":   "pnpm",
	"yarn_cache":   "yarn",
	"npm_cache":    "npm",
	"bun_install":  "bun",
	"cargo_home":   "cargo",
	"maven_local":  "maven",
	"gradle_cache": "gradle",
}

// projectSignal is per-project evidence used by scoring.
type projectSignal struct {
	path        string
	tools       map[string]struct{}
	hasLockfile bool
	active      bool
}

// signals is the aggregated per-ecosystem evidence behind one assessment.
type signals struct {
	ecosystem       string
	projects        []projectSignal
	inUse           map[string]int
	installed       map[string]struct{}
	versionManagers []string

	localInstallBytes int64
	localInstallCount int
	sharedStoreBytes  int64
	uncertainSizes    bool

	lockfileProjects int
	activeProjects   int
	pnpDetected      bool
}

// gather groups graph evidence by ecosystem. Machine-scoped tools are recorded
// for every ecosystem they can serve, since a store is not owned by a project.
func gather(graph assets.Graph, now time.Time, activeWithin time.Duration) map[string]*signals {
	byEco := map[string]*signals{}
	ensure := func(ecosystem string) *signals {
		if current, ok := byEco[ecosystem]; ok {
			return current
		}
		current := &signals{
			ecosystem: ecosystem,
			inUse:     map[string]int{},
			installed: map[string]struct{}{},
		}
		byEco[ecosystem] = current
		return current
	}

	projects := map[string]*projectSignal{}
	for _, asset := range graph.Assets {
		if asset.Ecosystem == "" {
			continue
		}
		current := ensure(asset.Ecosystem)
		collect(current, projects, asset, now, activeWithin)
	}

	for _, current := range byEco {
		finalize(current, projects)
	}
	return byEco
}

func collect(current *signals, projects map[string]*projectSignal, asset assets.Asset, now time.Time, activeWithin time.Duration) {
	switch asset.Kind {
	case assets.KindProject:
		key := asset.Ecosystem + "\x00" + asset.Path
		if _, ok := projects[key]; !ok {
			projects[key] = &projectSignal{path: asset.Path, tools: map[string]struct{}{}}
		}
		projects[key].active = isActive(asset.LastActivityAt, now, activeWithin)
	case assets.KindPackageManager:
		tool := toolName(asset)
		if tool == "" {
			break
		}
		if asset.Attributes["scope"] == "machine" {
			current.installed[tool] = struct{}{}
			break
		}
		current.inUse[tool]++
		key := asset.Ecosystem + "\x00" + asset.Path
		if _, ok := projects[key]; !ok {
			projects[key] = &projectSignal{path: asset.Path, tools: map[string]struct{}{}}
		}
		projects[key].tools[tool] = struct{}{}
		if hasLockfileEvidence(asset) {
			projects[key].hasLockfile = true
		}
		if asset.Attributes["yarn_mode"] == "pnp" {
			current.pnpDetected = true
		}
	case assets.KindProjectLocalInstall:
		current.localInstallCount++
		current.localInstallBytes += asset.Size.ExclusiveAllocatedBytes
		if asset.Size.Uncertain {
			current.uncertainSizes = true
		}
	case assets.KindDependencyStore, assets.KindDownloadCache:
		current.sharedStoreBytes += asset.Size.ExclusiveAllocatedBytes
		if tool, ok := storeToTool[toolName(asset)]; ok {
			current.installed[tool] = struct{}{}
		}
		if asset.Size.Uncertain {
			current.uncertainSizes = true
		}
	case assets.KindVersionManager:
		if tool := toolName(asset); tool != "" {
			current.versionManagers = append(current.versionManagers, tool)
		}
	}
}

// finalize replaces the placeholder project list with the deduplicated project
// signals for this ecosystem and derives the counts scoring reads.
func finalize(current *signals, projects map[string]*projectSignal) {
	prefix := current.ecosystem + "\x00"
	collected := make([]projectSignal, 0, len(projects))
	for key, signal := range projects {
		if len(key) <= len(prefix) || key[:len(prefix)] != prefix {
			continue
		}
		collected = append(collected, *signal)
	}
	sort.Slice(collected, func(i, j int) bool { return collected[i].path < collected[j].path })
	current.projects = collected
	current.lockfileProjects = 0
	current.activeProjects = 0
	for _, signal := range collected {
		if signal.hasLockfile {
			current.lockfileProjects++
		}
		if signal.active {
			current.activeProjects++
		}
	}
	current.versionManagers = uniqueSorted(current.versionManagers)
}

func toolName(asset assets.Asset) string {
	if tool, ok := asset.Attributes["tool"]; ok && tool != "" {
		return tool
	}
	return asset.DisplayName
}

// hasLockfileEvidence reports whether a package-manager finding was backed by a
// lockfile rather than a manifest field alone.
func hasLockfileEvidence(asset assets.Asset) bool {
	for _, evidence := range asset.Evidence {
		switch evidence.Kind {
		case "lockfile", "lockfile_or_manifest", "yarn_pnp":
			return true
		}
	}
	return false
}

func isActive(last, now time.Time, within time.Duration) bool {
	if last.IsZero() {
		return false
	}
	return now.Sub(last) <= within
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// projectCount is the number of distinct projects detected for the ecosystem.
func (s *signals) projectCount() int { return len(s.projects) }

// dominantTool is the package manager most projects already use. Ties resolve
// alphabetically so repeated scans rank identically.
func (s *signals) dominantTool() string {
	var dominant string
	var best int
	tools := make([]string, 0, len(s.inUse))
	for tool := range s.inUse {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	for _, tool := range tools {
		if s.inUse[tool] > best {
			dominant, best = tool, s.inUse[tool]
		}
	}
	return dominant
}
