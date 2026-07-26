package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/yudgnahk/devhearth/internal/policy"
)

// ErrNoPolicy reports that no policy has been stored yet. Callers decide
// whether that means "use the defaults" or "ask the user"; the store does not
// invent one on read.
var ErrNoPolicy = errors.New("no policy is stored")

// Policies are persisted as the exact portable JSON a user would export, so the
// stored form and the shared form cannot drift apart. Every read revalidates:
// the database file is local, but it is still a file somebody can edit.

// ActivePolicy returns the active policy document.
func (s *Store) ActivePolicy(ctx context.Context) (policy.Document, error) {
	return s.policyRow(ctx, `SELECT document_json FROM policies WHERE is_active = 1 LIMIT 1`)
}

// PolicyByID returns one stored policy.
func (s *Store) PolicyByID(ctx context.Context, id string) (policy.Document, error) {
	if id == "" {
		return policy.Document{}, fmt.Errorf("policy id is required")
	}
	return s.policyRow(ctx, `SELECT document_json FROM policies WHERE id = ?`, id)
}

func (s *Store) policyRow(ctx context.Context, query string, args ...any) (policy.Document, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return policy.Document{}, ErrNoPolicy
	}
	if err != nil {
		return policy.Document{}, fmt.Errorf("read policy: %w", err)
	}
	document, err := policy.Parse([]byte(raw))
	if err != nil {
		return policy.Document{}, fmt.Errorf("stored policy is not usable: %w", err)
	}
	return document, nil
}

// EnsureActivePolicy returns the active policy, storing a default one the first
// time the engine runs so later writes always have a row to update.
func (s *Store) EnsureActivePolicy(ctx context.Context, defaultID string) (policy.Document, error) {
	document, err := s.ActivePolicy(ctx)
	if err == nil {
		return document, nil
	}
	if !errors.Is(err, ErrNoPolicy) {
		return policy.Document{}, err
	}
	created := policy.DefaultDocument(defaultID)
	if err := s.SavePolicy(ctx, created, true); err != nil {
		return policy.Document{}, err
	}
	return created, nil
}

// SavePolicy validates and stores a document, optionally making it the active
// one. Activation clears the previous active row inside the same transaction so
// the single-active invariant never has a window where it is false.
func (s *Store) SavePolicy(ctx context.Context, document policy.Document, activate bool) error {
	validated, err := policy.Validate(document)
	if err != nil {
		return fmt.Errorf("refusing to store an invalid policy: %w", err)
	}
	encoded, err := policy.Marshal(validated)
	if err != nil {
		return fmt.Errorf("encode policy: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if activate {
		if _, err := tx.ExecContext(ctx, `UPDATE policies SET is_active = 0, updated_at = ? WHERE is_active = 1 AND id <> ?`, now, validated.ID); err != nil {
			return fmt.Errorf("clear the previously active policy: %w", err)
		}
	}
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO policies(id, schema_version, name, document_json, is_active, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			schema_version = excluded.schema_version,
			name = excluded.name,
			document_json = excluded.document_json,
			is_active = MAX(policies.is_active, excluded.is_active),
			updated_at = excluded.updated_at`,
		validated.ID, validated.SchemaVersion, validated.Name, string(encoded), boolToInt(activate), now, now,
	); err != nil {
		return fmt.Errorf("store policy: %w", err)
	}
	return tx.Commit()
}

// PolicySummary describes a stored policy without decoding the whole document.
type PolicySummary struct {
	ID        string
	Name      string
	Active    bool
	UpdatedAt time.Time
}

// ListPolicies returns stored policies, active first then most recently updated.
func (s *Store) ListPolicies(ctx context.Context) ([]PolicySummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, is_active, updated_at FROM policies ORDER BY is_active DESC, updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list policies: %w", err)
	}
	defer rows.Close()
	var out []PolicySummary
	for rows.Next() {
		var summary PolicySummary
		var active int
		var updated string
		if err := rows.Scan(&summary.ID, &summary.Name, &active, &updated); err != nil {
			return nil, err
		}
		summary.Active = active == 1
		if parsed, parseErr := time.Parse(time.RFC3339Nano, updated); parseErr == nil {
			summary.UpdatedAt = parsed
		}
		out = append(out, summary)
	}
	return out, rows.Err()
}
