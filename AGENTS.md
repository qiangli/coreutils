# Repository Guidelines

## Project Structure & Module Organization

This is a Go module (`github.com/qiangli/coreutils`) that implements a pure-Go, cross-platform userland for agents. Command implementations live in `cmds/`, with one package per utility such as `cmds/ls`, `cmds/cp`, and `cmds/sort`; `cmds/all` registers the full set. The multicall CLI entry point is `cmd/coreutils`. Shared command framework code is in `tool/`, the in-process git implementation is in `git/`, and reusable support packages are under `pkg/`. Project docs are in `docs/`; vendored or mirrored upstream code belongs under `external/`.

## Build, Test, and Development Commands

- `go test ./...` runs all unit and integration tests across the module.
- `go vet ./...` runs the same static checks used by CI.
- `go build ./cmd/coreutils` builds the busybox-style multicall binary.
- `go test ./cmds/ls ./tool` limits testing to specific packages while iterating.
- `gofmt -w <files>` formats changed Go files before review.

CI runs `go vet ./...` and `go test ./...` on Linux, macOS, and Windows, so avoid platform assumptions and host shell-outs.

## Coding Style & Naming Conventions

Use standard Go formatting: tabs from `gofmt`, short package names, and idiomatic `CamelCase` exported identifiers. Command packages should be named after the utility they implement (`cmds/base64`, `cmds/mkdir`). Keep behavior deterministic: prefer explicit environment handling through `tool.RunContext` over process globals, and preserve `LC_ALL=C` style output semantics. Unsupported flags should fail loudly rather than being ignored.

## Testing Guidelines

Tests use Go's standard `testing` package and follow `*_test.go` naming. Place command-specific tests beside the command implementation, for example `cmds/head/head_test.go`; shared framework tests belong near the relevant package, such as `tool/tool_test.go`. Add table-driven tests for flags, stdin/stdout behavior, errors, and platform-specific paths where applicable. Run targeted package tests during development, then `go test ./...` before submitting.

## Commit & Pull Request Guidelines

Recent commits use concise, imperative subjects with an optional scope, for example `binmgr: GitHub-release resolver` or `external/zot: run Zot...`. Keep the first line focused on the behavioral change. Pull requests should include a short description, test results (`go vet ./...`, `go test ./...`), linked issues when relevant, and notes for any intentional GNU/POSIX compatibility gaps or platform-specific behavior.

## Compatibility & Licensing Notes

Implement commands from public documentation and permissively licensed sources only; do not copy GPL implementation code. Update `THIRD_PARTY_LICENSES.md` when adding dependencies or adapted code that requires attribution.
