// Package python detects Python projects and local environments.
package python

import (
	"context"
	"path/filepath"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.python"
	detectorVersion = 1
	ecosystem       = "python"
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID:      detectorID,
		Version: detectorVersion,
		Triggers: []string{
			"pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock",
			"environment.yml", "environment.yaml", "setup.py", "setup.cfg",
			"poetry.lock", "uv.lock", "conda-lock.yml", ".python-version",
			".venv", "venv",
		},
		Cost:                 detect.CostMetadata,
		RequiresContentReads: false,
		Produces: []string{
			string(assets.KindProject),
			string(assets.KindPackageManager),
			string(assets.KindProjectLocalInstall),
		},
		RiskImplications: []string{"virtual environments may contain unreproducible local state"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	switch candidate.Name {
	case "pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock",
		"environment.yml", "environment.yaml", "setup.py", "setup.cfg",
		"poetry.lock", "uv.lock", "conda-lock.yml", ".python-version",
		".venv", "venv":
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

	var findings []detect.Finding
	var links []detect.Link

	switch candidate.Name {
	case ".venv", "venv":
		if !candidate.IsDir {
			return detect.Result{}, nil
		}
		// Require a sibling Python manifest so arbitrary folders named venv are
		// not promoted to full projects (high false-positive rate otherwise).
		if !detect.HasAnyParentChild(candidate,
			"pyproject.toml", "requirements.txt", "Pipfile", "Pipfile.lock",
			"environment.yml", "environment.yaml", "setup.py", "setup.cfg",
			"poetry.lock", "uv.lock", "conda-lock.yml", ".python-version",
		) {
			return detect.Result{}, nil
		}
		envPath := candidate.Path
		projectDir = candidate.Parent
		projectKey = detect.ProjectKey(ecosystem, projectDir)
		project.Key = projectKey
		project.Path = projectDir
		project.DisplayName = detect.DisplayNameFromPath(projectDir)
		install := detect.StampDetector(detect.Finding{
			Key:         detect.AssetKey(assets.KindProjectLocalInstall, envPath),
			Kind:        assets.KindProjectLocalInstall,
			DisplayName: candidate.Name,
			Path:        envPath,
			Risk:        assets.RiskMedium,
			Ecosystem:   ecosystem,
			Class:       assets.ClassProjectLocalInstall,
			Attributes:  map[string]string{"install_kind": "virtualenv"},
			Evidence:    []assets.Evidence{detect.Evidence("path_signature", envPath, 0.9)},
		}, detectorID, detectorVersion)
		findings = append(findings, project, install)
		links = append(links, detect.Link{
			SourceKey: projectKey, TargetKey: install.Key,
			Kind: assets.RelProjectOwnsEnvironment, Confidence: 0.85,
			Evidence: detect.Evidence("path_signature", envPath, 0.85),
		})
		return detect.Result{Findings: findings, Links: links}, nil
	case "poetry.lock":
		findings = append(findings, project, pm(projectDir, "poetry", candidate.Name))
		links = append(links, linkPM(projectKey, projectDir, "poetry", candidate.Name))
	case "Pipfile", "Pipfile.lock":
		findings = append(findings, project, pm(projectDir, "pipenv", candidate.Name))
		links = append(links, linkPM(projectKey, projectDir, "pipenv", candidate.Name))
	case "uv.lock":
		findings = append(findings, project, pm(projectDir, "uv", candidate.Name))
		links = append(links, linkPM(projectKey, projectDir, "uv", candidate.Name))
	case "environment.yml", "environment.yaml", "conda-lock.yml":
		findings = append(findings, project, pm(projectDir, "conda", candidate.Name))
		links = append(links, linkPM(projectKey, projectDir, "conda", candidate.Name))
	case "requirements.txt":
		findings = append(findings, project, pm(projectDir, "pip", candidate.Name))
		links = append(links, linkPM(projectKey, projectDir, "pip", candidate.Name))
	case "pyproject.toml", "setup.py", "setup.cfg", ".python-version":
		findings = append(findings, project)
	}

	return detect.Result{Findings: findings, Links: links}, nil
}

func pm(projectDir, tool, evidence string) detect.Finding {
	return detect.StampDetector(detect.Finding{
		Key:         detect.AssetKey(assets.KindPackageManager, projectDir+":"+tool),
		Kind:        assets.KindPackageManager,
		DisplayName: tool,
		Path:        projectDir,
		Risk:        assets.RiskInformational,
		Ecosystem:   ecosystem,
		Class:       assets.ClassPackageManager,
		Attributes:  map[string]string{"tool": tool, "scope": "project"},
		Evidence:    []assets.Evidence{detect.Evidence("lockfile_or_manifest", evidence, 0.9)},
	}, detectorID, detectorVersion)
}

func linkPM(projectKey, projectDir, tool, evidence string) detect.Link {
	return detect.Link{
		SourceKey:  projectKey,
		TargetKey:  detect.AssetKey(assets.KindPackageManager, projectDir+":"+tool),
		Kind:       assets.RelProjectUsesPackageManager,
		Confidence: 0.9,
		Evidence:   detect.Evidence("lockfile_or_manifest", evidence, 0.9),
	}
}
