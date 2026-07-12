// Package git detects Git repositories and linked worktrees from metadata.
// It never runs git subprocesses in this phase and never mutates the tree.
package git

import (
	"context"
	"path/filepath"
	"strings"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
)

const (
	detectorID      = "detect.git"
	detectorVersion = 1
)

type Detector struct{}

func New() *Detector { return &Detector{} }

func (d *Detector) Descriptor() detect.Descriptor {
	return detect.Descriptor{
		ID:                   detectorID,
		Version:              detectorVersion,
		Triggers:             []string{".git"},
		Cost:                 detect.CostContent,
		RequiresContentReads: true,
		Produces:             []string{string(assets.KindGitRepository), string(assets.KindGitWorktree)},
		RiskImplications:     []string{"unknown remote sync state without fetch"},
	}
}

func (d *Detector) Match(candidate detect.Candidate) bool {
	return candidate.Name == ".git"
}

func (d *Detector) Detect(ctx context.Context, candidate detect.Candidate) (detect.Result, error) {
	if err := ctx.Err(); err != nil {
		return detect.Result{}, err
	}
	worktreeRoot := candidate.Parent
	if worktreeRoot == "" {
		worktreeRoot = filepath.Dir(candidate.Path)
	}

	if candidate.IsDir {
		finding := detect.StampDetector(detect.Finding{
			Key:         detect.AssetKey(assets.KindGitRepository, worktreeRoot),
			Kind:        assets.KindGitRepository,
			DisplayName: detect.DisplayNameFromPath(worktreeRoot),
			Path:        worktreeRoot,
			Risk:        assets.RiskInformational,
			Class:       assets.ClassGit,
			Attributes: map[string]string{
				"git_kind": "repository",
			},
			Evidence: []assets.Evidence{
				detect.Evidence("path_signature", candidate.Path, 0.95),
			},
		}, detectorID, detectorVersion)
		return detect.Result{Findings: []detect.Finding{finding}}, nil
	}

	// `.git` file indicates a linked worktree or gitdir pointer.
	finding := detect.StampDetector(detect.Finding{
		Key:         detect.AssetKey(assets.KindGitWorktree, worktreeRoot),
		Kind:        assets.KindGitWorktree,
		DisplayName: detect.DisplayNameFromPath(worktreeRoot),
		Path:        worktreeRoot,
		Risk:        assets.RiskInformational,
		Class:       assets.ClassGit,
		Attributes: map[string]string{
			"git_kind": "worktree",
		},
		Evidence: []assets.Evidence{
			detect.Evidence("path_signature", candidate.Path, 0.9),
		},
	}, detectorID, detectorVersion)

	result := detect.Result{Findings: []detect.Finding{finding}}
	data, err := detect.ReadFileLimited(candidate.Path, 4096)
	if err != nil {
		return result, nil
	}
	line := strings.TrimSpace(string(data))
	if gitDir, ok := strings.CutPrefix(line, "gitdir:"); ok {
		gitDir = strings.TrimSpace(gitDir)
		finding.Attributes["gitdir"] = gitDir
		finding.Evidence = append(finding.Evidence, detect.Evidence("gitdir_pointer", gitDir, 0.85))
		result.Findings[0] = finding

		// Common layout: <repo>/.git/worktrees/<name>
		if idx := strings.Index(gitDir, string(filepath.Separator)+".git"+string(filepath.Separator)+"worktrees"+string(filepath.Separator)); idx > 0 {
			mainRepo := gitDir[:idx]
			mainKey := detect.AssetKey(assets.KindGitRepository, mainRepo)
			mainFinding := detect.StampDetector(detect.Finding{
				Key:         mainKey,
				Kind:        assets.KindGitRepository,
				DisplayName: detect.DisplayNameFromPath(mainRepo),
				Path:        mainRepo,
				Risk:        assets.RiskInformational,
				Class:       assets.ClassGit,
				Attributes:  map[string]string{"git_kind": "repository"},
				Evidence:    []assets.Evidence{detect.Evidence("worktree_gitdir", gitDir, 0.8)},
			}, detectorID, detectorVersion)
			result.Findings = append(result.Findings, mainFinding)
			result.Links = append(result.Links, detect.Link{
				SourceKey:  finding.Key,
				TargetKey:  mainKey,
				Kind:       assets.RelWorktreeBelongsToRepo,
				Confidence: 0.8,
				Evidence:   detect.Evidence("worktree_gitdir", gitDir, 0.8),
			})
		}
	}
	return result, nil
}
