// Package store owns the local SQLite inventory. Paths are persisted locally
// and are never logged or sent over the network by this package.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yudgnahk/devhearth/internal/assets"
	"github.com/yudgnahk/devhearth/internal/scan"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

type Store struct{ db *sql.DB }

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

// setPragmas applies connection settings in order. Errors are reported rather
// than ignored: a dropped `foreign_keys = ON` would silently disable integrity
// checks for every later write on this connection.
func (s *Store) setPragmas(ctx context.Context, statements ...string) error {
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("apply %q: %w", statement, err)
		}
	}
	return nil
}

func (s *Store) migrate(ctx context.Context) error {
	// WAL with synchronous = NORMAL is the durable-but-fast baseline: commits
	// avoid an fsync while a crash still cannot corrupt the database.
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL; PRAGMA synchronous = NORMAL;"); err != nil {
		return fmt.Errorf("apply connection settings: %w", err)
	}
	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		versionText, _, found := strings.Cut(entry.Name(), "_")
		if !found {
			return fmt.Errorf("migration %q must begin with a version", entry.Name())
		}
		version, err := strconv.Atoi(versionText)
		if err != nil {
			return fmt.Errorf("parse migration %q: %w", entry.Name(), err)
		}
		var applied int
		err = s.db.QueryRowContext(ctx, `SELECT 1 FROM schema_migrations WHERE version = ?`, version).Scan(&applied)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows && !strings.Contains(err.Error(), "no such table") {
			return err
		}
		contents, err := migrations.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return err
		}
		if _, err := s.db.ExecContext(ctx, string(contents)); err != nil {
			return fmt.Errorf("apply %s: %w", entry.Name(), err)
		}
	}
	return nil
}

// SaveOptions tunes bulk persistence. Zero value keeps previous behavior.
type SaveOptions struct {
	// DirectoryIndex reuses a precomputed rollup; when nil, Save builds one.
	DirectoryIndex map[string][]scan.DirectoryNode
	// Progress reports durable rows written so far and the planned total.
	Progress func(written, total int64)
}

// insertBatchSize balances statement size against modernc/sqlite bind overhead.
const insertBatchSize = 800

// alwaysBulkNames are directory names whose contents are always regenerable
// bulk data, identifiable from the name alone.
var alwaysBulkNames = map[string]struct{}{
	"node_modules": {}, ".git": {}, ".svn": {}, ".hg": {},
	".venv": {}, "venv": {}, "__pycache__": {}, ".build": {},
	"Pods": {}, ".gradle": {}, ".bun": {}, ".next": {}, ".turbo": {}, ".cache": {},
}

// cargoProfileNames are bulk only directly beneath a directory named "target",
// so a hand-written "debug" or "release" folder elsewhere is left alone.
var cargoProfileNames = map[string]struct{}{"debug": {}, "release": {}, "tmp": {}}

// ambiguousBulkNames are just as often hand-written source directories as
// generated output, so a name match alone is not evidence.
var ambiguousBulkNames = map[string]struct{}{"build": {}, "dist": {}}

// buildManifestNames sitting beside an ambiguous directory are the evidence
// that it is generated output rather than source.
var buildManifestNames = map[string]struct{}{
	"package.json": {}, "pyproject.toml": {}, "setup.py": {}, "Cargo.toml": {},
	"CMakeLists.txt": {}, "Makefile": {}, "meson.build": {},
	"pom.xml": {}, "build.gradle": {}, "build.gradle.kts": {}, "Package.swift": {},
}

// bulkTree holds the directories whose descendants are omitted from durable
// inventory. Each marker directory itself is still persisted, so sizes and
// assets remain; only its interior is dropped to keep Save bounded. The live
// session continues to drill into the full in-memory inventory.
type bulkTree struct {
	roots map[string]struct{}
}

