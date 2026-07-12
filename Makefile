# DevHearth developer commands
#
# Build artifacts land in build/. The Swift UI looks for the Go engine via
# DEVHEARTH_ENGINE_PATH (development) or a bundled auxiliary executable (app bundle).

SHELL := /bin/bash
.DEFAULT_GOAL := help

REPO_ROOT   := $(abspath .)
BUILD_DIR   := $(REPO_ROOT)/build
ENGINE      := $(BUILD_DIR)/devhearth
MACOS_PKG   := $(REPO_ROOT)/apps/macos
SWIFT_BIN   := $(MACOS_PKG)/.build/debug/DevHearth
FIXTURE     := $(REPO_ROOT)/testdata/filesystems/polyglot
DB_PATH     := $(BUILD_DIR)/inventory.sqlite
# Optional scan directory for `make scan ROOT=...`
ROOT        ?=

GO          ?= go
SWIFT       ?= swift
ENGINE_ENV  := DEVHEARTH_ENGINE_PATH=$(ENGINE)

.PHONY: help \
	build build-engine build-app \
	test test-go test-swift test-swift-if-available \
	vet check \
	run run-app run-engine \
	scan scan-fixture \
	clean clean-all \
	fmt tidy

help: ## Show available targets
	@awk 'BEGIN {FS = ":.*##"; printf "DevHearth targets:\n\n"} \
		/^[a-zA-Z0-9_.-]+:.*##/ { printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2 }' $(MAKEFILE_LIST)
	@printf "\nExamples:\n"
	@printf "  make build && make run\n"
	@printf "  make scan-fixture\n"
	@printf "  make scan ROOT=$(FIXTURE)\n"
	@printf "  make scan ROOT=$$HOME/Projects\n\n"
	@printf "Notes:\n"
	@printf "  - Full .app bundling/signing needs Xcode (not only Command Line Tools).\n"
	@printf "  - Swift tests need the Xcode Testing module; go tests always run.\n"
	@printf "  - make run launches the SwiftUI preview with the local Go engine.\n\n"

##@ Build

build: build-engine build-app ## Build Go engine and macOS SwiftUI binary

build-engine: ## Build the Go engine binary into build/devhearth
	@mkdir -p "$(BUILD_DIR)"
	$(GO) build -o "$(ENGINE)" ./cmd/devhearth
	@echo "engine: $(ENGINE)"

build-app: build-engine ## Build the SwiftUI executable and place the engine beside it
	$(SWIFT) build --package-path "$(MACOS_PKG)"
	@test -x "$(SWIFT_BIN)"
	@# SPM executables are not app bundles; co-locate the engine so the UI can
	@# find it without DEVHEARTH_ENGINE_PATH when launched from make run.
	cp -f "$(ENGINE)" "$(dir $(SWIFT_BIN))devhearth"
	chmod +x "$(dir $(SWIFT_BIN))devhearth"
	@echo "app:    $(SWIFT_BIN)"
	@echo "engine: $(dir $(SWIFT_BIN))devhearth"

##@ Test

test: test-go test-swift-if-available ## Run Go tests; Swift tests when toolchain supports them

test-go: ## Run all Go tests
	$(GO) test ./...

test-swift: build-engine ## Run Swift package tests (requires Xcode Testing module)
	$(ENGINE_ENV) $(SWIFT) test --package-path "$(MACOS_PKG)"

test-swift-if-available: build-engine ## Run Swift tests when possible; otherwise skip with a note
	@if $(ENGINE_ENV) $(SWIFT) test --package-path "$(MACOS_PKG)" >/tmp/devhearth-swift-test.log 2>&1; then \
		echo "swift tests: ok"; \
	else \
		echo "swift tests: skipped (see /tmp/devhearth-swift-test.log)"; \
		echo "  Install full Xcode if you need the Swift Testing module."; \
	fi

vet: ## Run go vet
	$(GO) vet ./...

check: vet test-go build ## Lint-ish gate used before PRs (Go-focused)
	@echo "check: ok (engine at $(ENGINE))"

##@ Run

run: run-app ## Alias for run-app

run-app: build ## Launch SwiftUI app with the local engine
	@test -x "$(ENGINE)"
	@test -x "$(dir $(SWIFT_BIN))devhearth"
	@echo "Launching DevHearth"
	@echo "  app:    $(SWIFT_BIN)"
	@echo "  engine: $(ENGINE)"
	@echo "Status should become 'Engine ready' immediately; then choose a folder."
	@# Prefer explicit env, with co-located binary as fallback inside the app.
	cd "$(REPO_ROOT)" && $(ENGINE_ENV) "$(SWIFT_BIN)"

run-engine: build-engine ## Run the engine on stdin/stdout (JSON-RPC)
	@mkdir -p "$(BUILD_DIR)"
	@echo "Engine ready on stdio. Database: $(DB_PATH)"
	@echo "Send newline-delimited JSON-RPC on stdin (Ctrl-D to exit)."
	"$(ENGINE)" -database "$(DB_PATH)"

scan: build-engine ## Scan ROOT via JSON-RPC and print assets/portfolio/report
	@test -n "$(ROOT)" || (echo "usage: make scan ROOT=/path/to/folder"; exit 2)
	@mkdir -p "$(BUILD_DIR)"
	@ROOT_ABS=$$(cd "$(ROOT)" && pwd); \
	"$(REPO_ROOT)/scripts/scan-roots.sh" "$(ENGINE)" "$$ROOT_ABS" "$(DB_PATH)"

scan-fixture: ## Scan the synthetic polyglot fixture
	@$(MAKE) scan ROOT="$(FIXTURE)"

##@ Maintenance

fmt: ## Format Go sources
	$(GO) fmt ./...

tidy: ## Tidy Go modules
	$(GO) mod tidy

clean: ## Remove build/ and Swift package build products
	rm -rf "$(BUILD_DIR)"
	$(SWIFT) package --package-path "$(MACOS_PKG)" clean >/dev/null 2>&1 || true
	rm -rf "$(MACOS_PKG)/.build"

clean-all: clean ## clean + remove local inventory leftovers under build/
	rm -rf "$(BUILD_DIR)"
