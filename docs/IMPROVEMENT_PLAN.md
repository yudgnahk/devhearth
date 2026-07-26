# DevHearth — Codebase Audit and Improvement Plans

Status: Draft
Last updated: 2026-07-26
Audit scope: full repository at branch `feature/phase-1-inventory-ui` (commit `bac9973` plus uncommitted work)

This document records a full-codebase investigation and three plans derived from
it: a refactoring plan for existing defects, an upgrade plan for product
capability, and a UI/UX redesign plan. It is an engineering working document,
not a product specification — [SPECS.md](SPECS.md) and
[ARCHITECTURE.md](ARCHITECTURE.md) remain authoritative for intent.

---

## 1. Executive summary

DevHearth is structurally sound. The read-only boundary is real and consistently
enforced, path redaction is applied at every protocol exit, the detector
interface is clean, and evidence is carried end-to-end from detector to UI. The
engine/UI process split is the right call and the JSON-RPC contract is honest
about versions.

The three highest-leverage problems are:

1. **Persistence is write-only.** Scans are saved to SQLite but never read back.
   The engine answers every query from an in-memory map that is never evicted,
   so memory grows with every scan and all history is lost on restart. Four
   store read methods exist and have zero callers.
2. **`EngineClient` is a 531-line God object** owning process lifecycle, RPC
   framing, scan state, and navigation state at once — with no request timeouts.
3. **No CI.** No workflow files, no race/lint/coverage gate, and seven detector
   packages have no tests at all.

The single most valuable change is making the scan lifecycle store-backed
(§3.2, R1). It fixes the memory issue and simultaneously unblocks scan history,
incremental scans, the compare UI, and the Phase 3 advisor.

### Verification performed

| Check | Result |
|---|---|
| `go vet ./...` | clean |
| `go test -race ./...` | 39 tests pass, 18 packages |
| `swift build --package-path apps/macos` | builds |
| CI configuration | **absent** (`.github/` does not exist) |
| `TODO`/`FIXME`/`HACK` markers in source | none |

---

## 2. Current state, as built

### 2.1 Runtime flow

1. The SwiftUI app spawns the Go engine as a child process and speaks
   newline-delimited JSON-RPC 2.0 over stdin/stdout
   (`EngineClient.swift:280-318`, `internal/protocol/server.go:62-95`).
2. `engine.hello` negotiates protocol and schema version and asserts
   `readOnly` (`server.go:110-120`); the client refuses to proceed on mismatch
   (`EngineClient.swift:85-89`).
3. `scan.start` walks each root single-threaded and recursively, metadata-only,
   never following symlinks, never crossing volume boundaries, recording
   inaccessible paths rather than failing, and de-duplicating hard-link
   allocation (`internal/scan/inventory.go:143-193`).
4. 13 registered detectors run over every inventory entry, producing
   path-keyed findings and links that are merged, de-duplicated, and
   materialized into a UUID-keyed asset graph
   (`internal/detect/runner.go:30-98`, `runner.go:189-290`).
5. `scan.BuildDirectoryIndex` rolls up per-directory totals bottom-up
   (`internal/scan/aggregate.go:26-89`).
6. Results persist to SQLite across four migrations, with bulk-tree interiors
   filtered out of durable inventory (`internal/store/store.go:101-132`).
7. The UI queries `assets.list`, `portfolio.list`, `inventory.children`, and
   `report.export` — all served from the in-memory `activeScan`.

### 2.2 Protocol surface

| Method | Purpose |
|---|---|
| `engine.hello` | version + read-only negotiation |
| `scan.start` / `scan.cancel` / `scan.status` | scan lifecycle |
| `scan.progress` (notification) | phase, entries, bytes, assets, persist rows |
| `inventory.children` | directory drill-down by `pathKey` |
| `assets.list` / `assets.get` | asset graph with evidence |
| `portfolio.list` | per-ecosystem tool summary |
| `report.export` | redacted JSON report |

### 2.3 What is genuinely good

- **Read-only discipline.** No mutation path exists anywhere in the engine.
  `ErrSymlinkRefused` (`internal/detect/helpers.go:13`) and bounded reads
  (`ReadFileLimited`) keep content access minimal and intentional.
