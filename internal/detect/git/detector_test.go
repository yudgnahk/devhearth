package git_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/git"
)

func TestDetectRepositoryDirectory(t *testing.T) {
	dir := t.TempDir()
	gitDir := filepath.Join(dir, ".git")
	if err := os.Mkdir(gitDir, 0o755); err != nil {
		t.Fatal(err)
	}
	detector := git.New()
	candidate := detect.Candidate{Path: gitDir, Name: ".git", IsDir: true, Parent: dir}
	if !detector.Match(candidate) {
		t.Fatal("expected match")
	}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].Kind != assets.KindGitRepository {
		t.Fatalf("findings = %#v", result.Findings)
	}
	if result.Findings[0].Path != dir {
		t.Fatalf("path = %q", result.Findings[0].Path)
	}
}

func TestDetectWorktreeGitdirFile(t *testing.T) {
	dir := t.TempDir()
	mainRepo := filepath.Join(dir, "main")
	worktree := filepath.Join(dir, "worktree")
	if err := os.MkdirAll(filepath.Join(mainRepo, ".git", "worktrees", "feature"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(worktree, 0o755); err != nil {
		t.Fatal(err)
	}
	gitFile := filepath.Join(worktree, ".git")
	gitdir := filepath.Join(mainRepo, ".git", "worktrees", "feature")
	if err := os.WriteFile(gitFile, []byte("gitdir: "+gitdir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	detector := git.New()
	candidate := detect.Candidate{Path: gitFile, Name: ".git", IsDir: false, Parent: worktree}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) < 1 || result.Findings[0].Kind != assets.KindGitWorktree {
		t.Fatalf("findings = %#v", result.Findings)
	}
	if len(result.Links) != 1 || result.Links[0].Kind != assets.RelWorktreeBelongsToRepo {
		t.Fatalf("links = %#v", result.Links)
	}
}
