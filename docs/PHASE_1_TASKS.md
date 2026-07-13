# Phase 1 task list — Read-only filesystem inventory

- [x] Create a selected-folder scan entry point in the macOS preview UI.
- [x] Implement metadata-only Go traversal without file-content reads.
- [x] Keep traversal bounded and cancellable; do not cross filesystem-volume boundaries implicitly.
- [x] Record logical and allocated sizes from filesystem metadata.
- [x] Do not follow symbolic links; avoid double-counting allocated bytes for hard links.
- [x] Represent inaccessible paths in scan results rather than failing the whole scan.
- [x] Persist scans and filesystem metadata locally in SQLite with schema migration coverage.
- [x] Add protocol support for scan start, cancellation, status, progress, and redacted JSON summary export.
- [x] Display live scan progress and a cancellation control in the macOS preview UI.
- [x] Add a directory drill-down backed by persisted aggregates.
- [x] Add a user-directed JSON file export flow and default path redaction.
- [ ] Add resumable scan checkpoints and pause/resume.
- [ ] Validate the folder access flow in a signed Xcode app bundle with security-scoped bookmarks.

This first Phase 1 slice deliberately retains the read-only boundary: it reads
filesystem metadata only and writes only the application's local SQLite
inventory. It does not inspect file contents, follow symlinks, cross volumes,
or modify scanned paths.
