# Phase 0 task list

- [x] Finalize product vocabulary and scope in `SPECS.md`.
- [x] Choose the DevHearth project name.
- [x] Create the GitHub repository.
- [x] Add the MIT license.
- [x] Establish independently testable Go and Swift directory structures.
- [x] Define and test the JSON-RPC handshake and schema version negotiation.
- [x] Define the versioned SQLite schema and detector interface.
- [x] Build representative synthetic filesystem fixtures.
- [x] Establish cold and incremental scan benchmarks.
- [x] Launch the Go engine from Swift, negotiate versions, and display its mocked scan stream through the development-path integration.
- [ ] Add the Xcode app target/build phase that embeds and signs the Go engine as an auxiliary executable.
- [ ] Validate the bundled-engine flow with a matching full Xcode toolchain.

The automated foundation check builds the Go engine and supplies it to a Swift subprocess integration test through `DEVHEARTH_ENGINE_PATH`. Phase 0 reaches its roadmap exit criterion after that check passes and the remaining app-bundle tasks are complete. The current machine has mismatched Swift compiler/SDK versions and no full Xcode installation, so bundled-engine and signing validation remain open.
