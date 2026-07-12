ALTER TABLE filesystem_entries ADD COLUMN modified_at TEXT;
ALTER TABLE filesystem_entries ADD COLUMN hard_link_alias INTEGER NOT NULL DEFAULT 0 CHECK (hard_link_alias IN (0, 1));

INSERT INTO schema_migrations(version, applied_at) VALUES (2, CURRENT_TIMESTAMP);
