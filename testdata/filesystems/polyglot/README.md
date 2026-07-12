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
- Version manager / store stubs (`.nvm`, `.pnpm-store`)
- AI store stub (`.ollama`)

Git repository and worktree signatures are covered by unit tests under
`internal/detect/git` rather than a nested `.git` directory (Git will not
store another repository directory inside this tree).
