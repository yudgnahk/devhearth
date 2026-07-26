-- Phase 3: attributed asset sizes, portfolio fit assessments, and deterministic
-- recommendations. Savings are stored as ranges; a single figure is never durable.

ALTER TABLE assets ADD COLUMN logical_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE assets ADD COLUMN allocated_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE assets ADD COLUMN exclusive_allocated_bytes INTEGER NOT NULL DEFAULT 0;
ALTER TABLE assets ADD COLUMN size_attributed INTEGER NOT NULL DEFAULT 0 CHECK (size_attributed IN (0, 1));
ALTER TABLE assets ADD COLUMN size_shared INTEGER NOT NULL DEFAULT 0 CHECK (size_shared IN (0, 1));
ALTER TABLE assets ADD COLUMN size_uncertain INTEGER NOT NULL DEFAULT 0 CHECK (size_uncertain IN (0, 1));
ALTER TABLE assets ADD COLUMN last_activity_at TEXT;

-- Rollup metadata that attribution reads back: own modification time and the
-- hard-link aliases that make a subtree total a lower bound.
ALTER TABLE directory_aggregates ADD COLUMN modified_at TEXT;
ALTER TABLE directory_aggregates ADD COLUMN hard_link_alias_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE fit_assessments (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    ecosystem TEXT NOT NULL,
    depth TEXT NOT NULL CHECK (depth IN ('deep', 'shallow')),
    project_count INTEGER NOT NULL CHECK (project_count >= 0),
    baseline_tool TEXT NOT NULL DEFAULT '',
    recommended_tool TEXT NOT NULL DEFAULT '',
    stay_put_wins INTEGER NOT NULL DEFAULT 0 CHECK (stay_put_wins IN (0, 1)),
    project_local_install_bytes INTEGER NOT NULL DEFAULT 0,
    shared_store_bytes INTEGER NOT NULL DEFAULT 0,
    version_managers_json TEXT NOT NULL DEFAULT '[]',
    notes_json TEXT NOT NULL DEFAULT '[]',
    UNIQUE (scan_id, ecosystem)
);

CREATE TABLE fit_options (
    id TEXT PRIMARY KEY,
    assessment_id TEXT NOT NULL REFERENCES fit_assessments(id) ON DELETE CASCADE,
    tool TEXT NOT NULL,
    rank INTEGER NOT NULL CHECK (rank >= 1),
    score REAL NOT NULL,
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    stay_put INTEGER NOT NULL DEFAULT 0 CHECK (stay_put IN (0, 1)),
    installed INTEGER NOT NULL DEFAULT 0 CHECK (installed IN (0, 1)),
    projects_using INTEGER NOT NULL DEFAULT 0,
    immediate_savings_low_bytes INTEGER NOT NULL DEFAULT 0,
    immediate_savings_high_bytes INTEGER NOT NULL DEFAULT 0,
    future_growth_reduction_bytes INTEGER NOT NULL DEFAULT 0,
    savings_uncertain INTEGER NOT NULL DEFAULT 0 CHECK (savings_uncertain IN (0, 1)),
    workflow_impact TEXT NOT NULL DEFAULT '',
    factors_json TEXT NOT NULL DEFAULT '[]',
    dominant_factors_json TEXT NOT NULL DEFAULT '[]',
    blockers_json TEXT NOT NULL DEFAULT '[]',
    UNIQUE (assessment_id, tool)
);

CREATE TABLE recommendations (
    id TEXT NOT NULL,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    family TEXT NOT NULL,
    title TEXT NOT NULL,
    ecosystem TEXT NOT NULL DEFAULT '',
    explanation TEXT NOT NULL,
    risk TEXT NOT NULL CHECK (risk IN ('informational', 'low', 'medium', 'high', 'prohibited')),
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    priority REAL NOT NULL DEFAULT 0,
    savings_low_bytes INTEGER NOT NULL DEFAULT 0,
    savings_high_bytes INTEGER NOT NULL DEFAULT 0,
    future_growth_reduction_bytes INTEGER NOT NULL DEFAULT 0,
    savings_uncertain INTEGER NOT NULL DEFAULT 0 CHECK (savings_uncertain IN (0, 1)),
    restoration_cost TEXT NOT NULL DEFAULT '',
    compatibility_impact TEXT NOT NULL DEFAULT '',
    preconditions_json TEXT NOT NULL DEFAULT '[]',
    proposed_actions_json TEXT NOT NULL DEFAULT '[]',
    verification_json TEXT NOT NULL DEFAULT '[]',
    rollback TEXT NOT NULL DEFAULT '',
    blockers_json TEXT NOT NULL DEFAULT '[]',
    alternatives_json TEXT NOT NULL DEFAULT '[]',
    dominant_factors_json TEXT NOT NULL DEFAULT '[]',
    rule_id TEXT NOT NULL,
    rule_version INTEGER NOT NULL,
    PRIMARY KEY (scan_id, id)
);

-- recommendation_affects_asset edges (SPECS §7.8 affected assets).
CREATE TABLE recommendation_assets (
    scan_id TEXT NOT NULL,
    recommendation_id TEXT NOT NULL,
    asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    PRIMARY KEY (scan_id, recommendation_id, asset_id),
    FOREIGN KEY (scan_id, recommendation_id) REFERENCES recommendations(scan_id, id) ON DELETE CASCADE
);

CREATE TABLE recommendation_evidence (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL,
    recommendation_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    value TEXT NOT NULL,
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    FOREIGN KEY (scan_id, recommendation_id) REFERENCES recommendations(scan_id, id) ON DELETE CASCADE
);

CREATE INDEX recommendations_scan_family ON recommendations(scan_id, family);
CREATE INDEX recommendations_scan_priority ON recommendations(scan_id, priority DESC);
CREATE INDEX recommendation_evidence_scan ON recommendation_evidence(scan_id, recommendation_id);
CREATE INDEX fit_assessments_scan ON fit_assessments(scan_id);

INSERT INTO schema_migrations(version, applied_at) VALUES (5, CURRENT_TIMESTAMP);