- **Redaction at the boundary.** `redactPath`, `redactEvidenceValue`, and
  `redactAttributes` (`server.go:446-494`) mean absolute paths never leave the
  engine in reports; `pathKey` carries the absolute path only as an opaque
  round-trip handle.
- **Evidence provenance.** Every finding carries detector id, version,
  evidence kind, value, and confidence through to the UI.
- **Deterministic output.** Findings sorted by key, relationships sorted by
  kind/source, children sorted directories-first — reports diff cleanly.
- **Honest incompleteness.** `Inaccessible` is a first-class result field, and
  the docs' phase-status notes accurately reflect what is unfinished.

---

## 3. Plan 1 — Refactoring plan

### 3.1 Findings

#### C1 — Persistence is write-only; queries are served from unbounded memory

- `Server.scans` (`server.go:38`) holds, per completed scan, the entire
  `scan.Result` — one `Entry` per file visited — plus `graph`, `dirIndex`, and
  `pathIndex` (`server.go:43-50`). Nothing ever evicts it. Two scans of a large
  home directory hold millions of entries simultaneously.
- The store's read API is dead code: `ListAssets`, `ListRelationships`,
  `ListEvidence`, and `ListDirectoryChildren`
  (`store.go:386-538`) have no callers.
- Two disconnected identity systems: the protocol issues `scan_000001`
  (`server.go:126`); the store generates a UUID that `cmd/devhearth/main.go:60`
  discards. Nothing can reload a past scan.
- Consequence: all inventory is lost on app restart despite being on disk, and
  the database grows without bound or pruning.

#### C2 — Cancellation and error reporting gaps

- `cancellationToken` is accepted by the protocol and never used
  (`internal/protocol/types.go:51`).
- Persistence runs on `context.Background()` (`server.go:313`), so Cancel
  cannot interrupt a long save.
- Six `_ = s.write(...)` calls in `runScan` discard every progress-notification
  error, so a broken pipe is invisible.
- Detector errors are dropped with only a comment and no log
  (`runner.go:71-74`), violating the project's own "never swallow errors" rule.

#### C3 — Store durability and correctness

- `SaveWithOptions` sets `PRAGMA synchronous = OFF` with the error ignored
  (`store.go:147-153`). A crash mid-save can corrupt the inventory database.
  WAL plus `synchronous = NORMAL` is nearly as fast and crash-safe.
- `migrate()` identifies a fresh database by string-matching `"no such table"`
  (`store.go:73`) and depends on each migration file inserting its own
  `schema_migrations` row.
- `bulkInteriorMarkers` contains `/build/` and `/dist/` (`store.go:117-118`),
  so any legitimate source directory with those names is silently dropped from
  durable inventory.
- Volume resolution scans all entries per root — O(roots × entries)
  (`store.go:165-171`).

#### C4 — `EngineClient` is a God object

`apps/macos/Sources/DevHearthApp/EngineClient.swift` (531 lines) owns engine
discovery, process lifecycle, RPC framing, the pending-request map, scan state,
navigation state, and status strings.

- **No request timeouts** — a hung engine leaves a continuation pending forever
  (`EngineClient.swift:330-343`).
- One malformed line fails *every* pending request (`handleLine`, line 354).
- `stop()` calls `process.waitUntilExit()` on the MainActor (line 240).
- `progress` grows unbounded (line 378) though the UI reads only `.last`.
- Protocol models are hand-duplicated from Go with hand-pinned version
  constants (`ProtocolModels.swift:3-4`).

#### C5 — Protocol server shape and detector dispatch

- `handle()` is a ~150-line switch repeating decode → lookup → status-check →
  respond eight times (`server.go:104-252`).
- `Descriptor.Triggers` is designed as a dispatch fast path but the runner
  ignores it, calling every detector's `Match` on every entry
  (`runner.go:66`) — O(entries × detectors).
- The walker descends fully into `node_modules`, `.git`, and `target` trees
  even though they are pruned at persist time, so the traversal and memory cost
  is paid for data that is then discarded.

#### C6 — No safety nets

No `.github/workflows`. `make check` runs `vet` + tests + build but no `-race`,
no linter, no coverage threshold, no `gosec`. Detector packages `docker`,
`java`, `golangmod`, `swiftdetect`, `runtime`, `ai`, and `builtin` have no
tests. Swift tests cover model decoding only.

### 3.2 Refactoring roadmap

