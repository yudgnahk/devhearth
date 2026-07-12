#!/bin/sh
set -eu

go test ./...
go test -run '^$' -bench 'Benchmark(Cold|Incremental)Fixture' ./internal/scan
engine_path="$(mktemp -d)/devhearth"
trap 'rm -rf "$(dirname "$engine_path")"' EXIT
go build -o "$engine_path" ./cmd/devhearth
DEVHEARTH_ENGINE_PATH="$engine_path" swift test --package-path apps/macos
