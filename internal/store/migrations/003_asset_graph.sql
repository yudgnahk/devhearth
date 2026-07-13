ALTER TABLE assets ADD COLUMN ecosystem TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN class TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN primary_path TEXT NOT NULL DEFAULT '';
ALTER TABLE assets ADD COLUMN attributes_json TEXT NOT NULL DEFAULT '{}';

ALTER TABLE evidence ADD COLUMN asset_id TEXT REFERENCES assets(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS assets_scan_path ON assets(scan_id, primary_path);
CREATE INDEX IF NOT EXISTS evidence_scan_asset ON evidence(scan_id, asset_id);

INSERT INTO schema_migrations(version, applied_at) VALUES (3, CURRENT_TIMESTAMP);
