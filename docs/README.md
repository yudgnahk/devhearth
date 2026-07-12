# DevHearth documentation

- [Product specification](SPECS.md)
- [Architecture and technology stack](ARCHITECTURE.md)
- [Safety, privacy, and trust model](SAFETY_AND_PRIVACY.md)
- [Roadmap](ROADMAP.md)
- [Phase 0 task list](PHASE_0_TASKS.md)

The current product direction is a read-only macOS development-environment advisor implemented with a native SwiftUI application and a Go scanning/analysis engine. It inventories projects, version managers, package managers, and dependency stores; builds an ownership graph; and recommends structural fit (shared stores, runtime consolidation, hibernation candidates)—not a single absolute “best” tool. Filesystem mutation is deliberately deferred until the inventory, evidence, and safety model have been validated.
