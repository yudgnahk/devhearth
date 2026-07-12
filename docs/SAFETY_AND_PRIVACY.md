# Safety, Privacy, and Trust Model

Status: Draft  
Last updated: 2026-07-13

## Trust promise

The application may see more of a developer's filesystem than most tools. It must earn that access through minimal collection, transparent evidence, deterministic safety rules, local processing, and reversible operations.

## Safety boundary

The scanner observes. Detectors classify. Recommendation rules advise. Operation planners prepare changes. Only an explicit user approval may authorize a mutation.

No detector, AI model, imported policy, or remote response may directly execute an operation.

## Data classes

### Reproducible generated data

Examples include installed dependencies and build output with validated manifests and lockfiles. These may become candidates for low-risk hibernation.

### Downloadable data

Examples include package archives, public base models, SDKs, and container images. They may be recoverable but expensive, unavailable later, rate-limited, or needed offline. Classify at least medium risk unless stronger evidence applies.

### Persistent development data

Examples include database volumes, local object stores, emulator state, and message queues. Configuration does not prove the data is reproducible. Treat as high risk.

### User-created data

Examples include source changes, local commits, datasets, fine-tuned models, LoRA adapters, generated assets, signing material, and unpublished packages. Never infer disposability from age.

### Credentials and sensitive data

Examples include keychains, SSH keys, cloud credentials, `.env` files, tokens, cookies, and signing certificates. Do not inspect content for optimization. Never include values in logs or reports.

## Git safeguards

Before project hibernation or worktree removal, the engine must check:

- Working-tree and index state.
- Untracked files, subject to explicit ignore handling.
- Local branches and detached HEAD.
- Commits not reachable from locally known remote-tracking references.
- Submodules and nested repositories.
- Git LFS availability and pointer state when applicable.
- Worktree registration.

A lack of network access means remote backup status is unknown. The product must not claim that a commit is pushed unless verified against fresh remote state or another trusted backup source.

## Recommendation evidence

Every recommendation records:

- Detector and rule versions.
- Source scan and timestamp.
- Relevant paths or asset identifiers.
- Facts used by the rule.
- Confidence and uncertainty.
- Expected mutation scope.
- Restoration prerequisites.

For portfolio-fit recommendations, also record:

- Options ranked, including the stay-put baseline.
- Factor scores and which signals dominated.
- Explicit blockers (for example Yarn PnP, private registry layouts, missing lockfiles).
- That the ranking is comparative fit for the observed portfolio, not an absolute industry best tool.

AI-generated prose is visually distinguishable from deterministic evidence.

## Portfolio fit and tool preference safety

Fit scoring and preferred-tool policy guide structure; they do not authorize mutation and cannot weaken risk.

- A higher fit score for pnpm, uv, mise, or any other tool never implies that lockfile rewrites, installs, or deletions are safe.
- Fit scores cannot lower a deterministic risk class for migration, consolidation, or hibernation.
- Preferring a package manager in policy does not mark unknown project data as reproducible.
- “Stay with the current tool” must remain a first-class outcome when migration friction exceeds expected benefit.
- AI may explain a fit ranking in plain language but cannot change scores, blockers, or risk.
- Exported policies carry explicit preferred tools only by default; inferred portfolio fingerprints and project-level tool usage require opt-in and redaction.

## Mutation protocol

Future mutation support must use:

1. Preflight: Re-evaluate evidence and detect changes since the scan.
2. Plan: Enumerate exact operations and estimated impact.
3. Consent: Obtain user approval at the appropriate risk level.
4. Journal: Persist the operation plan before mutation.
5. Execute: Perform bounded, interrupt-aware steps.
6. Verify: Confirm preserved assets and measure results.
7. Finalize: Record outcome and restoration instructions.

If execution is interrupted, the journal must accurately identify completed and pending steps.

## Deletion behavior

- Prefer moving ordinary files to Trash when practical.
- Explain when an external tool performs irreversible deletion.
- Never silently empty Trash.
- Never recursively delete an unresolved symbolic-link target.
- Never cross a volume boundary unless the plan explicitly names it.
- Never operate on a path that changed identity between planning and execution.
- Treat sparse files, hard links, clones, packages, and mount points carefully.
- Refuse paths that resolve into protected system or credential locations.

## macOS permissions

- The product remains useful with user-selected folder access.
- Full Disk Access is optional and explained in context.
- Root access is not a normal requirement.
- A privileged helper is prohibited until a concrete feature requires it and receives a separate threat model.
- Security-scoped bookmarks are stored locally and are not exported in policies.

## AI policy

The core product must work without an LLM.

Permitted AI uses:

- Explain a recommendation in the user's terminology.
- Summarize a complex asset relationship graph.
- Help compare migration approaches.
- Generate non-executing restoration documentation.

Prohibited AI authority:

- Marking unknown data safe.
- Lowering risk.
- Changing portfolio fit scores, blockers, or ranked order.
- Selecting files for deletion without deterministic rules.
- Executing shell commands directly.
- Sending filenames, source, manifests, or inventory to a provider without explicit preview and consent.

Prefer on-device inference for explanations. If a remote provider is enabled, redact paths and secrets, minimize payloads, and display exactly what categories leave the device.

## Logging and telemetry

- Telemetry is disabled by default.
- Local logs use structured redaction.
- File contents and secret values are never logged.
- Paths in exported diagnostics are redacted unless the user opts in.
- Usage analytics, if introduced, require explicit opt-in and a documented event schema.
- Crash reports must be previewable before submission.

## Threats to consider

- Malicious repository names or manifests attempting command or UI injection.
- Symlink races and filesystem changes between scan and operation.
- Crafted Git repositories causing excessive work.
- Untrusted detector/plugin code inheriting Full Disk Access.
- Protocol injection through engine stdout.
- A replaced or tampered bundled engine executable.
- Reports leaking client names, paths, model usage, or repository metadata.
- Denial of service from huge trees, cycles, device files, or slow mounts.
- Imported policies containing dangerous paths or overly broad exclusions.

All paths and manifest values are untrusted input. Subprocess arguments must never be constructed through shell interpolation.

## Security testing requirements

- Fixture tests for symlinks, hard links, sparse files, mount boundaries, unreadable directories, and changing paths.
- Fuzz tests for manifest and protocol parsers.
- Tests proving secret redaction.
- Tests proving AI output cannot authorize an operation.
- Mutation failure-injection tests.
- Code-signing and protocol-authentication tests for packaged builds.
- External security review before shipping mutation features.