#### R0 — Safety nets first (before touching production code)

1. GitHub Actions on macOS runner: `go vet`, `go test -race -cover ./...`,
   `golangci-lint`, `gosec`, `swift build`, `swift test`.
2. Table-driven tests for every untested detector — they are pure functions
   over `Candidate`, so this is cheap and high-yield.
3. A shared protocol golden-file fixture set consumed by both the Go and Swift
   test suites, so wire-format drift fails CI.

#### R1 — Unify the scan lifecycle around the store *(highest leverage)*

1. Introduce a `ScanManager` owning one scan identity end-to-end: a single
   UUID used by both the protocol and the database.
2. Serve `assets.list`, `inventory.children`, `portfolio.list`, and
   `report.export` from SQLite via the existing (currently dead) read methods.
   Keep only the *active* scan hot in memory.
3. Evict completed scans from memory after persist. Add `scans.list` for
   history and a retention policy (keep last N).
4. Thread a per-scan `context.Context` through persistence — use
   `context.WithTimeout` rather than `context.Background()`. Honour
   `cancellationToken` or delete it from the protocol.
5. Replace the `handle()` switch with `map[string]handlerFunc` plus shared
   decode/validate/lookup middleware.
6. Log detector and progress-write failures through the existing `slog` logger.

#### R2 — Store hardening

1. `synchronous = NORMAL` with WAL; check PRAGMA errors; wrap all errors with
   `%w` context.
2. Explicit `schema_migrations` creation, version list in code, one
   transaction per migration.
3. Fix `/build/` and `/dist/` false positives using sibling-manifest evidence;
   move the marker list into configurable policy rather than a package var.
4. Precompute a root→device map once — O(roots + entries).

#### R3 — Scan and detection performance

1. Prune at walk time: skip descending into bulk trees while still stat-ing the
   marker directory for size. Largest single memory and wall-clock win.
2. Build a trigger index (`map[filename][]Detector`) from `Descriptor.Triggers`
   so only relevant detectors run per entry.
3. Optional bounded worker pool for traversal (`errgroup` + context) keeping
   single-writer aggregation.
4. Stream entries to the store in batches during the walk instead of
   accumulating all of `Result.Entries`, removing the O(files) memory floor.

#### R4 — Swift client decomposition

1. `EngineProcess` — discovery, spawn, async terminate (no `waitUntilExit` on
   the MainActor).
2. `JSONRPCConnection` — framing, pending map, **per-request timeout**, and
   per-line error isolation so one bad line fails one request.
3. `ScanStore` (`@Observable`) for scan/asset/portfolio state;
   `DirectoryBrowserModel` for navigation state.
4. Cap `progress` to the latest event or a small ring buffer.
5. Unit-test `JSONRPCConnection` against the existing `--mock-scan` engine mode.

#### R5 — One source of truth for the protocol

Define the wire schema once (JSON Schema or a small IDL), generate or
CI-verify both the Go structs and the Swift `Codable` types, and derive
`protocolVersion`/`schemaVersion` from it.

**Suggested order:** R0 → R1 → R2 → R4 → R3 → R5. Each is independently
shippable and test-first friendly.

---

## 4. Plan 2 — Upgrade plan

### U1 — Close out Phase 1 and Phase 2 commitments

| Item | Notes |
|---|---|
| Scan history and reload | Direct payoff of R1; enables "what grew since last week?" |
| Resumable checkpoints, pause/resume | Open Phase 1 task; persist the walk frontier per volume |
| Signed `.app` with security-scoped bookmarks | Open Phase 1 task; today it is an SPM binary plus `DEVHEARTH_ENGINE_PATH`. Needs an Xcode/xcodegen project, codesign, notarization, engine bundled as an auxiliary executable |
| ~~**Size attribution onto assets**~~ | Done in `internal/attribute`: subtree totals from the inventory rollup, exclusive bytes that subtract nested assets, shared-store and hard-link uncertainty flags, plus source activity that skips generated trees and `.git` |
| Deep Git status | Guarded `git` subprocess behind an interface (dirty tree, unpushed commits); feeds hibernation safety |
| Homebrew / Xcode / SDKMAN / Docker Desktop data roots | Beyond directory-name signatures; these are the largest real consumers on developer Macs |

