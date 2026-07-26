-- Phase 4: portable policy, recommendation feedback, and scan snapshots for
-- growth trends.
--
-- Two privacy rules shape this schema. Policy documents are stored as the exact
-- portable JSON the user would export, so what is kept and what is shared are
-- the same bytes and cannot drift apart. Snapshots hold only counts and byte
-- totals per named series, never paths: trend history must not become a second
-- copy of the inventory.

CREATE TABLE policies (
    id           TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL,
    name         TEXT NOT NULL DEFAULT '',
    document_json TEXT NOT NULL,
    is_active    INTEGER NOT NULL DEFAULT 0 CHECK (is_active IN (0, 1)),
    created_at   TEXT NOT NULL,
    updated_at   TEXT NOT NULL
);

-- Exactly one policy may be active. A partial unique index enforces it in the
-- database rather than in whichever code path happens to write next.
CREATE UNIQUE INDEX policies_single_active ON policies(is_active) WHERE is_active = 1;

-- Feedback is local-only and deliberately has no export path: a verdict is a
-- usage trace. Users promote a rejection to a portable suppression explicitly,
-- and that suppression lives in the policy document instead.
CREATE TABLE recommendation_feedback (
    id                TEXT PRIMARY KEY,
    recommendation_id TEXT NOT NULL,
    family            TEXT NOT NULL DEFAULT '',
    ecosystem         TEXT NOT NULL DEFAULT '',
    verdict           TEXT NOT NULL CHECK (verdict IN ('accepted', 'rejected', 'unclear', 'later')),
    note              TEXT NOT NULL DEFAULT '',
    scan_id           TEXT,
    created_at        TEXT NOT NULL,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE SET NULL
);

CREATE INDEX recommendation_feedback_by_recommendation
    ON recommendation_feedback(recommendation_id, created_at DESC);

-- One row per completed scan. scope_key groups snapshots taken over the same
-- roots; comparing across scopes would report growth that never happened. It is
-- a one-way digest of local paths and is never exported.
CREATE TABLE scan_snapshots (
    scan_id              TEXT PRIMARY KEY,
    captured_at          TEXT NOT NULL,
    scope_key            TEXT NOT NULL,
    root_count           INTEGER NOT NULL DEFAULT 0,
    entries_visited      INTEGER NOT NULL DEFAULT 0,
    logical_bytes        INTEGER NOT NULL DEFAULT 0,
    allocated_bytes      INTEGER NOT NULL DEFAULT 0,
    recommendation_count INTEGER NOT NULL DEFAULT 0,
    savings_low_bytes    INTEGER NOT NULL DEFAULT 0,
    savings_high_bytes   INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (scan_id) REFERENCES scans(id) ON DELETE CASCADE
);

CREATE INDEX scan_snapshots_by_scope ON scan_snapshots(scope_key, captured_at);

CREATE TABLE scan_snapshot_series (
    scan_id         TEXT NOT NULL,
    series_key      TEXT NOT NULL,
    label           TEXT NOT NULL DEFAULT '',
    allocated_bytes INTEGER NOT NULL DEFAULT 0,
    logical_bytes   INTEGER NOT NULL DEFAULT 0,
    item_count      INTEGER NOT NULL DEFAULT 0,
    uncertain       INTEGER NOT NULL DEFAULT 0 CHECK (uncertain IN (0, 1)),
    PRIMARY KEY (scan_id, series_key),
    FOREIGN KEY (scan_id) REFERENCES scan_snapshots(scan_id) ON DELETE CASCADE
);

INSERT INTO schema_migrations(version, applied_at) VALUES (6, CURRENT_TIMESTAMP);
