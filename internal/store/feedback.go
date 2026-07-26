package store

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Feedback verdicts. "later" is deliberately separate from "rejected": a user
// deferring advice has not disagreed with it, and treating the two the same
// would train the inbox on a decision nobody made.
const (
	VerdictAccepted = "accepted"
	VerdictRejected = "rejected"
	VerdictUnclear  = "unclear"
	VerdictLater    = "later"
)

// Verdicts lists the accepted values.
func Verdicts() []string {
	return []string{VerdictAccepted, VerdictRejected, VerdictUnclear, VerdictLater}
}

// Feedback is one user verdict on one recommendation. It never leaves the
// machine: verdicts are a usage trace, so there is no export path for this
// table. A user who wants a decision to travel records a policy suppression,
// which is path-free and portable by construction.
type Feedback struct {
	ID               string
	RecommendationID string
	Family           string
	Ecosystem        string
	Verdict          string
	Note             string
	ScanID           string
	CreatedAt        time.Time
}

// RecordFeedback stores a verdict. The recommendation id is not a foreign key:
// recommendation ids are stable across scans by design, so feedback outlives
// the scan that produced the advice.
func (s *Store) RecordFeedback(ctx context.Context, feedback Feedback) (Feedback, error) {
	feedback.RecommendationID = strings.TrimSpace(feedback.RecommendationID)
	if feedback.RecommendationID == "" {
		return Feedback{}, fmt.Errorf("feedback needs a recommendation id")
	}
	feedback.Verdict = strings.ToLower(strings.TrimSpace(feedback.Verdict))
	if !slices.Contains(Verdicts(), feedback.Verdict) {
		return Feedback{}, fmt.Errorf("unknown verdict %q; expected one of %s", feedback.Verdict, strings.Join(Verdicts(), ", "))
	}
	if feedback.ID == "" {
		feedback.ID = uuid.NewString()
	}
	if feedback.CreatedAt.IsZero() {
		feedback.CreatedAt = time.Now().UTC()
	}
	// The note is the user's own words and is shown back to them; cap it so a
	// pasted log cannot become a database row of unbounded size.
	const maxNote = 1000
	if len(feedback.Note) > maxNote {
		feedback.Note = feedback.Note[:maxNote]
	}

	var scanID any
	if feedback.ScanID != "" {
		scanID = feedback.ScanID
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO recommendation_feedback(id, recommendation_id, family, ecosystem, verdict, note, scan_id, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		feedback.ID, feedback.RecommendationID, feedback.Family, feedback.Ecosystem,
		feedback.Verdict, feedback.Note, scanID, feedback.CreatedAt.Format(time.RFC3339Nano),
	); err != nil {
		return Feedback{}, fmt.Errorf("record feedback: %w", err)
	}
	return feedback, nil
}

// LatestFeedback returns the most recent verdict per recommendation, newest
// first. An empty recommendationID returns the latest verdict for every
// recommendation that has one.
func (s *Store) LatestFeedback(ctx context.Context, recommendationID string) ([]Feedback, error) {
	query := `
		SELECT f.id, f.recommendation_id, f.family, f.ecosystem, f.verdict, f.note, COALESCE(f.scan_id, ''), f.created_at
		FROM recommendation_feedback f
		JOIN (
			SELECT recommendation_id, MAX(created_at) AS newest
			FROM recommendation_feedback
			GROUP BY recommendation_id
		) latest
		ON latest.recommendation_id = f.recommendation_id AND latest.newest = f.created_at`
	args := []any{}
	if recommendationID != "" {
		query += ` WHERE f.recommendation_id = ?`
		args = append(args, recommendationID)
	}
	query += ` ORDER BY f.created_at DESC`

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("read feedback: %w", err)
	}
	defer rows.Close()
	var out []Feedback
	for rows.Next() {
		var feedback Feedback
		var created string
		if err := rows.Scan(&feedback.ID, &feedback.RecommendationID, &feedback.Family, &feedback.Ecosystem,
			&feedback.Verdict, &feedback.Note, &feedback.ScanID, &created); err != nil {
			return nil, err
		}
		if parsed, parseErr := time.Parse(time.RFC3339Nano, created); parseErr == nil {
			feedback.CreatedAt = parsed
		}
		out = append(out, feedback)
	}
	return out, rows.Err()
}
