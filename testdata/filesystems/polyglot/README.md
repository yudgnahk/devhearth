# Synthetic polyglot fixture

This tree contains only invented project metadata. It is safe to scan and must never contain credentials or copied user data.

Layouts covered for Phase 2 detectors:

- Node/pnpm (`package.json`, `pnpm-lock.yaml`, `node_modules`)
- Python (`pyproject.toml`, `.venv`)
- Go (`go.mod`)
- Rust (`Cargo.toml`)
- SwiftPM (`Package.swift`)
- Java/Maven (`pom.xml`)
- Terraform (`main.tf`)
- Docker/Compose (`Dockerfile`, `compose.yaml`)
- Version manager / store stubs (`.nvm`, `.fnm`, `.pnpm-store`, `.npm`)
- AI store stubs (`.ollama`, LM Studio `models`)

Layouts added for Phase 3 advice:

- A second Node project on npm (`node-legacy`) so the portfolio holds mixed
  package managers and more than one project-local install.
- A second Python project on Poetry (`python-legacy`) with its own `.venv`.
- Two Node version managers (`.nvm` plus `.fnm`) so runtime consolidation has
  overlapping capability to report.
- Duplicate model filenames in two stores. They are deliberately tiny, so the
  duplicate-model rule ignores them: name and size matching alone is not
  evidence, and the rule has a minimum size to keep placeholders out of the
  inbox. Rule behaviour for real sizes is covered by unit tests.

Every file is invented metadata. Sizes here are far too small to produce
meaningful savings figures; the fixture exists to exercise detection, fit
ranking, and rule preconditions, not to measure storage.

Git repository and worktree signatures are covered by unit tests under
`internal/detect/git` rather than a nested `.git` directory (Git will not
store another repository directory inside this tree).
