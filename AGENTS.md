# AGENTS.md

## Project

DevHearth is a privacy-first macOS application that scans developer storage, understands relationships among projects and their environments, and recommends structural optimizations.

It is not a generic disk or cache cleaner. Prefer reducing duplication and future growth through shared runtimes, shared dependency stores, tool-portfolio fit (version managers and package managers ranked with evidence—not a single absolute best tool), ownership mapping, and safe project hibernation.

## Source of truth

Read the relevant document before changing behavior:

- `docs/SPECS.md` — product behavior and scope.
- `docs/ARCHITECTURE.md` — technology and component boundaries.
- `docs/SAFETY_AND_PRIVACY.md` — mandatory trust and mutation rules.
- `docs/ROADMAP.md` — sequencing and current scope.

If code and documentation disagree, surface the conflict. Do not silently reinterpret safety requirements.

## Technology boundaries

- Swift 6 and SwiftUI own the macOS UI, permissions, lifecycle, and native integrations.
- Go owns scanning, detection, analysis (including tool portfolio and fit scoring), recommendations, persistence, reports, and the CLI.
- SQLite stores the local inventory.
- Swift and Go initially communicate through versioned JSON-RPC with Go as a bundled child process.
- AI may explain deterministic findings and fit rankings but cannot change scores, blockers, risk, or authorize operations.
- Do not introduce Rust without a reproducible benchmark showing Go cannot meet an important requirement.

Keep product logic in Go and platform/UI behavior in Swift. Do not duplicate recommendation or safety rules across languages.

## Non-negotiable safety

- The initial product is read-only. Scanners and detectors never mutate the filesystem.
- Every recommendation retains evidence, confidence, risk, estimated savings, and restoration assumptions.
- Treat paths, manifests, repository contents, protocol messages, and policies as untrusted input.
- Never inspect or log credential contents.
- Never claim Git data is pushed without fresh, trustworthy remote evidence.
- Do not follow symlinks by default, double-count hard links, or cross volume boundaries implicitly.
- Treat unknown persistent data, databases, container volumes, datasets, fine-tunes, and uncommitted or unpushed Git state as high risk.
- Do not assume Full Disk Access or root privileges.
- Future mutations require preflight, an exact plan, explicit consent, a journal, verification, and recovery information.

## Engineering expectations

- Keep filesystem concurrency bounded, scanning cancellable, memory bounded, and SQLite writes batched.
- Prefer measured optimization over language or dependency changes.
- Keep macOS-specific Go behavior behind platform interfaces.
- Never construct subprocess commands through shell interpolation.
- Version protocol and database schemas from the start.
- Add regression tests for behavior changes and filesystem edge cases.
- Update the relevant document when changing scope, architecture, safety, protocol behavior, or roadmap sequencing.
- Preserve unrelated work. Do not commit, push, publish, add telemetry, or add privileged helpers without explicit authorization.

When requirements are ambiguous, prefer read-only behavior and document the unresolved decision.

## Grok agent workflow

Use the local `grok` CLI as a bounded collaborator for implementation support and code review. Give it the repository path, the task scope, and the relevant safety constraints; its output and any edits remain untrusted until independently checked.

- Start with a read-only review before committing a non-trivial change. Ask Grok to read `AGENTS.md` and the relevant source-of-truth documents, inspect the current diff, and return severity-ranked findings with file and line references.
- For a read-only review, state `DO NOT edit files` in the prompt. Disable web search unless current external information is necessary. Do not allow Grok to commit, push, create pull requests, change credentials, or perform mutations outside the approved task scope.
- For a bounded implementation task, state exactly which files and behavior it may change, retain the project’s read-only filesystem safety boundary, and require tests. Review the resulting diff yourself; do not accept claims about tests, Git state, or remote state without fresh local verification.
- Resolve actionable Grok findings before handoff, or record why a finding is deferred. Include material findings and any remaining limitations in the pull request description.

Example read-only review:

```sh
grok --single "Review the current uncommitted diff. Read AGENTS.md and the relevant docs first. Do not edit files. Return severity-ranked findings with file and line references." --no-subagents --disable-web-search --max-turns 20
```

<!-- gitnexus:start -->
# GitNexus — Code Intelligence

This project is indexed by GitNexus as **dev-env-optimizer** (819 symbols, 1719 relationships, 25 execution flows). Use the GitNexus MCP tools to understand code, assess impact, and navigate safely.

> If any GitNexus tool warns the index is stale, run `npx gitnexus analyze` in terminal first.

## Always Do

