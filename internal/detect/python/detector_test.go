package python_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/python"
)

func TestVenvWithoutManifestIsIgnored(t *testing.T) {
	detector := python.New()
	candidate := detect.Candidate{
		Path:           filepath.Join("/tmp", "misc", "venv"),
		Name:           "venv",
		IsDir:          true,
		Parent:         filepath.Join("/tmp", "misc"),
		ParentChildren: []string{"venv", "notes.txt"},
	}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("expected no findings without manifest, got %#v", result.Findings)
	}
}

func TestVenvWithManifestProducesInstall(t *testing.T) {
	detector := python.New()
	projectDir := filepath.Join("/tmp", "pyapp")
	envPath := filepath.Join(projectDir, ".venv")
	candidate := detect.Candidate{
		Path:           envPath,
		Name:           ".venv",
		IsDir:          true,
		Parent:         projectDir,
		ParentChildren: []string{".venv", "pyproject.toml"},
	}
	result, err := detector.Detect(context.Background(), candidate)
	if err != nil {
		t.Fatal(err)
	}
	var foundInstall bool
	for _, finding := range result.Findings {
		if finding.Kind == assets.KindProjectLocalInstall && finding.Path == envPath {
			foundInstall = true
		}
	}
	if !foundInstall {
		t.Fatalf("expected virtualenv install, got %#v", result.Findings)
	}
}
