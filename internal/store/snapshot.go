package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/yudgnahk/devhearth/internal/trend"
)

// Snapshots are the durable input to trend analysis. They hold counts and byte
// totals per named series and nothing else: keeping history cheap is the point,
// and a trend table full of paths would be a second inventory nobody asked for.

// insertSnapshot writes one snapshot inside the scan transaction, so a scan is
// never durable with a snapshot that disagrees with the advice beside it.
func insertSnapshot(ctx context.Context, tx *sql.Tx, snapshot trend.Snapshot) error {
	if snapshot.ScanID == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO scan_snapshots(scan_id, captured_at, scope_key, root_count, entries_visited, logical_bytes, allocated_bytes, recommendation_count, savings_low_bytes, savings_high_bytes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snapshot.ScanID, snapshot.CapturedAt.UTC().Format(time.RFC3339Nano), snapshot.ScopeKey, snapshot.RootCount,
		snapshot.EntriesVisited, snapshot.LogicalBytes, snapshot.AllocatedBytes,
		snapshot.RecommendationCount, snapshot.SavingsLowBytes, snapshot.SavingsHighBytes,
	); err != nil {
		return fmt.Errorf("persist scan snapshot: %w", err)
	}
	if len(snapshot.Measurements) == 0 {
		return nil
	}
	statement, err := tx.PrepareContext(ctx, `
		INSERT INTO scan_snapshot_series(scan_id, series_key, label, allocated_bytes, logical_bytes, item_count, uncertain)
		VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, measurement := range snapshot.Measurements {
		if _, err := statement.ExecContext(ctx, snapshot.ScanID, measurement.Key, measurement.Label,
			measurement.AllocatedBytes, measurement.LogicalBytes, measurement.ItemCount, boolToInt(measurement.Uncertain),
		); err != nil {
			return fmt.Errorf("persist snapshot series %q: %w", measurement.Key, err)
		}
	}
	return nil
}

// ListSnapshots returns the most recent snapshots, oldest first so the result
// can be handed straight to trend analysis. A limit of zero or less returns the
// default retention window.
func (s *Store) ListSnapshots(ctx context.Context, limit int) ([]trend.Snapshot, error) {
	if limit <= 0 {
		limit = defaultSnapshotLimit
	}
	if limit > maxSnapshotLimit {
		limit = maxSnapshotLimit
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT scan_id, captured_at, scope_key, root_count, entries_visited, logical_bytes, allocated_bytes, recommendation_count, savings_low_bytes, savings_high_bytes
		FROM scan_snapshots
		ORDER BY captured_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list snapshots: %w", err)
	}
	defer rows.Close()

	var snapshots []trend.Snapshot
	byID := map[string]int{}
	for rows.Next() {
		var snapshot trend.Snapshot
		var captured string
		if err := rows.Scan(&snapshot.ScanID, &captured, &snapshot.ScopeKey, &snapshot.RootCount,
			&snapshot.EntriesVisited, &snapshot.LogicalBytes, &snapshot.AllocatedBytes,
			&snapshot.RecommendationCount, &snapshot.SavingsLowBytes, &snapshot.SavingsHighBytes); err != nil {
			return nil, err
		}
		if parsed, parseErr := time.Parse(time.RFC3339Nano, captured); parseErr == nil {
			snapshot.CapturedAt = parsed
		}
		byID[snapshot.ScanID] = len(snapshots)
		snapshots = append(snapshots, snapshot)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(snapshots) == 0 {
		return nil, nil
	}
	if err := s.attachSeries(ctx, snapshots, byID); err != nil {
		return nil, err
	}
	// Reverse into chronological order; the query sorted newest-first to apply
	// the limit to recent history rather than to the oldest rows.
	for left, right := 0, len(snapshots)-1; left < right; left, right = left+1, right-1 {
		snapshots[left], snapshots[right] = snapshots[right], snapshots[left]
	}
	return snapshots, nil
}

// Retention bounds. The upper bound exists so a caller-supplied limit cannot
// pull an unbounded history into memory.
const (
	defaultSnapshotLimit = 30
	maxSnapshotLimit     = 365
)

// attachSeries loads the measurements for exactly the snapshots that were
// selected. The scan ids are bound as parameters rather than interpolated, and
// the query is bounded by the caller's limit, so a long history does not pull
// every series row ever written into memory.
func (s *Store) attachSeries(ctx context.Context, snapshots []trend.Snapshot, byID map[string]int) error {
	if len(snapshots) == 0 {
		return nil
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(snapshots)), ",")
	args := make([]any, 0, len(snapshots))
	for _, snapshot := range snapshots {
		args = append(args, snapshot.ScanID)
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT scan_id, series_key, label, allocated_bytes, logical_bytes, item_count, uncertain
		FROM scan_snapshot_series
		WHERE scan_id IN (`+placeholders+`)
		ORDER BY scan_id, series_key`, args...)
	if err != nil {
		return fmt.Errorf("list snapshot series: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var scanID string
		var measurement trend.Measurement
		var uncertain int
		if err := rows.Scan(&scanID, &measurement.Key, &measurement.Label,
			&measurement.AllocatedBytes, &measurement.LogicalBytes, &measurement.ItemCount, &uncertain); err != nil {
			return err
		}
		index, wanted := byID[scanID]
		if !wanted {
			continue
		}
		measurement.Uncertain = uncertain == 1
		snapshots[index].Measurements = append(snapshots[index].Measurements, measurement)
	}
	return rows.Err()
}

// PruneSnapshots drops the oldest snapshots beyond the retention count. It
// touches only the trend tables; inventory rows are kept until a scan is
// removed for other reasons.
func (s *Store) PruneSnapshots(ctx context.Context, keep int) error {
	if keep <= 0 {
		keep = defaultSnapshotLimit
	}
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM scan_snapshot_series
		WHERE scan_id IN (
			SELECT scan_id FROM scan_snapshots
			ORDER BY captured_at DESC
			LIMIT -1 OFFSET ?
		)`, keep); err != nil {
		return fmt.Errorf("prune snapshot series: %w", err)
	}
	if _, err := s.db.ExecContext(ctx, `
		DELETE FROM scan_snapshots
		WHERE scan_id IN (
			SELECT scan_id FROM scan_snapshots
			ORDER BY captured_at DESC
			LIMIT -1 OFFSET ?
		)`, keep); err != nil {
		return fmt.Errorf("prune snapshots: %w", err)
	}
	return nil
}
