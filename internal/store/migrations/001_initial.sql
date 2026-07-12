PRAGMA foreign_keys = ON;

-- Migrations are self-recording. The future runner must execute each migration
-- transactionally and must not insert a second schema_migrations row.
CREATE TABLE schema_migrations (
    version INTEGER PRIMARY KEY,
    applied_at TEXT NOT NULL
);

CREATE TABLE scans (
    id TEXT PRIMARY KEY,
    schema_version INTEGER NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'complete', 'cancelled', 'failed')),
    started_at TEXT NOT NULL,
    completed_at TEXT
);

CREATE TABLE volumes (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    root_path TEXT NOT NULL,
    device_id TEXT NOT NULL,
    UNIQUE (scan_id, root_path)
);

CREATE TABLE filesystem_entries (
    id INTEGER PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    volume_id TEXT NOT NULL REFERENCES volumes(id) ON DELETE CASCADE,
    parent_id INTEGER REFERENCES filesystem_entries(id) ON DELETE CASCADE,
    path TEXT NOT NULL,
    kind TEXT NOT NULL,
    logical_bytes INTEGER NOT NULL CHECK (logical_bytes >= 0),
    allocated_bytes INTEGER NOT NULL CHECK (allocated_bytes >= 0),
    device_id TEXT NOT NULL,
    inode TEXT NOT NULL,
    link_count INTEGER NOT NULL CHECK (link_count >= 1),
    is_symlink INTEGER NOT NULL DEFAULT 0 CHECK (is_symlink IN (0, 1)),
    UNIQUE (scan_id, path)
);

CREATE TABLE assets (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    display_name TEXT NOT NULL,
    risk TEXT NOT NULL CHECK (risk IN ('informational', 'low', 'medium', 'high', 'prohibited')),
    detector_id TEXT NOT NULL,
    detector_version INTEGER NOT NULL
);

CREATE TABLE asset_locations (
    asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    entry_id INTEGER NOT NULL REFERENCES filesystem_entries(id) ON DELETE CASCADE,
    PRIMARY KEY (asset_id, entry_id)
);

CREATE TABLE evidence (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    detector_id TEXT NOT NULL,
    detector_version INTEGER NOT NULL,
    kind TEXT NOT NULL,
    value TEXT NOT NULL,
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1)
);

CREATE TABLE relationships (
    id TEXT PRIMARY KEY,
    scan_id TEXT NOT NULL REFERENCES scans(id) ON DELETE CASCADE,
    source_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    target_asset_id TEXT NOT NULL REFERENCES assets(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    evidence_id TEXT NOT NULL REFERENCES evidence(id),
    confidence REAL NOT NULL CHECK (confidence >= 0 AND confidence <= 1),
    detector_id TEXT NOT NULL,
    detector_version INTEGER NOT NULL
);

CREATE INDEX filesystem_entries_scan_parent ON filesystem_entries(scan_id, parent_id);
CREATE INDEX filesystem_entries_identity ON filesystem_entries(scan_id, device_id, inode);
CREATE INDEX assets_scan_kind ON assets(scan_id, kind);
CREATE INDEX relationships_scan_kind ON relationships(scan_id, kind);

INSERT INTO schema_migrations(version, applied_at) VALUES (1, CURRENT_TIMESTAMP);
