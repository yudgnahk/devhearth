package node_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/detect"
	"github.com/yudgnahk/devhearth/internal/detect/node"
)

func TestDetectPNPMFromPackageManagerField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "package.json")
	if err := os.WriteFile(path, []byte(`{"name":"demo","packageManager":"pnpm@10.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	detector := node.New()
	result, err := detector.Detect(context.Background(), detect.Candidate{
		Path: path, Name: "package.json", Parent: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundPM := false
	for _, finding := range result.Findings {
		if finding.Kind == assets.KindPackageManager && finding.Attributes["tool"] == "pnpm" {
			foundPM = true
		}
	}
	if !foundPM {
		t.Fatalf("expected pnpm finding, got %#v", result.Findings)
	}
}

func TestDetectNodeModulesInstall(t *testing.T) {
	dir := t.TempDir()
	modules := filepath.Join(dir, "node_modules")
	if err := os.Mkdir(modules, 0o755); err != nil {
		t.Fatal(err)
	}
	detector := node.New()
	result, err := detector.Detect(context.Background(), detect.Candidate{
		Path: modules, Name: "node_modules", IsDir: true, Parent: dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	foundInstall := false
	for _, finding := range result.Findings {
		if finding.Kind == assets.KindProjectLocalInstall {
			foundInstall = true
		}
	}
	if !foundInstall {
		t.Fatalf("expected project_local_install, got %#v", result.Findings)
	}
}