- **MUST run impact analysis before editing any symbol.** Before modifying a function, class, or method, run `gitnexus_impact({target: "symbolName", direction: "upstream"})` and report the blast radius (direct callers, affected processes, risk level) to the user.
- **MUST run `gitnexus_detect_changes()` before committing** to verify your changes only affect expected symbols and execution flows.
- **MUST warn the user** if impact analysis returns HIGH or CRITICAL risk before proceeding with edits.
- When exploring unfamiliar code, use `gitnexus_query({query: "concept"})` to find execution flows instead of grepping. It returns process-grouped results ranked by relevance.
- When you need full context on a specific symbol — callers, callees, which execution flows it participates in — use `gitnexus_context({name: "symbolName"})`.

## When Debugging

1. `gitnexus_query({query: "<error or symptom>"})` — find execution flows related to the issue
2. `gitnexus_context({name: "<suspect function>"})` — see all callers, callees, and process participation
3. `READ gitnexus://repo/dev-env-optimizer/process/{processName}` — trace the full execution flow step by step
4. For regressions: `gitnexus_detect_changes({scope: "compare", base_ref: "main"})` — see what your branch changed

## When Refactoring

- **Renaming**: MUST use `gitnexus_rename({symbol_name: "old", new_name: "new", dry_run: true})` first. Review the preview — graph edits are safe, text_search edits need manual review. Then run with `dry_run: false`.
- **Extracting/Splitting**: MUST run `gitnexus_context({name: "target"})` to see all incoming/outgoing refs, then `gitnexus_impact({target: "target", direction: "upstream"})` to find all external callers before moving code.
- After any refactor: run `gitnexus_detect_changes({scope: "all"})` to verify only expected files changed.

## Never Do

- NEVER edit a function, class, or method without first running `gitnexus_impact` on it.
- NEVER ignore HIGH or CRITICAL risk warnings from impact analysis.
- NEVER rename symbols with find-and-replace — use `gitnexus_rename` which understands the call graph.
- NEVER commit changes without running `gitnexus_detect_changes()` to check affected scope.

## Tools Quick Reference

| Tool | When to use | Command |
|------|-------------|---------|
| `query` | Find code by concept | `gitnexus_query({query: "auth validation"})` |
| `context` | 360-degree view of one symbol | `gitnexus_context({name: "validateUser"})` |
| `impact` | Blast radius before editing | `gitnexus_impact({target: "X", direction: "upstream"})` |
| `detect_changes` | Pre-commit scope check | `gitnexus_detect_changes({scope: "staged"})` |
| `rename` | Safe multi-file rename | `gitnexus_rename({symbol_name: "old", new_name: "new", dry_run: true})` |
| `cypher` | Custom graph queries | `gitnexus_cypher({query: "MATCH ..."})` |

## Impact Risk Levels

| Depth | Meaning | Action |
|-------|---------|--------|
| d=1 | WILL BREAK — direct callers/importers | MUST update these |
| d=2 | LIKELY AFFECTED — indirect deps | Should test |
| d=3 | MAY NEED TESTING — transitive | Test if critical path |

## Resources

| Resource | Use for |
|----------|---------|
| `gitnexus://repo/dev-env-optimizer/context` | Codebase overview, check index freshness |
| `gitnexus://repo/dev-env-optimizer/clusters` | All functional areas |
| `gitnexus://repo/dev-env-optimizer/processes` | All execution flows |
| `gitnexus://repo/dev-env-optimizer/process/{name}` | Step-by-step execution trace |

## Self-Check Before Finishing

Before completing any code modification task, verify:
1. `gitnexus_impact` was run for all modified symbols
2. No HIGH/CRITICAL risk warnings were ignored
3. `gitnexus_detect_changes()` confirms changes match expected scope
4. All d=1 (WILL BREAK) dependents were updated

## Keeping the Index Fresh

After committing code changes, the GitNexus index becomes stale. Re-run analyze to update it:

```bash
npx gitnexus analyze
```

If the index previously included embeddings, preserve them by adding `--embeddings`:

```bash
npx gitnexus analyze --embeddings
```

To check whether embeddings exist, inspect `.gitnexus/meta.json` — the `stats.embeddings` field shows the count (0 means no embeddings). **Running analyze without `--embeddings` will delete any previously generated embeddings.**

> Claude Code users: A PostToolUse hook handles this automatically after `git commit` and `git merge`.

## CLI

| Task | Read this skill file |
|------|---------------------|
| Understand architecture / "How does X work?" | `.claude/skills/gitnexus/gitnexus-exploring/SKILL.md` |
| Blast radius / "What breaks if I change X?" | `.claude/skills/gitnexus/gitnexus-impact-analysis/SKILL.md` |
| Trace bugs / "Why is X failing?" | `.claude/skills/gitnexus/gitnexus-debugging/SKILL.md` |
| Rename / extract / split / refactor | `.claude/skills/gitnexus/gitnexus-refactoring/SKILL.md` |
| Tools, resources, schema reference | `.claude/skills/gitnexus/gitnexus-guide/SKILL.md` |
| Index, status, clean, wiki CLI commands | `.claude/skills/gitnexus/gitnexus-cli/SKILL.md` |

<!-- gitnexus:end -->