// newBulkTree resolves bulk roots from inventory entries. Ambiguous names need
// sibling evidence, so when a directory could plausibly be source it is kept:
// persisting extra rows is recoverable, silently dropping a source tree is not.
func newBulkTree(entries []scan.Entry) bulkTree {
	roots := make(map[string]struct{})
	// Only parents of ambiguous directories are tracked, so the second pass
	// stays bounded rather than indexing every basename in the scan.
	candidates := make(map[string][]string)
	for _, entry := range entries {
		if entry.Kind != "directory" {
			continue
		}
		name := filepath.Base(entry.Path)
		if _, ok := alwaysBulkNames[name]; ok {
			roots[entry.Path] = struct{}{}
			continue
		}
		if _, ok := cargoProfileNames[name]; ok && filepath.Base(entry.ParentPath) == "target" {
			roots[entry.Path] = struct{}{}
			continue
		}
		if _, ok := ambiguousBulkNames[name]; ok && entry.ParentPath != "" {
			candidates[entry.ParentPath] = append(candidates[entry.ParentPath], entry.Path)
		}
	}
	for _, entry := range entries {
		if len(candidates) == 0 {
			break
		}
		paths, watched := candidates[entry.ParentPath]
		if !watched {
			continue
		}
		if _, ok := buildManifestNames[filepath.Base(entry.Path)]; !ok {
			continue
		}
		for _, path := range paths {
			roots[path] = struct{}{}
		}
		delete(candidates, entry.ParentPath)
	}
	return bulkTree{roots: roots}
}

