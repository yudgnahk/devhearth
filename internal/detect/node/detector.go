// Package node detects Node.js/TypeScript projects, package managers, and
// local installs from path signatures and small manifests.
package node

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.node"
	detectorVersion = 1
	ecosystem       = "node"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID:      detectorID,
		Version: detectorVersion,
		Triggers: []string{
			"package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
			"bun.lock", "bun.lockb", "node_modules", ".pnp.cjs", ".pnp.js",
			"pnpm-workspace.yaml",
		},
		Cost:                 detect.CostContent,
		RequiresContentReads: true,
		Produces: []string{
			string(assets.KindProject),
			string(assets.KindPackageManager),
			string(assets.KindProjectLocalInstall),
		},
		RiskImplications: []string{"project-local node_modules are usually reproducible from lockfiles"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch candidate.Name {
	case "package.json", "package-lock.json", "yarn.lock", "pnpm-lock.yaml",
		"bun.lock", "bun.lockb", "node_modules", ".pnp.cjs", ".pnp.js",
		"pnpm-workspace.yaml":
		return true
	default:
		return false
	}
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	projectDir := candidate.Parent
	if projectDir == "" {
		projectDir = filepath.Dir(candidate.Path)
	}
	if candidate.Name == "node_modules" && candidate.IsDir {
		projectDir = candidate.Parent
	}

	var findings []detect.Finding
	var links []detect.Link

	projectKey := detect.ProjectKey(ecosystem, projectDir)
	project := detect.StampDetector(detect.Finding{
		Key:         projectKey,
		Kind:        assets.KindProject,
		DisplayName: detect.DisplayNameFromPath(projectDir),
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassProject,
		Attributes:  map[string]string{"ecosystem": ecosystem},
		Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.9)},
	}, detectorID, detectorVersion)

	switch candidate.Name {
	case "package.json":
		project.Evidence = append(project.Evidence, detect.Evidence("manifest", "package.json", 0.95))
		if tool, attrEvidence := readPackageManager(candidate.Path); tool != "" {
			project.Attributes["package_manager"] = tool
			project.Evidence = append(project.Evidence, attrEvidence)
			findings = append(findings, packageManagerFinding(projectDir, tool, attrEvidence))
			links = append(links, detect.Link{
				SourceKey:  projectKey,
				TargetKey:  detect.AssetKey(assets.KindPackageManager, projectDir+":"+tool),
				Kind:       assets.RelProjectUsesPackageManager,
				Confidence: attrEvidence.Confidence,
				Evidence:   attrEvidence,
			})
		}
		findings = append(findings, project)
	case "package-lock.json":
		findings = append(findings, project, packageManagerFinding(projectDir, "npm", detect.Evidence("lockfile", "package-lock.json", 0.95)))
		links = append(links, pmLink(projectKey, projectDir, "npm", "package-lock.json"))
	case "yarn.lock":
		findings = append(findings, project, packageManagerFinding(projectDir, "yarn", detect.Evidence("lockfile", "yarn.lock", 0.95)))
		links = append(links, pmLink(projectKey, projectDir, "yarn", "yarn.lock"))
	case "pnpm-lock.yaml", "pnpm-workspace.yaml":
		findings = append(findings, project, packageManagerFinding(projectDir, "pnpm", detect.Evidence("lockfile", candidate.Name, 0.95)))
		links = append(links, pmLink(projectKey, projectDir, "pnpm", candidate.Name))
	case "bun.lock", "bun.lockb":
		findings = append(findings, project, packageManagerFinding(projectDir, "bun", detect.Evidence("lockfile", candidate.Name, 0.95)))
		links = append(links, pmLink(projectKey, projectDir, "bun", candidate.Name))
	case ".pnp.cjs", ".pnp.js":
		ev := detect.Evidence("yarn_pnp", candidate.Name, 0.9)
		pm := packageManagerFinding(projectDir, "yarn", ev)
		pm.Attributes["yarn_mode"] = "pnp"
		findings = append(findings, project, pm)
		links = append(links, pmLink(projectKey, projectDir, "yarn", candidate.Name))
	case "node_modules":
		if !candidate.IsDir {
			return detect.Result{}, nil
		}
		install := detect.StampDetector(detect.Finding{
			Key:         detect.AssetKey(assets.KindProjectLocalInstall, candidate.Path),
			Kind:        assets.KindProjectLocalInstall,
			DisplayName: "node_modules",
			Path:        candidate.Path,
			Risk:        assets.RiskLow,
			Ecosystem:   ecosystem,
			Class:       assets.ClassProjectLocalInstall,
			Attributes:  map[string]string{"install_kind": "node_modules"},
			Evidence:    []assets.Evidence{detect.Evidence("path_signature", candidate.Path, 0.95)},
		}, detectorID, detectorVersion)
		findings = append(findings, project, install)
		links = append(links, detect.Link{
			SourceKey:  projectKey,
			TargetKey:  install.Key,
			Kind:       assets.RelProjectOwnsEnvironment,
			Confidence: 0.9,
			Evidence:   detect.Evidence("path_signature", candidate.Path, 0.9),
		})
	}

	return detect.Result{Findings: findings, Links: links}, nil
}

func packageManagerFinding(projectDir, tool string, evidence assets.Evidence) detect.Finding {
	pathKey := projectDir + ":" + tool
	return detect.StampDetector(detect.Finding{
		Key:         detect.AssetKey(assets.KindPackageManager, pathKey),
		Kind:        assets.KindPackageManager,
		DisplayName: tool,
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassPackageManager,
		Attributes:  map[string]string{"tool": tool, "scope": "project"},
		Evidence:    []assets.Evidence{evidence},
	}, detectorID, detectorVersion)
}

func pmLink(projectKey, projectDir, tool, evidenceValue string) detect.Link {
	return detect.Link{
		SourceKey:  projectKey,
		TargetKey:  detect.AssetKey(assets.KindPackageManager, projectDir+":"+tool),
		Kind:       assets.RelProjectUsesPackageManager,
		Confidence: 0.95,
		Evidence:   detect.Evidence("lockfile", evidenceValue, 0.95),
	}
}

type packageJSON struct {
	PackageManager string `json:"packageManager"`
	Name           string `json:"name"`
}

func readPackageManager(path string) (string, assets.Evidence) {
	data, err := detect.ReadFileLimited(path, 32*1024)
	if err != nil {
		return "", assets.Evidence{}
	}
	var manifest packageJSON
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", assets.Evidence{}
	}
	if manifest.PackageManager == "" {
		return "", assets.Evidence{}
	}
	tool := strings.ToLower(manifest.PackageManager)
	if idx := strings.Index(tool, "@"); idx > 0 {
		tool = tool[:idx]
	}
	switch tool {
	case "npm", "yarn", "pnpm", "bun":
		return tool, detect.Evidence("package_manager_field", manifest.PackageManager, 0.9)
	default:
		return tool, detect.Evidence("package_manager_field", manifest.PackageManager, 0.7)
	}
}
