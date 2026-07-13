// Package store owns the local SQLite inventory. Paths are persisted locally
// and are never logged or sent over the network by this package.
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
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

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.db.ExecContext(ctx, "PRAGMA foreign_keys = ON; PRAGMA journal_mode = WAL;"); err != nil {
		return err
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

// Save persists inventory metadata and an optional asset graph for one scan.
func (s *Store) Save(ctx context.Context, result scan.Result, graph assets.Graph, status string) (string, error) {
	if status == "" {
		status = "complete"
	}
	scanID := uuid.NewString()
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
	statement, err := tx.PrepareContext(ctx, `INSERT INTO filesystem_entries(scan_id, volume_id, parent_id, path, kind, logical_bytes, allocated_bytes, device_id, inode, link_count, is_symlink, modified_at, hard_link_alias) VALUES (?, ?, NULL, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return "", err
	}
	pathToEntryID := make(map[string]int64, len(result.Entries))
	// Insert in path-length order so parents exist before children when we patch parent_id.
	entries := append([]scan.Entry(nil), result.Entries...)
	sortEntriesByPathDepth(entries)
	for _, entry := range entries {
		volumeID, found := volumeIDs[entry.DeviceID]
		if !found {
			statement.Close()
			return "", fmt.Errorf("entry %q has no selected volume", entry.Path)
		}
		execResult, err := statement.ExecContext(ctx, scanID, volumeID, entry.Path, entry.Kind, entry.LogicalBytes, entry.AllocatedBytes, fmt.Sprint(entry.DeviceID), fmt.Sprint(entry.Inode), entry.LinkCount, entry.IsSymlink, entry.ModifiedAt.Format(time.RFC3339Nano), entry.HardLinkAlias)
		if err != nil {
			statement.Close()
			return "", err
		}
		entryID, err := execResult.LastInsertId()
		if err != nil {
			statement.Close()
			return "", err
		}
		pathToEntryID[entry.Path] = entryID
	}
	statement.Close()

	// Wire parent_id for directory drill-down queries against persisted inventory.
	for _, entry := range entries {
		if entry.ParentPath == "" {
			continue
		}
		parentID, ok := pathToEntryID[entry.ParentPath]
		if !ok {
			continue
		}
		if _, err := tx.ExecContext(ctx, `UPDATE filesystem_entries SET parent_id = ? WHERE scan_id = ? AND path = ?`, parentID, scanID, entry.Path); err != nil {
			return "", err
		}
	}

	if err := insertDirectoryAggregates(ctx, tx, scanID, result); err != nil {
		return "", err
	}

	if err := insertGraph(ctx, tx, scanID, graph, pathToEntryID); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return scanID, nil
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

func insertDirectoryAggregates(ctx context.Context, tx *sql.Tx, scanID string, result scan.Result) error {
	index := scan.BuildDirectoryIndex(result)
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO directory_aggregates(scan_id, path, parent_path, name, kind, logical_bytes, allocated_bytes, total_logical_bytes, total_allocated_bytes, direct_child_count, is_symlink) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	seen := map[string]struct{}{}
	for _, nodes := range index {
		for _, node := range nodes {
			if _, ok := seen[node.Path]; ok {
				continue
			}
			seen[node.Path] = struct{}{}
			isSymlink := 0
			if node.IsSymlink {
				isSymlink = 1
			}
			if _, err := stmt.ExecContext(ctx, scanID, node.Path, node.ParentPath, node.Name, node.Kind, node.LogicalBytes, node.AllocatedBytes, node.TotalLogicalBytes, node.TotalAllocatedBytes, node.DirectChildCount, isSymlink); err != nil {
				return err
			}
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