### U2 — Begin Phase 3, the optimization advisor

Status: started. The framework, fit scoring, six rule families, `fit.*` and
`recommendations.*` methods, persistence, and the inbox UI are in place; see
[the Phase 3 task list](PHASE_3_TASKS.md). Duplicate-checkout detection by
remote and stale download caches are not yet rules, and both hibernation and
worktree advice stay blocked until deep Git verification lands.

1. Deterministic rule framework:
   `Rule(graph, sizes) -> []Recommendation{savingsRange, confidence, evidence, restorationCost, blockers, risk}`
   — pure functions, table-tested, no mutation.
2. First rules, all computable once size attribution lands:
   - Reproducible build output and `node_modules` hibernation candidates
     (lockfile present, clean git, untouched for N days).
   - Duplicate runtime and version-manager detection (nvm + mise + Volta all
     present).
   - Duplicate checkouts of the same repository (same remote across paths).
   - Stale download caches per ecosystem.
3. A `recommendations.list` method plus persistence, carrying an explicit
   advice-only flag to preserve the trust model.

### U3 — Engine and platform

- **Incremental scans.** `ModifiedAt` is already captured; re-stat only changed
  subtrees, then FSEvents invalidation. Compounds every other feature.
- **Structured diagnostics.** A `scan.diagnostics` method (per-phase timings,
  slowest directories, inaccessible summary) surfaced in the UI.
- **Benchmarks in CI** using the polyglot fixture — a benchmark test already
  exists in `internal/scan`; wire it up with thresholds.
- **Policy file** (`~/.config/devhearth/policy.json`): excluded paths, prune
  markers, retention. Replaces today's hardcoded lists and aligns with the
  Phase 4 portable-policy goal.

### U4 — Distribution and lifecycle

- Notarized DMG; Sparkle updates; automatic engine respawn and re-hello on
  `engineExited`.
- Menu-bar companion showing the last scan summary plus "Scan now" — cheap once
  scan history exists.

**Sequencing:** U1 (size attribution and history first) → U2 rules → U3
incremental scanning → U4 packaging. All read-only, consistent with
[SAFETY_AND_PRIVACY.md](SAFETY_AND_PRIVACY.md).

---

## 5. Plan 3 — UI/UX redesign

### 5.1 Current problems

All verified in `apps/macos/Sources/DevHearthApp/DevHearthApp.swift`:

- The sidebar is a kitchen sink: title, four buttons, progress, engine path,
  two error labels, scan overview, portfolio, and the asset list in one
  `VStack` (lines 34-115).
- The progress bar is **indeterminate** (line 53) even though the engine
  streams `entriesVisited`, `allocatedBytes`, and `rowsWritten`/`rowsTotal`.
  Real numbers arrive and are discarded.
- Selecting an asset does not switch the detail pane to the Assets tab, so a
  click appears to do nothing until the segmented picker is also clicked.
- The drill-down `Table` has no column sorting, no size bars, no context menu,
  and no Reveal in Finder despite `pathKey` carrying the absolute path.
- No menu-bar commands, no keyboard shortcuts, no Settings scene, no
  onboarding, errors as small red captions, blocking `NSSavePanel.runModal()`
  on export, no accessibility labels, no localization scaffolding.

### 5.2 Target information architecture

```
┌───────────┬──────────────────────────────────────────────┐
│ SIDEBAR   │  TOOLBAR: [Scan ▾]  [breadcrumb]  [search]   │
│ Overview  ├──────────────────────────────────────────────┤
│ Storage   │                                              │
│ Assets    │              CONTENT AREA                    │
│ Portfolio │                                              │
│ Advisor*  │                        ┌───────────────────┐ │
│ History   │                        │ INSPECTOR (⌥⌘I)   │ │
│ Reports   │                        │ evidence, sizes   │ │
└───────────┴────────────────────────┴───────────────────┘
                                       * Phase 3
```

- `NavigationSplitView` with real sidebar sections. The scan action moves to
  the toolbar as the primary action and to **File ▸ Scan Folder… (⇧⌘O)**.
  Settings under `⌘,`.
- A macOS 14 `.inspector` replaces the segmented picker, so selecting an asset
  or folder anywhere reveals its detail and evidence without tab juggling.

### 5.3 Screens