// contains reports whether path lies below a bulk root (the root itself does not).
func (b bulkTree) contains(path string) bool {
	if len(b.roots) == 0 {
		return false
	}
	for current := filepath.Dir(path); ; {
		if _, ok := b.roots[current]; ok {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current {
			return false
		}
		current = parent
	}
}

// Save persists inventory metadata and an optional asset graph for one scan.
func (s *Store) Save(ctx context.Context, result scan.Result, graph assets.Graph, status string) (string, error) {
	return s.SaveWithOptions(ctx, result, graph, status, SaveOptions{})
}

// SaveWithOptions is Save with bulk-load options (progress, reused directory index).
func (s *Store) SaveWithOptions(ctx context.Context, result scan.Result, graph assets.Graph, status string, options SaveOptions) (_ string, err error) {
	if status == "" {
		status = "complete"
	}
	scanID := uuid.NewString()

	// Bulk-load settings for this write, restored before returning. Durability
	// settings are deliberately left at the connection baseline so a crash
	// mid-save cannot corrupt the inventory.
	if err := s.setPragmas(ctx, `PRAGMA temp_store = MEMORY`, `PRAGMA foreign_keys = OFF`); err != nil {
		return "", err
	}
	defer func() {
		err = errors.Join(err, s.setPragmas(context.Background(), `PRAGMA temp_store = DEFAULT`, `PRAGMA foreign_keys = ON`))
	}()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	completed := nullableTime(result.CompletedAt)
	if _, err := tx.ExecContext(ctx, `INSERT INTO scans(id, schema_version, status, started_at, completed_at) VALUES (?, 1, ?, ?, ?)`, scanID, status, result.StartedAt.Format(time.RFC3339Nano), completed); err != nil {
		return "", err
	}
	volumeIDs := map[uint64]string{}
	for _, root := range result.Roots {
		var device uint64
		for _, entry := range result.Entries {
			if entry.Path == root {
				device = entry.DeviceID
				break
			}
		}
		volumeID := uuid.NewString()
		volumeIDs[device] = volumeID
		if _, err := tx.ExecContext(ctx, `INSERT INTO volumes(id, scan_id, root_path, device_id) VALUES (?, ?, ?, ?)`, volumeID, scanID, root, fmt.Sprint(device)); err != nil {
			return "", err
		}
	}

	// Keep asset primary paths even when they fall inside a bulk tree.
	keepPaths := make(map[string]struct{}, len(graph.Assets))
	for _, asset := range graph.Assets {
		if asset.Path != "" {
			keepPaths[asset.Path] = struct{}{}
		}
	}

	bulk := newBulkTree(result.Entries)

	// Depth order + explicit row ids lets us set parent_id in the INSERT and
	// avoid a second pass of per-row UPDATEs (dominant cost on large trees).
	entries := make([]scan.Entry, 0, len(result.Entries)/4+len(keepPaths))
	for _, entry := range result.Entries {
		if _, keep := keepPaths[entry.Path]; !keep && bulk.contains(entry.Path) {
			continue
		}
		entries = append(entries, entry)
	}
	sortEntriesByPathDepth(entries)
	pathToEntryID := make(map[string]int64, len(entries))
	for i, entry := range entries {
		pathToEntryID[entry.Path] = int64(i + 1)
	}

	dirIndex := options.DirectoryIndex
	if dirIndex == nil {
		dirIndex = scan.BuildDirectoryIndex(result)
	}
	aggregates := make([]scan.DirectoryNode, 0, len(entries))
	for _, node := range flattenDirectoryIndex(dirIndex) {
		if _, keep := keepPaths[node.Path]; !keep && bulk.contains(node.Path) {
			continue
		}
		aggregates = append(aggregates, node)
	}
	totalRows := int64(len(entries) + len(aggregates))
	var written int64
	var lastReported int64
	report := func(n int) {
		written += int64(n)
		if options.Progress == nil {
			return
		}
		// Throttle UI/protocol chatter: ~2% steps, always include start/end.
		step := totalRows / 50
		if step < 1 {
			step = 1
		}
		if written == totalRows || written-lastReported >= step {
			lastReported = written
			options.Progress(written, totalRows)
		}
	}
	if options.Progress != nil {
		options.Progress(0, totalRows)
	}

	if err := insertFilesystemEntries(ctx, tx, scanID, entries, volumeIDs, pathToEntryID, report); err != nil {
		return "", err
	}
	if err := insertDirectoryAggregates(ctx, tx, scanID, aggregates, report); err != nil {
		return "", err
	}
	if err := insertGraph(ctx, tx, scanID, graph, pathToEntryID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if options.Progress != nil {
		options.Progress(totalRows, totalRows)
	}
	return scanID, nil
}

func insertFilesystemEntries(
	ctx context.Context,
	tx *sql.Tx,
	scanID string,
	entries []scan.Entry,
	volumeIDs map[uint64]string,
	pathToEntryID map[string]int64,
	report func(n int),
) error {
	const columns = 14
	const rowSQL = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	prefix := `INSERT INTO filesystem_entries(id, scan_id, volume_id, parent_id, path, kind, logical_bytes, allocated_bytes, device_id, inode, link_count, is_symlink, modified_at, hard_link_alias) VALUES `

	for start := 0; start < len(entries); start += insertBatchSize {
		end := start + insertBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[start:end]
		var b strings.Builder
		b.WriteString(prefix)
		args := make([]any, 0, len(batch)*columns)
		for i, entry := range batch {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(rowSQL)
			volumeID, found := volumeIDs[entry.DeviceID]
			if !found {
				return fmt.Errorf("entry %q has no selected volume", entry.Path)
			}
			entryID := pathToEntryID[entry.Path]
			var parentID any
			if entry.ParentPath != "" {
				if id, ok := pathToEntryID[entry.ParentPath]; ok {
					parentID = id
				}
			}
			isSymlink := 0
			if entry.IsSymlink {
				isSymlink = 1
			}
			hardLinkAlias := 0
			if entry.HardLinkAlias {
				hardLinkAlias = 1
			}
			args = append(args,
				entryID, scanID, volumeID, parentID, entry.Path, entry.Kind,
				entry.LogicalBytes, entry.AllocatedBytes,
				fmt.Sprint(entry.DeviceID), fmt.Sprint(entry.Inode), entry.LinkCount,
				isSymlink, entry.ModifiedAt.Format(time.RFC3339Nano), hardLinkAlias,
			)
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
		if report != nil {
			report(len(batch))
		}
	}
	return nil
}

func sortEntriesByPathDepth(entries []scan.Entry) {
	sort.Slice(entries, func(i, j int) bool {
		di := strings.Count(entries[i].Path, string(filepath.Separator))
		dj := strings.Count(entries[j].Path, string(filepath.Separator))
		if di != dj {
			return di < dj
		}
		return entries[i].Path < entries[j].Path
	})
}

func flattenDirectoryIndex(index map[string][]scan.DirectoryNode) []scan.DirectoryNode {
	if len(index) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(index))
	out := make([]scan.DirectoryNode, 0, len(index))
	for _, nodes := range index {
		for _, node := range nodes {
			if _, ok := seen[node.Path]; ok {
				continue
			}
			seen[node.Path] = struct{}{}
			out = append(out, node)
		}
	}
	return out
}

func insertDirectoryAggregates(ctx context.Context, tx *sql.Tx, scanID string, aggregates []scan.DirectoryNode, report func(n int)) error {
	const columns = 11
	const rowSQL = "(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)"
	prefix := `INSERT INTO directory_aggregates(scan_id, path, parent_path, name, kind, logical_bytes, allocated_bytes, total_logical_bytes, total_allocated_bytes, direct_child_count, is_symlink) VALUES `

	for start := 0; start < len(aggregates); start += insertBatchSize {
		end := start + insertBatchSize
		if end > len(aggregates) {
			end = len(aggregates)
		}
		batch := aggregates[start:end]
		var b strings.Builder
		b.WriteString(prefix)
		args := make([]any, 0, len(batch)*columns)
		for i, node := range batch {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(rowSQL)
			isSymlink := 0
			if node.IsSymlink {
				isSymlink = 1
			}
			args = append(args,
				scanID, node.Path, node.ParentPath, node.Name, node.Kind,
				node.LogicalBytes, node.AllocatedBytes, node.TotalLogicalBytes, node.TotalAllocatedBytes,
				node.DirectChildCount, isSymlink,
			)
		}
		if _, err := tx.ExecContext(ctx, b.String(), args...); err != nil {
			return err
		}
		if report != nil {
			report(len(batch))
		}
	}
	return nil
}

// ListDirectoryChildren returns direct children for a persisted scan (absolute paths).
func (s *Store) ListDirectoryChildren(ctx context.Context, scanID, parentPath string) ([]scan.DirectoryNode, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT path, parent_path, name, kind, logical_bytes, allocated_bytes, total_logical_bytes, total_allocated_bytes, direct_child_count, is_symlink FROM directory_aggregates WHERE scan_id = ? AND parent_path = ? ORDER BY kind = 'directory' DESC, name COLLATE NOCASE`, scanID, parentPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []scan.DirectoryNode
	for rows.Next() {
		var node scan.DirectoryNode
		var isSymlink int
		if err := rows.Scan(&node.Path, &node.ParentPath, &node.Name, &node.Kind, &node.LogicalBytes, &node.AllocatedBytes, &node.TotalLogicalBytes, &node.TotalAllocatedBytes, &node.DirectChildCount, &isSymlink); err != nil {
			return nil, err
		}
		node.IsSymlink = isSymlink == 1
		result = append(result, node)
	}
	return result, rows.Err()
}

func insertGraph(ctx context.Context, tx *sql.Tx, scanID string, graph assets.Graph, pathToEntryID map[string]int64) error {
	assetStmt, err := tx.PrepareContext(ctx, `INSERT INTO assets(id, scan_id, kind, display_name, risk, detector_id, detector_version, ecosystem, class, primary_path, attributes_json) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer assetStmt.Close()
	locStmt, err := tx.PrepareContext(ctx, `INSERT INTO asset_locations(asset_id, entry_id) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer locStmt.Close()
	evidenceStmt, err := tx.PrepareContext(ctx, `INSERT INTO evidence(id, scan_id, detector_id, detector_version, kind, value, confidence, asset_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer evidenceStmt.Close()

	for _, asset := range graph.Assets {
		attrs, err := json.Marshal(asset.Attributes)
		if err != nil {
			return err
		}
		if string(attrs) == "null" {
			attrs = []byte("{}")
		}
		if _, err := assetStmt.ExecContext(ctx, asset.ID, scanID, string(asset.Kind), asset.DisplayName, string(asset.Risk), asset.DetectorID, asset.DetectorVersion, asset.Ecosystem, string(asset.Class), asset.Path, string(attrs)); err != nil {
			return err
		}
		if entryID, ok := pathToEntryID[asset.Path]; ok {
			if _, err := locStmt.ExecContext(ctx, asset.ID, entryID); err != nil {
				return err
			}
		}
		for _, evidence := range asset.Evidence {
			if _, err := evidenceStmt.ExecContext(ctx, uuid.NewString(), scanID, asset.DetectorID, asset.DetectorVersion, evidence.Kind, evidence.Value, evidence.Confidence, asset.ID); err != nil {
				return err
			}
		}
	}

	relStmt, err := tx.PrepareContext(ctx, `INSERT INTO relationships(id, scan_id, source_asset_id, target_asset_id, kind, evidence_id, confidence, detector_id, detector_version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer relStmt.Close()
	for _, rel := range graph.Relationships {
		evidenceID := uuid.NewString()
		detectorID := rel.DetectorID
		if detectorID == "" {
			detectorID = "detect.unknown"
		}
		version := rel.DetectorVersion
		if version < 1 {
			version = 1
		}
		if _, err := evidenceStmt.ExecContext(ctx, evidenceID, scanID, detectorID, version, rel.Evidence.Kind, rel.Evidence.Value, rel.Confidence, nil); err != nil {
			return err
		}
		if _, err := relStmt.ExecContext(ctx, rel.ID, scanID, rel.SourceID, rel.TargetID, rel.Kind, evidenceID, rel.Confidence, detectorID, version); err != nil {
			return err
		}
	}
	return nil
}

// ListAssets returns assets for a scan ID, newest-matching scan only when
// looking up by protocol scan handles that map to engine-assigned IDs.
func (s *Store) ListAssets(ctx context.Context, scanID string) ([]assets.Asset, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, kind, display_name, risk, detector_id, detector_version, ecosystem, class, primary_path, attributes_json FROM assets WHERE scan_id = ? ORDER BY kind, display_name, primary_path`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []assets.Asset
	for rows.Next() {
		var asset assets.Asset
		var kind, risk, class, attrs string
		if err := rows.Scan(&asset.ID, &kind, &asset.DisplayName, &risk, &asset.DetectorID, &asset.DetectorVersion, &asset.Ecosystem, &class, &asset.Path, &attrs); err != nil {
			return nil, err
		}
		asset.Kind = assets.Kind(kind)
		asset.Risk = assets.Risk(risk)
		asset.Class = assets.Class(class)
		if attrs != "" && attrs != "{}" {
			_ = json.Unmarshal([]byte(attrs), &asset.Attributes)
		}
		result = append(result, asset)
	}
	return result, rows.Err()
}

// ListRelationships returns relationship edges for a scan.
func (s *Store) ListRelationships(ctx context.Context, scanID string) ([]assets.Relationship, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT r.id, r.source_asset_id, r.target_asset_id, r.kind, r.confidence, r.detector_id, r.detector_version, e.kind, e.value FROM relationships r JOIN evidence e ON e.id = r.evidence_id WHERE r.scan_id = ? ORDER BY r.kind`, scanID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []assets.Relationship
	for rows.Next() {
		var rel assets.Relationship
		var evidenceKind, evidenceValue string
		if err := rows.Scan(&rel.ID, &rel.SourceID, &rel.TargetID, &rel.Kind, &rel.Confidence, &rel.DetectorID, &rel.DetectorVersion, &evidenceKind, &evidenceValue); err != nil {
			return nil, err
		}
		rel.Evidence = assets.Evidence{Kind: evidenceKind, Value: evidenceValue, Confidence: rel.Confidence}
		result = append(result, rel)
	}
	return result, rows.Err()
}

// ListEvidence returns detector evidence rows for a scan, optionally filtered by asset.
func (s *Store) ListEvidence(ctx context.Context, scanID, assetID string) ([]assets.Evidence, error) {
	var rows *sql.Rows
	var err error
	if assetID == "" {
		rows, err = s.db.QueryContext(ctx, `SELECT kind, value, confidence FROM evidence WHERE scan_id = ? ORDER BY kind`, scanID)
	} else {
		rows, err = s.db.QueryContext(ctx, `SELECT kind, value, confidence FROM evidence WHERE scan_id = ? AND asset_id = ? ORDER BY kind`, scanID, assetID)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []assets.Evidence
	for rows.Next() {
		var evidence assets.Evidence
		if err := rows.Scan(&evidence.Kind, &evidence.Value, &evidence.Confidence); err != nil {
			return nil, err
		}
		result = append(result, evidence)
	}
	return result, rows.Err()
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}
