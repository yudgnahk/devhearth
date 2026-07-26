# Phase 4 task list — Incremental monitoring and portable policy

Phase 4 makes DevHearth answer "what changed?" and lets a user carry their
preferences to a second Mac without carrying their inventory with them. It stays
read-only: nothing here mutates a scanned path, and a policy can hide advice but
can never raise a savings figure, lower a risk class, or authorize an operation.

## First slice (this phase entry)

- [x] Define the portable policy document (`internal/policy`): scan roots as
      portable aliases, exclusion patterns, preferred tools, fit weighting mode
      and per-factor overrides, risk display threshold, activity windows,
      retention, suppressions, and machine-class overlays.
- [x] Make portability structural rather than procedural: a `Document` has no
      field that can hold an absolute path, so an export cannot leak one even if
      a future writer forgets to redact.
- [x] Validate untrusted policy files at the boundary: unknown schema versions,
      unknown fields, absolute or escaping roots, absolute exclusions, invalid
      globs, out-of-range weights and windows, blanket suppressions, control
      characters, and oversized documents are refused rather than repaired.
- [x] Resolve per-machine overlays, least specific first, so one shared policy
      behaves sensibly on a 256 GB laptop and a workstation.
- [x] Policy-driven fit weights with named tradeoffs (`balanced`,
      `prefer_disk_savings`, `prefer_workflow_stability`) plus per-factor
      overrides, renormalized before scoring (Phase 3 carry-over).
- [x] Recommendation suppression: hide one recommendation or a whole family,
      optionally per ecosystem, recorded in the portable policy.
- [x] Recommendation feedback (accepted / rejected / unclear / later) stored
      locally with no export path.
- [x] Growth trends (`internal/trend`): path-free per-scan snapshots, series per
      ecosystem and storage class, deltas, rates, and direction.
- [x] Regression detection: sudden growth (absolute *and* proportional),
      sustained growth across consecutive scans, and rising recoverable storage
      measured on the conservative savings bound.
- [x] Compare only snapshots that share a scan scope, so a narrow scan followed
      by a wide one cannot report growth that never happened.
- [x] Expose `policy.get`, `policy.set`, `policy.export`, `policy.import`,
      `recommendations.suppress`, `recommendations.feedback`, and `trends.list`
      over JSON-RPC; extend `recommendations.list` with an `include` filter and
      withheld counts.
- [x] Let `scan.start` resolve the policy's portable roots on the engine side
      (`usePolicyRoots`), so absolute paths never cross the protocol boundary.
- [x] Persist policies, feedback, and snapshots in SQLite (migration 006), with
      snapshots written inside the scan transaction and retention pruning.
- [x] Trends and Policy views in the macOS preview UI, plus suppress and
      feedback controls in the recommendation inbox.
- [x] App-owned text scaling with View-menu zoom commands, because macOS gives a
      SwiftUI window no zoom of its own and the default ramp is unreadable on a
      large external display.

## Remaining before the Phase 4 exit criterion

- [ ] FSEvents-based invalidation so a rescan only revisits what changed. This
      is the Swift-side platform work described in ARCHITECTURE ("macOS-specific
      helper boundary") and is the main reason a scan is still a full walk.
- [ ] Scheduled low-impact scans, including a policy for when *not* to run (on
      battery, under thermal pressure, during a build).
- [ ] Notifications for meaningful changes only, driven by the regression
      signals rather than by every completed scan.
- [ ] Policy-driven exclusions applied during traversal. `Effective.ExcludesPath`
      exists and is tested, but `scan.Inventory` does not consult it yet, so an
      exclusion currently narrows nothing.
- [ ] Trend history for suppressed advice: a family a user hid should still be
      tracked, so "you hid this and it has since tripled" is answerable.
- [ ] Per-machine overlay editing in the UI. Overlays are parsed, resolved,
      exported, and explained, but can only be authored by editing a file.
- [ ] Retention beyond snapshot count: a time-based window, and pruning of the
      inventory rows that back retired scans.
- [ ] A second-Mac integration test that exports on one database, imports on a
      fresh one, and asserts the resulting advice matches under the same fixture.
- [ ] `EnsureActivePolicy` has a first-run race: two engine processes opening the
      same database with no stored policy can both try to insert an active row,
      and the partial unique index makes the loser fail rather than re-read. The
      app runs one engine, so this is currently unreachable; fix it before any
      second consumer of the database exists.
- [ ] Policy-driven trend thresholds. `trend.Options` is tunable but the engine
      always passes defaults, so a user cannot say "1 GiB is noise on this
      machine."

## Review findings addressed in this slice

A read-only review (per AGENTS.md) surfaced these; each is fixed and covered by
a regression test rather than deferred:

- Suppressing or re-configuring a policy did not affect a scan already held in
  memory, so hiding a recommendation appeared to do nothing until the next scan.
  Held scans are now re-partitioned when a policy is saved, without re-running
  the rules.
- Report and trend savings summed only visible advice, so hiding a
  recommendation made recoverable storage drop as if the work had been done.
  Totals now cover the full rule output, with the withheld portion reported
  alongside.
- `fitWeights` could set migration friction to zero, letting a policy rank a
  disruptive migration as if it were free. A floor is now enforced before
  normalization.
- `policy.set` accepted unknown fields that `policy.import` rejected. Both paths
  now use the same parser.
- Exclusion patterns containing `..` were accepted, describing something outside
  the scanned tree. They are now refused, matching the rule already applied to
  roots.
- Risk-hidden advice was countable but not listable. `include: "all"` now
  returns it flagged with `hiddenByRisk`.
- The export panel claimed a policy carries no repository names. A stored root
  does keep its folder names below the alias, and the export notes now say so.

## Boundaries for this phase

- A policy may change what is shown and how options are weighted. It may never
  change a savings figure, a confidence, a risk class, or a blocker. Suppression
  moves advice out of the inbox and records that it did so; the withheld counts
  travel with every inbox response and with `report.export`.
- Feedback is a usage trace and has no export path. A user who wants a decision
  to travel records a suppression, which is path-free by construction.
- Trend snapshots hold counts and byte totals per named series. They never hold
  paths, so history cannot become a second copy of the inventory.
- A scan scope is fingerprinted with a one-way digest of its roots. The digest
  stays in the local database: it answers "is this the same scope?" and is never
  exported.
- Growth is reported between completed scans only. Cancelled and failed scans
  are not recorded, because a partial measurement compared against a full one
  would read as storage disappearing.
