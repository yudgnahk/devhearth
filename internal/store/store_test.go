package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/scan"
)

func TestSavePersistsInventoryAndAssets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	result := scan.Result{
		Roots: []string{"/fixtures"}, StartedAt: now, CompletedAt: now,
		Entries: []scan.Entry{
			{Path: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now},
			{Path: "/fixtures/data", ParentPath: "/fixtures", Kind: "file", LogicalBytes: 3, AllocatedBytes: 4096, DeviceID: 1, Inode: 2, LinkCount: 1, ModifiedAt: now},
			{Path: "/fixtures/app", ParentPath: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 3, LinkCount: 1, ModifiedAt: now},
		},
	}
	graph := assets.Graph{
		Assets: []assets.Asset{{
			ID: "asset-1", Kind: assets.KindProject, DisplayName: "app", Path: "/fixtures/app",
			Risk: assets.RiskInformational, Ecosystem: "node", Class: assets.ClassProject,
			DetectorID: "detect.node", DetectorVersion: 1,
			Evidence: []assets.Evidence{{Kind: "manifest", Value: "package.json", Confidence: 0.9}},
		}},
		Relationships: nil,
	}
	id, err := database.Save(context.Background(), result, graph, "complete")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("entries = %d, want 3", count)
	}
	listed, err := database.ListAssets(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if len(listed) != 1 || listed[0].Ecosystem != "node" {
		t.Fatalf("assets = %#v", listed)
	}
	evidence, err := database.ListEvidence(context.Background(), id, "asset-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(evidence) != 1 {
		t.Fatalf("evidence = %#v", evidence)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if database, err = Open(path); err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
	defer database.Close()
}

func TestMigrationAppliesAssetGraphColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var version int
	if err := database.db.QueryRow(`SELECT MAX(version) FROM schema_migrations`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version < 3 {
		t.Fatalf("schema version = %d, want >= 3", version)
	}
}
