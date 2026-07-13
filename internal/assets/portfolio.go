package assets

import "sort"

// SummarizePortfolio groups graph assets by ecosystem for tool-portfolio views.
// Fit scoring is intentionally out of scope for Phase 2.
func SummarizePortfolio(graph Graph) []PortfolioSummary {
	byEco := map[string]*PortfolioSummary{}
	ensure := func(eco string) *PortfolioSummary {
		if eco == "" {
			eco = "unknown"
		}
		if current, ok := byEco[eco]; ok {
			return current
		}
		summary := &PortfolioSummary{
			Ecosystem:       eco,
			PackageManagers: map[string]int{},
		}
		byEco[eco] = summary
		return summary
	}

	versionManagers := map[string]map[string]struct{}{}

	for _, asset := range graph.Assets {
		eco := asset.Ecosystem
		if eco == "" {
			continue
		}
		summary := ensure(eco)
		switch asset.Kind {
		case KindProject:
			summary.ProjectCount++
		case KindPackageManager:
			name := asset.DisplayName
			if tool, ok := asset.Attributes["tool"]; ok && tool != "" {
				name = tool
			}
			summary.PackageManagers[name]++
		case KindVersionManager:
			if versionManagers[eco] == nil {
				versionManagers[eco] = map[string]struct{}{}
			}
			name := asset.DisplayName
			if tool, ok := asset.Attributes["tool"]; ok && tool != "" {
				name = tool
			}
			versionManagers[eco][name] = struct{}{}
		case KindProjectLocalInstall:
			summary.ProjectLocalInstallCount++ // bytes deferred until size attribution
		case KindDependencyStore:
			summary.SharedStoreCount++
		case KindDownloadCache:
			summary.DownloadCacheCount++
		case KindBuildOutput:
			summary.BuildOutputCount++
		}
	}

	result := make([]PortfolioSummary, 0, len(byEco))
	for eco, summary := range byEco {
		if tools := versionManagers[eco]; len(tools) > 0 {
			names := make([]string, 0, len(tools))
			for name := range tools {
				names = append(names, name)
			}
			sort.Strings(names)
			summary.VersionManagers = names
		}
		var dominant string
		var dominantCount int
		for name, count := range summary.PackageManagers {
			if count > dominantCount || (count == dominantCount && (dominant == "" || name < dominant)) {
				dominant = name
				dominantCount = count
			}
		}
		summary.DominantPackageTool = dominant
		result = append(result, *summary)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Ecosystem < result[j].Ecosystem
	})
	return result
}
