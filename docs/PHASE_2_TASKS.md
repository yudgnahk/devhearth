# Phase 2 task list — Development asset graph

- [x] Define concrete asset, relationship, evidence, and portfolio summary models.
- [x] Extend the detector interface with match heuristics and path-keyed findings.
- [x] Implement Git repository and linked-worktree detection (metadata + small `.git` pointer reads).
- [x] Implement project detectors for Node, Python, Go, Rust, Swift, Java, and Terraform.
- [x] Implement Docker/Compose resource detection.
- [x] Implement well-known runtime/version-manager and shared-store/cache path signatures.
- [x] Implement initial AI model/store path signatures.
- [x] Run detectors after metadata inventory with progress (`detection` phase).
- [x] Persist assets, locations, relationships, and evidence in SQLite (migration 003).
- [x] Expose `assets.list`, `assets.get`, and `portfolio.list` over JSON-RPC with path redaction.
- [x] Include asset counts and portfolio summaries in `report.export`.
- [x] Show portfolio and asset list with evidence inspector in the macOS preview UI.
- [x] Expand the synthetic polyglot fixture for multi-ecosystem coverage.
- [x] Attribute allocated sizes onto assets without double-counting shared stores.
      (Delivered with the Phase 3 advisor slice, which needed it for savings
      estimates: see `internal/attribute` and [the Phase 3 task list](PHASE_3_TASKS.md).)
- [ ] Deep Git status (working tree, unpushed commits) via guarded `git` interface.
- [ ] Homebrew / Xcode / SDKMAN inventory beyond directory-name signatures.
- [ ] Container runtime image/volume inventory (Docker Desktop data roots).
- [ ] Richer relationship graph visualization (beyond list + detail).
- [x] Fit scoring moved to Phase 3 as planned and its first slice is implemented
      there (`internal/portfolio`); nothing about it remains open in Phase 2.

This Phase 2 slice remains read-only: detectors may read small manifests and
Git pointer files, but never mutate scanned paths. Recommendations and
mutations stay out of scope.
