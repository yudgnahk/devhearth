-- Rolled-up inventory nodes for directory drill-down (direct children + subtree totals).
CREATE TABLE directory_aggregates (
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    parent_path TEXT NOT NULL DEFAULT '',
    name TEXT NOT NULL,
    kind TEXT NOT NULL,
    logical_bytes INTEGER NOT NULL CHECK (logical_bytes >= 0),
    allocated_bytes INTEGER NOT NULL CHECK (allocated_bytes >= 0),
    total_logical_bytes INTEGER NOT NULL CHECK (total_logical_bytes >= 0),
    total_allocated_bytes INTEGER NOT NULL CHECK (total_allocated_bytes >= 0),
    direct_child_count INTEGER NOT NULL CHECK (direct_child_count >= 0),
    is_symlink INTEGER NOT NULL DEFAULT 0 CHECK (is_symlink IN (0, 1)),
    PRIMARY KEY (scan_id, path)
);

CREATE INDEX directory_aggregates_parent ON directory_aggregates(scan_id, parent_path);

INSERT INTO schema_migrations(version, applied_at) VALUES (4, CURRENT_TIMESTAMP);
