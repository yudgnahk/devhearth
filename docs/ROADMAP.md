# Roadmap

This roadmap prioritizes trustworthy understanding before automation.

## Phase 0 — Foundations

- Finalize product vocabulary and scope.
- Choose a project name.
- Create the GitHub repository and licenses.
- Establish Go and Swift directory structure.
- Define JSON-RPC handshake and schema versioning.
- Define the SQLite schema and detector interface.
- Build representative synthetic filesystem fixtures.
- Establish cold and incremental scan benchmarks.

Exit criteria: Swift launches the bundled Go engine, negotiates a protocol version, and displays a mocked scan stream.

## Phase 1 — Read-only filesystem inventory

- Folder selection and permission onboarding.
- Bounded Go filesystem traversal.
- Logical and allocated-size accounting.
- Hard-link handling and safe symlink behavior.
- SQLite scan persistence.
- Cancellation and progress reporting.
- Overview UI and directory drill-down.
- JSON report export.

Exit criteria: The app can scan selected developer roots reliably and explain inaccessible or uncertain storage.

## Phase 2 — Development asset graph

- Git repository and worktree detection.
- Node, Python, Go, Rust, Swift, Java, and Terraform detectors.
- Runtime-manager inventory.
- Docker/Compose and local VM-runtime inventory.
- Initial AI model and dataset detectors.
- Asset relationship UI.
- Detector evidence inspector.

Exit criteria: Most storage within representative developer roots is attributed to projects, runtimes, dependencies, containers, AI assets, or unknown data.

## Phase 3 — Optimization advisor

- Deterministic recommendation framework.
- Duplicate runtime analysis.
- pnpm migration suitability assessment.
- uv migration suitability assessment.
- Hibernation candidate assessment.
- Duplicate Git checkout and worktree analysis.
- Downloadable AI model duplication analysis.
- Savings ranges, confidence, restoration costs, and risk ratings.
- Plan comparison UI.

Exit criteria: The product provides useful, evidence-backed structural recommendations without modifying the filesystem.

## Phase 4 — Incremental monitoring and portable policy

- FSEvents-based invalidation.
- Scheduled low-impact scans.
- Growth trends and regression detection.
- Portable redacted policy format.
- Per-machine policy overlays.
- Recommendation suppression and feedback.
- Notifications for meaningful changes only.

Exit criteria: A policy can be reused on a second Mac without exposing the first Mac's private inventory.

## Phase 5 — Guided operations

- Hibernation plan export.
- Dry-run execution framework.
- Operation journal and recovery.
- Native Trash integration.
- Project hibernation and restoration for the safest supported ecosystems.
- Runtime consolidation assistant.
- Post-operation verification.

Exit criteria: Supported low-risk operations survive failure injection and restore successfully in integration tests.

## Phase 6 — AI-assisted explanation

- Local explanation model evaluation.
- Optional remote-provider abstraction.
- Redaction and payload preview.
- Natural-language graph exploration.
- Personalized recommendation explanations.

Exit criteria: The product remains fully functional without AI, and AI cannot influence deterministic safety decisions.

## Not scheduled until evidence supports them

- Third-party executable plugins.
- Privileged helper.
- Mac App Store distribution.
- Linux or Windows GUI.
- Cloud inventory synchronization.
- Team fleet enforcement.
- Automatic unattended optimization.
- Rust components.

## Early validation plan

Before building mutation features, test the read-only prototype with at least these profiles:

- iOS developer with multiple Xcode versions and simulators.
- Node/TypeScript developer with dozens of repositories.
- Python/ML developer with Conda, venv, uv, and large model stores.
- Go/Rust backend developer using containers.
- AI-heavy developer using multiple editors, worktrees, local models, and vector databases.
- Intel Mac and Apple Silicon Mac where available.
- 256 GB machine under storage pressure and a larger workstation.

Collect which recommendations users trust, reject, or cannot understand. Do not use raw file inventories as telemetry.

