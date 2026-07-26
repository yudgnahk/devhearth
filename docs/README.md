# DevHearth documentation

- [Product specification](SPECS.md)
- [Architecture and technology stack](ARCHITECTURE.md)
- [Safety, privacy, and trust model](SAFETY_AND_PRIVACY.md)
- [Roadmap](ROADMAP.md)
- [Codebase audit and improvement plans](IMPROVEMENT_PLAN.md)
- [Phase 0 task list](PHASE_0_TASKS.md)
- [Phase 1 task list](PHASE_1_TASKS.md)
- [Phase 2 task list](PHASE_2_TASKS.md)
- [Phase 3 task list](PHASE_3_TASKS.md)
- [Phase 4 task list](PHASE_4_TASKS.md)

The current product direction is a read-only macOS development-environment advisor implemented with a native SwiftUI application and a Go scanning/analysis engine. It inventories projects, version managers, package managers, and dependency stores; builds an ownership graph; and recommends structural fit (shared stores, runtime consolidation, hibernation candidates)—not a single absolute “best” tool. Filesystem mutation is deliberately deferred until the inventory, evidence, and safety model have been validated.

## Developer commands

From the repository root, `make help` lists targets. Common ones:

```sh
make build          # Go engine + SwiftUI binary
make run            # launch the preview UI with the local engine
make scan-fixture   # CLI scan of testdata/filesystems/polyglot
make scan ROOT=…    # CLI scan of a chosen folder
make test-go        # Go tests
make check          # vet + Go tests + build
```

A signed `.app` bundle that embeds the engine still needs full Xcode; the current path is SPM + `DEVHEARTH_ENGINE_PATH`.
