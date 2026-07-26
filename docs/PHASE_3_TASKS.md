# Phase 3 task list — Optimization advisor

Phase 3 turns the Phase 2 asset graph into evidence-backed structural advice.
It stays read-only: the advisor proposes plans and non-executing commands, and
never mutates a scanned path.

## First slice (this phase entry)

- [x] Attribute allocated/logical sizes and last source activity onto assets
      without double-counting nested assets or shared stores (Phase 2 carry-over
      that savings estimates depend on).
- [x] Define the deterministic recommendation model: evidence, confidence,
      savings range, restoration cost, compatibility impact, risk class,
      preconditions, proposed actions, verification, and rollback.
- [x] Add a rule registry with deterministic ordering by value, risk, and
      confidence, and stable path-free recommendation IDs.
- [x] Implement multi-factor tool-portfolio fit scoring with an explicit
      stay-put baseline (deep for Node.js and Python; shallow elsewhere).
- [x] Rule: consolidate duplicate runtime / version-manager installations.
- [x] Rule: adopt a shared dependency store.
- [x] Rule: standardize package-manager portfolio fit (ranked, never a single
      absolute winner).
- [x] Rule: hibernation candidates for inactive reproducible projects.
- [x] Rule: obsolete Git worktree candidates, blocked pending Git verification.
- [x] Rule: duplicate downloadable AI models (base models only, never
      user-created fine-tunes).
- [x] Expose `fit.list`, `fit.get`, `recommendations.list`, and
      `recommendations.get` over JSON-RPC with path redaction.
- [x] Include recommendation and fit summaries in `report.export`.
- [x] Persist attributed sizes, fit assessments, and recommendations in SQLite
      (migration 005).
- [x] Show a recommendation inbox and fit-factor evidence views in the macOS
      preview UI.
- [x] Expand the synthetic fixture with duplicate version managers, mixed
      package managers, and duplicate model files.

## Remaining before the Phase 3 exit criterion

- [ ] Deep Git verification (working tree, unpushed commits) so hibernation and
      worktree recommendations can clear their blockers.
- [ ] Staged duplicate detection (size, metadata, sampled hash, full hash) so AI
      model and checkout duplication stops relying on name plus size alone.
- [ ] Installed-version comparison for runtime consolidation (match declared
      project requirements against installed versions).
- [ ] Later fit depth for Go, Rust, Swift/Xcode, and Java as fixtures and
      confidence allow.
- [ ] Rule: pin missing runtime versions, package managers, or lockfiles
      (SPECS §7.8 family 9).
- [ ] Rule: shared local service and unused architecture variant families.
- [ ] Deduplicate overlapping recommendations before any total is summed.
      Two rules can propose the same change from the same evidence: on the
      polyglot fixture, `adopt_shared_store` ("Share node dependencies through
      the pnpm store", 1024–3276 B) and `standardize_portfolio_fit`
      ("Standardize node on pnpm", 1024–3276 B) both describe adopting pnpm.
      Two effects, one cosmetic and one a defect:
      - The inbox shows two cards for one piece of work.
      - `report.export` sums bounds across recommendations, so those bytes are
        counted twice (fixture roll-up reports 2048–10648 where the pnpm
        migration contributes 1024–3276 once, not twice). **The report total
        overstates savings until this is resolved.**
      Rules are deliberately independent, so the fix does not belong inside a
      rule. It needs a model concept the graph does not have yet: either an
      overlap/supersedes relationship between recommendations, or a plan-level
      aggregation that groups by proposed action and affected assets before
      summing. Resolve this together with plan comparison below, since both
      need the same grouping.
- [ ] Plan comparison UI with before/after estimates.
- [ ] Policy-driven fit weights (waits on the Phase 4 policy format).

## Boundaries for this phase

- Savings are ranges with explicit confidence and uncertainty. Allocated bytes
  exclude hard-link aliases, so shared-store estimates are lower bounds and are
  marked uncertain rather than presented as exact.
- No recommendation may authorize mutation, and no explanation layer (including
  a future AI layer) may raise a savings figure or lower a risk class.
- A rule that cannot verify its preconditions emits a blocker instead of a
  higher confidence score.
- Fit scoring always ranks options and always includes stay-put; it never
  declares an absolute best tool.
