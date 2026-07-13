package rust_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/rust"
)

func TestTargetWithoutCargoTomlIsIgnored(t *testing.T) {
	detector := rust.New()
	candidate := detect.Candidate{
		Path:           filepath.Join("/tmp", "other", "target"),
		Name:           "target",
		IsDir:          true,
		Parent:         filepath.Join("/tmp", "other"),
		ParentChildren: []string{"target", "README.md"},
	}
	if !detector.Match(candidate) {
		t.Fatal("expected match on directory name target")
	}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected no findings without Cargo.toml, got %#v", result.Findings)
	}
}

func TestTargetWithCargoTomlIsBuildOutput(t *testing.T) {
	detector := rust.New()
	projectDir := filepath.Join("/tmp", "crate")
	targetPath := filepath.Join(projectDir, "target")
	candidate := detect.Candidate{
		Path:           targetPath,
		Name:           "target",
		IsDir:          true,
		Parent:         projectDir,
		ParentChildren: []string{"Cargo.toml", "src", "target"},
	}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 2 {
		t.Fatalf("findings = %#v", result.Findings)
	}
	var foundBuild bool
	for _, finding := range result.Findings {
		if finding.Kind == assets.KindBuildOutput && finding.Path == targetPath {
			foundBuild = true
		}
	}
	if !foundBuild {
		t.Fatalf("expected cargo target build output, got %#v", result.Findings)
	}
}
