package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/yudgnahk/devhearth/internal/advisor"
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
	var lastWritten, lastTotal int64
	id, err := database.SaveWithOptions(context.Background(), result, advisor.Result{Graph: graph}, "complete", SaveOptions{
		Progress: func(written, total int64) {
			lastWritten, lastTotal = written, total
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if lastTotal == 0 || lastWritten != lastTotal {
		t.Fatalf("progress written=%d total=%d", lastWritten, lastTotal)
	}
	var count int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 3 {
		t.Fatalf("entries = %d, want 3", count)
	}
	var parented int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ? AND parent_id IS NOT NULL`, id).Scan(&parented); err != nil {
		t.Fatal(err)
	}
	if parented != 2 {
		t.Fatalf("parented entries = %d, want 2", parented)
	}
	// Explicit ids should be stable parent-before-child.
	var childParent, rootID int64
	if err := database.db.QueryRow(`SELECT id FROM filesystem_entries WHERE scan_id = ? AND path = ?`, id, "/fixtures").Scan(&rootID); err != nil {
		t.Fatal(err)
	}
	if err := database.db.QueryRow(`SELECT parent_id FROM filesystem_entries WHERE scan_id = ? AND path = ?`, id, "/fixtures/data").Scan(&childParent); err != nil {
		t.Fatal(err)
	}
	if childParent != rootID {
		t.Fatalf("parent_id = %d, want root id %d", childParent, rootID)
	}
	children, err := database.ListDirectoryChildren(context.Background(), id, "/fixtures")
	if err != nil {
		t.Fatal(err)
	}
	if len(children) != 2 {
		t.Fatalf("directory children = %#v", children)
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

func TestSaveSkipsBulkTreeInteriors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	now := time.Now().UTC()
	result := scan.Result{
		Roots: []string{"/proj"}, StartedAt: now, CompletedAt: now,
		Entries: []scan.Entry{
			{Path: "/proj", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now},
			{Path: "/proj/node_modules", ParentPath: "/proj", Kind: "directory", DeviceID: 1, Inode: 2, LinkCount: 1, ModifiedAt: now},
			{Path: "/proj/node_modules/lodash", ParentPath: "/proj/node_modules", Kind: "directory", DeviceID: 1, Inode: 3, LinkCount: 1, ModifiedAt: now},
			{Path: "/proj/node_modules/lodash/index.js", ParentPath: "/proj/node_modules/lodash", Kind: "file", LogicalBytes: 10, AllocatedBytes: 4096, DeviceID: 1, Inode: 4, LinkCount: 1, ModifiedAt: now},
			{Path: "/proj/src", ParentPath: "/proj", Kind: "directory", DeviceID: 1, Inode: 5, LinkCount: 1, ModifiedAt: now},
			{Path: "/proj/src/main.go", ParentPath: "/proj/src", Kind: "file", LogicalBytes: 20, AllocatedBytes: 4096, DeviceID: 1, Inode: 6, LinkCount: 1, ModifiedAt: now},
		},
	}
	id, err := database.Save(context.Background(), result, advisor.Result{}, "complete")
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ?`, id).Scan(&count); err != nil {
		t.Fatal(err)
	}
	// node_modules itself kept; lodash interior dropped; src tree kept.
	if count != 4 {
		t.Fatalf("entries = %d, want 4 (skipped bulk interiors)", count)
	}
	var interior int
	if err := database.db.QueryRow(`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ? AND path LIKE ?`, id, "%/node_modules/%").Scan(&interior); err != nil {
		t.Fatal(err)
	}
	if interior != 0 {
		t.Fatalf("bulk interiors persisted = %d, want 0", interior)
	}
}

// "build" and "dist" are as often hand-written source directories as generated
// output, so pruning them needs sibling evidence. Dropping a source tree from
// durable inventory is silent: it still appears in the live drill-down.
func TestSavePrunesGeneratedOutputOnlyWithSiblingEvidence(t *testing.T) {
	now := time.Now().UTC()
	dir := func(path, parent string) scan.Entry {
		return scan.Entry{Path: path, ParentPath: parent, Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now}
	}
	file := func(path, parent string) scan.Entry {
		return scan.Entry{Path: path, ParentPath: parent, Kind: "file", LogicalBytes: 10, AllocatedBytes: 4096, DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now}
	}

	tests := []struct {
		name     string
		entries  []scan.Entry
		interior string
		persist  bool
	}{
		{
			name: "build beside a manifest is generated output",
			entries: []scan.Entry{
				dir("/proj", ""), file("/proj/CMakeLists.txt", "/proj"),
				dir("/proj/build", "/proj"), file("/proj/build/app.o", "/proj/build"),
			},
			interior: "/proj/build/app.o",
			persist:  false,
		},
		{
			name: "build with no manifest beside it is source",
			entries: []scan.Entry{
				dir("/proj", ""), file("/proj/README.md", "/proj"),
				dir("/proj/build", "/proj"), file("/proj/build/release.sh", "/proj/build"),
			},
			interior: "/proj/build/release.sh",
			persist:  true,
		},
		{
			name: "dist beside a manifest is generated output",
			entries: []scan.Entry{
				dir("/app", ""), file("/app/package.json", "/app"),
				dir("/app/dist", "/app"), file("/app/dist/bundle.js", "/app/dist"),
			},
			interior: "/app/dist/bundle.js",
			persist:  false,
		},
		{
			name: "dist with no manifest beside it is source",
			entries: []scan.Entry{
				dir("/docs", ""), dir("/docs/dist", "/docs"),
				file("/docs/dist/logo.svg", "/docs/dist"),
			},
			interior: "/docs/dist/logo.svg",
			persist:  true,
		},
		{
			name: "release only counts as bulk beneath target",
			entries: []scan.Entry{
				dir("/notes", ""), dir("/notes/release", "/notes"),
				file("/notes/release/changelog.md", "/notes/release"),
			},
			interior: "/notes/release/changelog.md",
			persist:  true,
		},
		{
			name: "cargo target profile stays bulk",
			entries: []scan.Entry{
				dir("/rs", ""), dir("/rs/target", "/rs"),
				dir("/rs/target/release", "/rs/target"),
				file("/rs/target/release/app", "/rs/target/release"),
			},
			interior: "/rs/target/release/app",
			persist:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			database, err := Open(filepath.Join(t.TempDir(), "inventory.sqlite"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			result := scan.Result{
				Roots: []string{test.entries[0].Path}, StartedAt: now, CompletedAt: now,
				Entries: test.entries,
			}
			id, err := database.Save(context.Background(), result, advisor.Result{}, "complete")
			if err != nil {
				t.Fatal(err)
			}
			var count int
			if err := database.db.QueryRow(
				`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ? AND path = ?`,
				id, test.interior,
			).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if persisted := count == 1; persisted != test.persist {
				t.Fatalf("%s persisted = %v, want %v", test.interior, persisted, test.persist)
			}
		})
	}
}

// A bulk save must not trade crash safety for speed, and must leave the
// connection's integrity settings exactly as it found them.
func TestSaveKeepsDurableConnectionSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "inventory.sqlite")
	database, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()

	pragma := func(name string) string {
		var value string
		if err := database.db.QueryRow(`PRAGMA ` + name).Scan(&value); err != nil {
			t.Fatalf("read pragma %s: %v", name, err)
		}
		return value
	}

	// journal_mode=wal with synchronous=1 (NORMAL) is crash-safe; 0 (OFF) is not.
	if mode := pragma("journal_mode"); mode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", mode)
	}
	if sync := pragma("synchronous"); sync != "1" {
		t.Fatalf("synchronous = %q, want 1 (NORMAL)", sync)
	}

	now := time.Now().UTC()
	result := scan.Result{
		Roots: []string{"/fixtures"}, StartedAt: now, CompletedAt: now,
		Entries: []scan.Entry{
			{Path: "/fixtures", Kind: "directory", DeviceID: 1, Inode: 1, LinkCount: 1, ModifiedAt: now},
		},
	}
	if _, err := database.Save(context.Background(), result, advisor.Result{}, "complete"); err != nil {
		t.Fatal(err)
	}

	if sync := pragma("synchronous"); sync != "1" {
		t.Fatalf("synchronous after save = %q, want 1 (NORMAL)", sync)
	}
	if fk := pragma("foreign_keys"); fk != "1" {
		t.Fatalf("foreign_keys after save = %q, want 1 (restored)", fk)
	}
	if temp := pragma("temp_store"); temp != "0" {
		t.Fatalf("temp_store after save = %q, want 0 (DEFAULT)", temp)
	}
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
	if version < 4 {
		t.Fatalf("schema version = %d, want >= 4", version)
	}
}

