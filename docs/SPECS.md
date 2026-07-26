# Local Development Environment Optimizer — Product Specification

Status: Draft 0.2  
Last updated: 2026-07-13

## 1. Summary

The product is a privacy-first macOS application that scans developer storage, understands relationships between projects and their environments, and guides users toward a smaller, simpler, reproducible local development system.

It is not a cache cleaner. Cleanup tools enumerate known cache paths; DevHearth inventories tool portfolios—version managers, package managers, and dependency stores—builds an ownership graph, and recommends structural optimizations: consolidating duplicated runtimes, adopting shared dependency stores, standardizing package-manager fit, hibernating inactive projects safely, identifying redundant container and AI assets, and preventing new duplication.

## 2. Problem

Modern developers create many short-lived and heterogeneous projects. Coding agents amplify this behavior by generating clones, worktrees, experiments, environments, models, datasets, containers, and build artifacts quickly. Existing disk analyzers can show large directories, while cleanup applications remove known files, but neither answers:

- Which project or tool owns this data?
- Is it reproducible?
- Is another copy already installed elsewhere?
- Can several projects share it?
- Which package managers and version managers are installed, which are actually used, and which portfolio shape fits this machine?
- What would break if it were changed?
- How much space would an optimization actually save?
- How can the project be restored later?

## 3. Product principles

1. Evidence before advice. Every recommendation must show the facts that produced it.
2. Guidance before mutation. Scanning and planning are the default experience.
3. Reversible where possible. Prefer Trash, quarantine, snapshots, or recorded restoration steps.
4. Deterministic safety. An LLM may explain a recommendation but cannot classify an operation as safe or authorize it.
5. Local first. Inventory and analysis stay on the device by default.
6. Optimize systems, not just files. Prefer consolidation and reproducibility over periodic deletion.
7. Respect ownership. Unknown, uncommitted, unpushed, generated-by-user, and credential-bearing data receive conservative treatment.
8. Cross-device policy. A user can export non-sensitive preferences and apply them to another Mac.

## 4. Target users

### Primary

- Polyglot macOS developers with many local repositories.
- Developers using AI coding tools to create frequent experiments or worktrees.
- Developers working on constrained 256 GB or 512 GB Macs.
- Developers using combinations of Xcode, containers, Node.js, Python, Go, Rust, Java, and local AI models.

### Secondary

- Teams that want a common local-environment policy.
- Consultants switching among many client stacks.
- Developers managing multiple Macs.

## 5. Product vocabulary

- Asset: A disk-backed item relevant to development, such as a project, runtime, dependency store, model, image, volume, SDK, or build output.
- Relationship: Evidence that one asset uses, owns, generates, duplicates, or can restore another asset.
- Version manager: A tool that installs and switches language or platform runtimes (for example mise, asdf, nvm, fnm, pyenv, rustup, SDKMAN).
- Package manager: A tool that resolves and installs project dependencies (for example npm, pnpm, Cargo, uv, Gradle).
- Dependency store: A shared or content-addressed location that multiple projects can reuse (for example a pnpm store, Yarn Berry cache, Cargo registry, Go module cache).
- Download cache: Regenerable package or artifact cache that is not a project-local install (for example npm `_cacache`, pip cache).
- Tool portfolio: The set of version managers, package managers, stores, and caches present on a machine, plus how projects actually use them.
- Fit score: A multi-factor, evidence-backed ranking of portfolio options for an ecosystem or project set—not a single absolute “best tool.”
- Reproducible: Restorable from source, a lockfile, a trusted registry, and documented commands without relying on unique local state.
- Optimization: A structural change that reduces disk use or future growth while preserving intended capability.
- Hibernation: Removal of reproducible project-local state after recording and validating a restoration plan.
- Recommendation: An evidence-backed proposed optimization with savings, cost, risk, and rollback information.
- Policy: Versioned user preferences covering scan roots, exclusions, risk tolerance, retention, and preferred tools.

## 6. Core user journey

1. The user grants access to selected folders or Full Disk Access.
2. The application performs a read-only scan and shows progress without blocking other work.
3. The application detects development assets and builds an ownership/usage graph.
4. The user sees disk usage grouped by projects, ecosystems, runtimes, package managers, dependency stores, containers, AI assets, and recoverability.
5. The application proposes ranked optimizations, including portfolio-fit advice when evidence supports it.
6. The user opens a recommendation to inspect evidence, affected projects, estimated savings, effort, risks, and restoration steps.
7. The user exports a plan or explicitly approves a supported operation.
8. The application verifies the result and records an audit event.
9. Later scans show growth, regressions, and completed recommendations.