1. **Onboarding (first run).** What DevHearth does, the read-only promise —
   the core differentiator, so state it — the folder-grant flow with
   security-scoped bookmarks, and suggested roots (`~/Projects`,
   `~/Developer`).
2. **Overview dashboard.** Hero total-allocated stat, bytes-by-asset-kind
   donut, top-10-largest-directories bar list, ecosystem cards
   ("Node · 23 projects · pnpm dominant"), inaccessible-paths callout. Empty
   state is a single friendly "Scan a folder" card.
3. **Storage explorer.** Sortable columns via `Table` `sortOrder`, inline
   proportional size bars, a breadcrumb path control, `⌘↑` to navigate up, a
   context menu (Reveal in Finder, Copy path, Copy redacted path), and a
   **treemap toggle** for the current directory drawn with `Canvas` — the
   aggregates already carry every number it needs.
4. **Assets.** Filter chips by kind, ecosystem, and risk plus `.searchable`;
   risk as a coloured badge; a relationship mini-graph in the inspector
   ("this `node_modules` → owned by → project X") replacing raw
   "kind · confidence 90%" strings.
5. **Portfolio.** Per-ecosystem cards with SF Symbols, version-manager chips,
   dominant-tool highlight; clicking filters the Assets view.
6. **History.** Past scans (needs R1) with per-scan totals and a compare view
   showing Δ bytes by directory.
7. **Reports.** Preview the redacted JSON in a sheet *before* saving — this is
   how redaction earns trust — using async `fileExporter` rather than a modal
   `NSSavePanel`.

### 5.4 Scan progress experience

- Determinate and phase-aware: *Scanning 48,211 items · 31 GB* → *Detecting
  assets… 112 found* → *Saving 12,400/61,000 rows*. Every number already
  arrives over the wire.
- Progress as a compact toolbar pill; cancellation is immediate and explained
  ("Cancelled — partial results kept").
- Completion: inline banner ("Scan complete · 84 assets · 31 GB") plus a user
  notification when the window is in the background.

### 5.5 System and polish

- **States.** Explicit empty, loading, and error designs on every screen.
  Engine health becomes a status dot with a popover (path, version, restart)
  instead of a raw path string in the sidebar.
- **Design system.** Semantic colours only (dark mode for free), SF Symbols per
  asset kind, monospaced digits for all sizes, one shared byte formatter, a
  4-pt spacing grid.
- **Accessibility and i18n.** Labels on every control, a VoiceOver rotor for
  the table, all strings through `String(localized:)`.
- **Keyboard.** `⇧⌘O` scan, `⌘E` export, `⌘1`–`⌘5` sections, `⌘F` search,
  `⌥⌘I` inspector.

### 5.6 Delivery order

1. Structure — sidebar IA, toolbar, inspector. Unblocks everything else.
2. Determinate progress and state/error surfaces. Best quality-per-effort.
3. Storage explorer upgrades — sorting, size bars, Finder integration.
4. Overview dashboard and portfolio cards.
5. Treemap, history/compare, export preview.

---

## 6. Recommended overall sequence

```
R0 (CI + detector tests)
  └─> R1 (store-backed scan lifecycle)  ← highest leverage in this document
        ├─> R2 (store hardening)
        ├─> UI 5.2/5.3 (sidebar IA + inspector)
        └─> U1 (size attribution + scan history)
              ├─> UI dashboard + history/compare
              └─> U2 (Phase 3 advisor rules)
R4 (Swift decomposition) and R3 (scan performance) run in parallel after R2.
```

R1 is the keystone: it resolves the unbounded-memory defect and is a hard
prerequisite for scan history, incremental scanning, the compare UI, and the
advisor.

---

## 7. Appendix — untested packages

Packages with no test file, all pure-function detectors that are cheap to
cover:

- `internal/detect/docker`
- `internal/detect/java`
- `internal/detect/golangmod`
- `internal/detect/swiftdetect`
- `internal/detect/runtime`
- `internal/detect/ai`
- `internal/detect/builtin`

Packages with tests: `internal/detect` (merge, registry, runner, helpers),
`internal/detect/node`, `python`, `rust`, `git`, `terraform`,
`internal/scan` (inventory, aggregate, benchmark), `internal/assets`
(portfolio), `internal/store`, `internal/protocol`, plus Swift
`ProtocolModelsTests`.
