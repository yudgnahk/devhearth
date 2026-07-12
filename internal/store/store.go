// Package store owns the local SQLite inventory. Paths are persisted locally
// and are never logged or sent over the network by this package.
package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
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

func (s *Store) Save(ctx context.Context, result scan.Result, status string) (string, error) {
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
	defer statement.Close()
	for _, entry := range result.Entries {
		volumeID, found := volumeIDs[entry.DeviceID]
		if !found {
			return "", fmt.Errorf("entry %q has no selected volume", entry.Path)
		}
		if _, err := statement.ExecContext(ctx, scanID, volumeID, entry.Path, entry.Kind, entry.LogicalBytes, entry.AllocatedBytes, fmt.Sprint(entry.DeviceID), fmt.Sprint(entry.Inode), entry.LinkCount, entry.IsSymlink, entry.ModifiedAt.Format(time.RFC3339Nano), entry.HardLinkAlias); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return scanID, nil
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}