## 7. Functional requirements

### 7.1 Storage inventory

The scanner must:

- Scan user-selected directories and mounted local volumes.
- Work without Full Disk Access, clearly representing inaccessible areas.
- Report logical size and allocated size when macOS exposes both.
- Avoid following symbolic links by default.
- Detect hard links and avoid double-counting their allocated storage.
- Detect APFS clones where feasible; otherwise mark savings estimates as uncertain.
- Support cancellation, pause, bounded concurrency, and incremental rescan.
- Persist scan checkpoints so an interrupted scan can resume.
- Record permission errors without repeatedly prompting.
- Avoid entering system-managed, network, cloud-placeholder, or backup locations unless explicitly selected.

### 7.2 Project discovery

Initial detectors must recognize:

- Git repositories, linked worktrees, submodules, remotes, branches, and shallow clones.
- Node.js projects and npm, Yarn, pnpm, and Bun lockfiles/workspaces.
- Python projects using pip, venv, uv, Poetry, Pipenv, and Conda.
- Go modules and workspaces.
- Rust Cargo projects and workspaces.
- Swift Package Manager and Xcode projects/workspaces.
- Java/Kotlin projects using Gradle or Maven.
- Dockerfiles and Compose projects.
- Terraform projects.

For each repository, the product must determine without modifying it:

- Working-tree status.
- Local commits not reachable from configured remotes, when remotes are available.
- Last source change and last Git activity.
- Available manifests and lockfiles.
- Project-local dependencies and build outputs.
- Required or inferred runtime versions.
- Associated worktrees and likely copies.

Network access is not required for the initial scan. Remote synchronization status must be labeled unknown unless the user explicitly requests a fetch or provides recent local remote-tracking evidence.

### 7.3 Tool taxonomy and runtime inventory

The product must keep these classes distinct. A single binary may participate in more than one class, but inventory and recommendations must not collapse them:

| Class | Role | Examples |
|---|---|---|
| Version manager / runtime installer | Installs and switches language or platform runtimes | Homebrew, mise, asdf, nvm, fnm, Volta, pyenv, rustup, SDKMAN, Xcode, standalone installers |
| Package manager | Resolves and installs project dependencies | npm, Yarn, pnpm, Bun, pip, Poetry, Pipenv, uv, Cargo, Go modules, Maven, Gradle, CocoaPods, SwiftPM, Composer, Bundler, Pub |
| Dependency store | Shared or content-addressed install material reused across projects | pnpm store, Yarn Berry cache/global folder, Cargo registry/git, Go module cache, shared uv/Python environments where applicable |
| Download cache | Regenerable downloads that are not project-local installs | npm `_cacache`, pip cache, Homebrew cache, Gradle caches |

#### Runtime and version-manager inventory

Detect installations managed by:

- Homebrew.
- mise.
- asdf.
- nvm, Volta, and fnm.
- pyenv, uv, Conda, and standalone Python installers.
- SDKMAN and standalone JDK distributions.
- rustup.
- Xcode and Apple platform SDKs.
- Manual installations discoverable through common paths and executable metadata.

For each installation, record location, version identity when available, estimated allocated size, and which projects declare or invoke it. The app must associate declared project requirements with installed versions before recommending consolidation.

### 7.4 Package managers, stores, and dependency analysis

#### Detection coverage

Inventory package managers and related stores under user-selected roots and well-known home paths. Detection should be broad; deep migration analysis may be narrower (see §7.5).

Initial package-manager and store detectors should cover at least:

- **JavaScript / TypeScript:** npm, Yarn classic, Yarn Berry (including PnP evidence), pnpm, Bun; project-local `node_modules`; global/content-addressed stores and download caches.
- **Python:** pip, venv/virtualenv, Poetry, Pipenv, uv, Conda; project environments; shared caches and tool-specific stores.
- **Go:** module cache and workspace/module projects.
- **Rust:** Cargo registry, git checkout cache, and `target/` build output as distinct from dependency stores.
- **Swift / Apple:** SwiftPM caches and Xcode-related package material, separate from DerivedData where possible.
- **Java / Kotlin:** Maven local repository, Gradle caches and wrappers, project build outputs.
- **Other common tools when present:** CocoaPods, Composer, Bundler/RubyGems, Flutter/Pub.

Absence of a tool is a valid inventory result. Do not assume a tool is unused solely because no project in the current scan roots references it; mark scope limits explicitly.

#### Dependency classes

The product must distinguish:

- Project-local installed dependencies.
- Global or content-addressed dependency stores.
- Download caches.
- Compiled build output.
- Unique user-created data.

#### Baseline analyses

- Identify repeated runtime versions installed through different version managers.
- Identify inactive reproducible project environments eligible for hibernation.
- Detect monorepo or workspace candidates conservatively; never recommend combining repositories solely from dependency similarity.
- Attribute store and cache sizes without double-counting hard links or shared store entries when metadata allows; otherwise mark savings ranges as uncertain.

### 7.5 Tool portfolio inventory and fit analysis

DevHearth inventories the local tool portfolio and recommends **fit**, not a single global “best package manager.”

#### Portfolio summary

Per ecosystem (and machine-wide where useful), the product must summarize:

- Which version managers, package managers, stores, and caches are installed.
- How many projects use each package manager (from lockfiles, manifests, and config).
- Allocated storage in project-local installs versus shared stores versus caches.
- Dominant tools already in use (portfolio gravity).
- Duplication across managers that provide overlapping capability (for example nvm + fnm + mise all managing Node).
- Activity of associated projects (active versus dormant), using the product’s inactivity signals.

#### Fit scoring

Fit analysis ranks portfolio options with multi-factor, evidence-backed scores. Disk savings are important but not the only factor.

Signals include:

| Signal | Use |
|---|---|
| Installed presence | What can the user adopt without a new install? |
| In-use share | What do most projects already use? |
| Duplication cost | GB tied up in per-project installs that a shared store could reduce |
| Runtime sprawl | Same runtime version via multiple managers |
| Reproducibility | Lockfiles, pinned engines/tool versions, workspace layout |
| Migration friction | Private registries, native addons, postinstall scripts, PnP, monorepo tooling, CI assumptions |
| Project activity | Prefer hibernation for cold projects; migration for hot ones when worthwhile |
| Policy preference | Explicit preferred tools override or reweight scores |

Outputs must be:

- Ranked options with tradeoffs (including **stay with the current tool** when migration cost exceeds benefit).
- Estimated immediate savings as a range.
- Estimated future-growth reduction when standardizing or adopting a shared store.
- Compatibility and workflow impact.
- Confidence, uncertainty, and explicit blockers.
- Proposed non-executing commands or plans only until mutation features ship.

#### Depth policy

- **Broad detection:** inventory many package managers, version managers, stores, and caches when present.
- **Deep fit analysis (MVP priority):** Node.js/TypeScript (npm, Yarn, pnpm, Bun) and Python (pip/venv, Poetry, Pipenv, uv, Conda) first.
- **Later depth:** Go, Rust, Swift/Xcode, Java/Gradle-Maven, then Ruby, PHP, Flutter/Pub as evidence and demand justify.

Deep analysis for an ecosystem may remain disabled until fixture coverage and confidence thresholds are met; shallow inventory is still valuable.

#### Explicit non-goals for fit analysis

- Declaring one absolute best package manager for the machine or industry.
- Auto-migrating lockfiles or package managers without explicit consent.
- Scoring that optimizes only for disk while ignoring workflow and team constraints.
- Treating “newer tool” as a safety or compatibility signal.
- Recommending monorepo merges solely from dependency similarity.

### 7.6 Containers and virtual machines

Detect Docker Desktop, Colima, Lima, OrbStack, Podman, and Apple container installations when present.

Associate:

- Compose definitions with containers, images, networks, and named volumes where evidence permits.
- Image tags and digests with project references.
- Virtual-machine disk images with their owning runtime.
- Architecture variants such as arm64 and amd64.

Named volumes and database storage are never classified as reproducible based only on container configuration.

### 7.7 AI development assets

Initial detectors should cover common locations and formats for:

- Ollama.
- Hugging Face Hub.
- LM Studio.
- MLX.
- Stable Diffusion applications and checkpoints.
- GGUF, SafeTensors, PyTorch, ONNX, and Core ML model files.
- LoRA adapters and fine-tuning checkpoints.
- Vector databases, embeddings, datasets, and generated outputs.
- AI editor workspaces and agent-created worktrees where identifiable.

The product must distinguish downloaded base models from user-created fine-tunes, adapters, datasets, and outputs. Similar filenames are not sufficient evidence of duplication. Full hashing should be opt-in or scheduled only for high-value candidates because of I/O and energy cost.

### 7.8 Recommendation engine

Each recommendation must contain:

- Title and plain-language explanation.
- Evidence and confidence.
- Affected assets and projects.
- Estimated immediate savings as a range where necessary.
- Estimated future-growth reduction.
- Time or download cost to restore.
- Compatibility and workflow impact.
- Risk class.
- Preconditions.
- Proposed commands or actions.
- Verification and rollback plan.

