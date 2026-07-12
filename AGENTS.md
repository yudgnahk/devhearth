# AGENTS.md

## Project

DevHearth is a privacy-first macOS application that scans developer storage, understands relationships among projects and their environments, and recommends structural optimizations.

It is not a generic disk or cache cleaner. Prefer reducing duplication and future growth through shared runtimes, shared dependency stores, ownership mapping, and safe project hibernation.

## Source of truth

Read the relevant document before changing behavior:

- `docs/SPECS.md` — product behavior and scope.
- `docs/ARCHITECTURE.md` — technology and component boundaries.
- `docs/SAFETY_AND_PRIVACY.md` — mandatory trust and mutation rules.
- `docs/ROADMAP.md` — sequencing and current scope.

If code and documentation disagree, surface the conflict. Do not silently reinterpret safety requirements.

## Technology boundaries

- Swift 6 and SwiftUI own the macOS UI, permissions, lifecycle, and native integrations.
- Go owns scanning, detection, analysis, recommendations, persistence, reports, and the CLI.
- SQLite stores the local inventory.
- Swift and Go initially communicate through versioned JSON-RPC with Go as a bundled child process.
- AI may explain deterministic findings but cannot make safety decisions.
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
