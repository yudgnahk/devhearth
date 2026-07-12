package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/scan"
)

func TestSavePersistsInventory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	result := scan.Result{Roots: []string{"/fixtures"}, StartedAt: now, CompletedAt: now, Entries: []scan.Entry{{Path: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now}, {Path: "/fixtures/data", ParentPath: "/fixtures", Kind: "file", LogicalBytes: 3, AllocatedBytes: 4096, DeviceID: 1, Inode: 2, LinkCount: 1, ModifiedAt: now}}}
	id, err := database.Save(context.Background(), result, "complete")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("entries = %d, want 2", count)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if database, err = Open(path); err != nil {
		t.Fatalf("reopen migrated database: %v", err)
	}
}