For portfolio-fit recommendations, also include:

- Ranked alternatives considered (including stay-put).
- Scoring factors and which signals dominated.
- Blockers that prevented a higher-ranked option.

Initial recommendation families:

1. Consolidate duplicate runtime or version-manager installations.
2. Adopt a shared dependency store.
3. Standardize package-manager or version-manager portfolio fit (rank options; never a single absolute winner).
4. Hibernate an inactive reproducible project.
5. Remove an obsolete worktree after Git verification.
6. Unify or relocate duplicate downloadable AI models.
7. Replace redundant per-project services with a shared local service, as guidance only.
8. Remove an unused architecture variant.
9. Pin missing runtime versions, package managers, or lockfiles to make future hibernation and fit analysis possible.
10. Move cold but non-reproducible assets to external or managed storage.

### 7.9 Hibernation and restoration

Hibernation is not part of the first read-only release. When added, it must:

- Refuse by default when a repository has uncommitted changes or unpushed commits.
- Require a suitable manifest and lockfile for dependency removal.
- Record exact affected paths, sizes, tool versions, and restoration commands.
- Support a dry run.
- Remove only explicitly approved generated assets.
- Verify that source files and declared persistent data remain.
- Produce a portable hibernation manifest.
- Restore and validate the environment on request.

### 7.10 Policy synchronization

Users must be able to export a redacted, versionable policy file containing:

- Scan roots expressed using portable aliases where possible.
- Exclusion patterns.
- Preferred runtime, version, and package managers (explicit user choices, not raw inventory).
- Fit-scoring weights or modes when configurable (for example prefer disk savings versus prefer workflow stability).
- Risk thresholds and retention rules.
- Recommendation suppressions.
- Machine-class overrides for architecture, disk capacity, and role.

Absolute personal paths, repository names, asset inventory, credentials, scan history, and inferred portfolio fingerprints must not be exported by default. Optional redacted portfolio summaries may be exported only with explicit opt-in.

The policy document must make this structural rather than procedural: no field may be capable of holding an absolute path. A scan root is an alias plus a relative segment, and a root that cannot be expressed portably is not stored at all. Exclusions are relative patterns.

An imported policy is untrusted input and is validated at the boundary: unknown schema versions, unknown fields, roots that escape their alias, absolute exclusions, invalid globs, out-of-range weights or windows, suppressions that name no target, control characters, and oversized documents are refused rather than repaired.

Recommendation feedback (whether advice was useful) is local only and has no export path. A decision the user wants to carry to another machine is recorded as a suppression.

### 7.10a Monitoring and growth

Repeat scans of the same roots must be able to answer what changed:

- Per-scan snapshots holding counts and byte totals for named series (per ecosystem, per storage class, and the scan total). Snapshots must not hold paths.
- Growth series with deltas, rate per day, and direction, where a small move reads as unchanged rather than as a trend.
- Regression signals for a sudden jump, sustained growth across consecutive scans, and rising recoverable storage. A signal names an observation, never a cause.
- Comparison only within one scan scope. Scans covering different roots are different measurements and must not be subtracted from each other.
- Only completed scans enter history; a partial scan compared against a full one would read as storage disappearing.

### 7.11 User interface

The macOS UI must provide:

- Onboarding and permission status.
- Scan scope selection.
- Progress with current phase, files visited, bytes assessed, and inaccessible areas.
- Overview by actual allocated storage.
- Project, ecosystem, runtime, package-manager, dependency-store, container, and AI-asset views.
- Tool portfolio summary per ecosystem (installed tools, in-use share, store versus project-local storage).
- Recommendation inbox ranked by value, risk, and confidence.
- Evidence inspector, including fit-score factors for portfolio recommendations.
- Plan comparison showing before/after estimates.
- Growth trends across repeat scans, with the changes worth a look called out.
- Policy view: fit weighting, risk threshold, portable scan roots, and hidden advice, with export and import.
- Search, filtering, and exclusions.
- Audit history.
- Export of reports and policy.
- Accessibility, keyboard navigation, dark mode, and reduced-motion support.
- Adjustable text size. macOS has no Dynamic Type, so the app owns its own scale: `⌘+`, `⌘−`, and `⌘0` must resize the whole window, and the setting must persist across launches. The default ramp targets a large external display rather than the AppKit defaults, because the densest views here are evidence tables read at arm's length.

## 8. Risk model