func TestSaveAcceptsASecondScanIntoTheSameDatabase(t *testing.T) {
	// filesystem_entries.id is global, so numbering each scan's rows from 1
	// collided with every earlier scan and the second save always failed. The
	// engine's default database is long-lived, so this broke the second scan a
	// user ever ran.
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
		},
	}

	firstScan, err := database.Save(context.Background(), result, advisor.Result{}, "complete")
	if err != nil {
		t.Fatalf("first save: %v", err)
	}
	secondScan, err := database.Save(context.Background(), result, advisor.Result{}, "complete")
	if err != nil {
		t.Fatalf("second save into the same database: %v", err)
	}
	if firstScan == secondScan {
		t.Fatal("each scan needs its own id")
	}

	// Both scans keep their own rows, and parent links stay within their scan.
	for _, scanID := range []string{firstScan, secondScan} {
		var rows int
		if err := database.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM filesystem_entries WHERE scan_id = ?`, scanID).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 2 {
			t.Fatalf("scan %s persisted %d entries, want 2", scanID, rows)
		}
		var crossed int
		if err := database.db.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM filesystem_entries child
			   JOIN filesystem_entries parent ON parent.id = child.parent_id
			  WHERE child.scan_id = ? AND parent.scan_id <> child.scan_id`, scanID).Scan(&crossed); err != nil {
			t.Fatal(err)
		}
		if crossed != 0 {
			t.Fatalf("scan %s has %d entries parented to another scan", scanID, crossed)
		}
	}
}