- Informational: No mutation; facts or configuration advice.
- Low: Reproducible, project-local generated data with verified restoration inputs.
- Medium: Re-download/rebuild cost, package-manager or runtime migration, version-manager consolidation, or uncertain compatibility.
- High: Persistent application data, container volumes, local databases, unsigned Git state, user-created AI assets, datasets, or system-managed data.
- Prohibited: Credentials, keychains, protected macOS system data, or any deletion whose ownership and recovery cannot be established.

AI-generated explanations cannot lower a deterministic risk rating.

## 9. Non-functional requirements

### Performance

- Scanning should be I/O-efficient and keep the UI responsive.
- Concurrency must be bounded to avoid saturating storage or degrading active development.
- The scanner should batch database writes.
- Hashing should use staged comparison: size, metadata, sampled hash, then full hash only when justified.
- Incremental scans should reuse unchanged directory results.
- Memory use must not grow proportionally with the total number of files.

Initial performance targets on a modern Apple Silicon Mac with local APFS storage:

- UI remains interactive throughout a scan.
- Cancellation acknowledged within one second under normal filesystem conditions.
- Peak engine memory below 500 MB for ten million discovered filesystem entries.
- A repeated scan with few changes completes substantially faster than a cold scan.

These are engineering targets, not product claims, until benchmarked.

### Reliability

- A crash cannot leave an approved operation partially undocumented.
- Scan results are versioned by schema.
- Mutations use preflight, execution, and verification phases.
- Engine and UI versions negotiate protocol compatibility.
- Unsupported detector versions degrade to read-only reporting.

### Privacy and security

- No telemetry by default.
- No file contents sent off-device by default.
- Secrets and known credential locations are excluded from content inspection.
- External AI providers require explicit configuration and a preview of data to be sent.
- Reports are redacted by default.
- The application does not require root privileges for ordinary operation.

## 10. MVP scope

The first public milestone is a read-only advisor:

- Native macOS UI.
- Go CLI/scanning engine.
- User-selected scan roots.
- Project and Git discovery.
- Node, Python, Go, Rust, Swift, Java, container, and basic AI-asset detection.
- Runtime, version-manager, package-manager, and dependency-store inventory.
- Logical and allocated-size reporting.
- Asset relationship graph stored locally.
- Portfolio summary for major ecosystems; deep fit scoring for Node.js/TypeScript and Python first.
- Evidence-backed recommendations for runtime consolidation, shared-store adoption, portfolio fit, and project hibernation candidates.
- JSON report and portable policy export.
- No deletion or automatic migration.

## 11. Explicitly out of scope for MVP

- General-purpose macOS cleaning.
- Antivirus or malware scanning.
- Duplicate personal photo/document management.
- Automated package-manager migration or lockfile rewriting.
- Declaring a single absolute best package manager for the machine.
- Deep fit analysis for every detected ecosystem in the first release.
- Automatic deletion.
- Cloud inventory synchronization.
- Team administration or fleet enforcement.
- Linux or Windows graphical applications.
- Perfect APFS clone attribution.
- Inferring that persistent data is safe from filenames alone.

## 12. Success measures

- Percentage of allocated developer storage assigned to a known asset and owner.
- Percentage of developer storage attributed to a known package manager, store, or version manager.
- Recommendation acceptance and dismissal reasons, including stay-put acceptances.
- Verified space reduction from completed structural optimizations.
- Reduction in duplicate runtime, version-manager, and dependency installation over time.
- Successful hibernation restoration rate when that feature ships.
- Zero loss of user-created data.
- Time to useful first recommendation.
- Percentage of recommendations with high-confidence evidence.
- Share of portfolio-fit recommendations where users report the ranking matched their constraints.

## 13. Open product questions

- Should the first release support the Mac App Store, or distribute directly to simplify privileged filesystem access and backend updates?
- Which AI applications provide supported shared-model configuration rather than requiring fragile symbolic links?
- How accurately can APFS clone ownership and exclusive allocated size be measured using public APIs?
- Should policies be a standalone repository, dotfile-compatible file, or both?
- What inactivity signal should dominate: source modification, Git activity, IDE use, or explicit user status?
- Should restoration commands execute inside the app or be exported for terminal review first?
- How opinionated should default fit scoring be: disk-first, workflow-stability-first, or balanced with user-visible weights?
- When portfolio gravity and industry defaults disagree (for example most projects on npm while pnpm would save more disk), should the product prefer standardize-on-dominant or migrate-to-shared-store by default?
- Should team policy export ever include redacted portfolio fingerprints, or only explicit preferred-tool settings?

