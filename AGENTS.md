# Repository Guidelines

**`bashy sprint` is the source of requests, plans and details for every agent** — read the sprint card (spec-ref, acceptance, continuity) for what to do, never this file. Delivery commits carry `Sprint:` / `Story:` / `Story-ID:` trailers.

## Project Structure & Module Organization


This Go module (`github.com/qiangli/coreutils`) is the certified POSIX package of bashy's pure-Go, cross-platform userland: the 116 POSIX-required names ∪ GNU coreutils, and nothing else (everything agentic is the flat sibling `github.com/qiangli/yoke`, which imports this module). Command packages live in `cmds/`, one utility per directory (`cmds/ls`, `cmds/sed`, `cmds/awk`); `cmds/all` registers the required set. Entrypoints are under `cmd/`: `cmd/coreutils` is the busybox-style certification binary, and `cmd/perfbench` is a benchmark/conformance host. Shared runtime and flags live in `tool/`; the few shared packages the applets need are in `pkg/`. Docs live in `docs/`.

## Build, Test, and Development Commands

- `go build ./cmd/coreutils` builds the multicall userland binary.
- `go test ./...` runs the suite (the whole tree is pure Go; nothing vendored).
- `go test ./cmds/ls ./tool` runs focused packages while iterating.
- `go vet ./...` runs static checks (CI runs `scripts/ci-test-gate.sh`, the no-regression ratchet).
- `go run ./cmd/perfbench --help` runs the development conformance/benchmark harness.
- `gofmt -w <files>` formats changed Go files before review.

Use Go from `go.mod`. The module replaces `mvdan.cc/sh/v3` with sibling `../sh`; standalone clones need that checkout. There are no submodules.

## Coding Style & Naming Conventions

Use standard Go formatting: tabs from `gofmt`, short package names, and idiomatic `CamelCase` exported identifiers. Command packages are named after the utility (`cmds/base64`, `cmds/mkdir`) and register through `tool`. Use `tool.RunContext` for stdio, working directory, and environment instead of process globals. Preserve `LC_ALL=C`-style output; unsupported flags or modes must fail loudly.

## Testing Guidelines

Tests use Go's standard `testing` package and `*_test.go` files. Place command tests beside implementations (`cmds/head/head_test.go`); shared runtime tests belong with their package (`tool/tool_test.go`, `pkg/bre/*_test.go`). Prefer table-driven coverage for flags, stdio, stderr, exit codes, and platform paths. For parity work, update relevant docs or `cmds/perfbench/results/` artifacts.

## Commit & Pull Request Guidelines

Recent commits use concise, imperative subjects with an optional scope, for example `shuf: fail cleanly on unsatisfiable -i ranges`, `cmds: add shell utilities`, or `docs: refresh commands.md against the current tree`. Keep the first line focused on behavior. Pull requests should include a short description, test/vet results, linked issues when relevant, and notes for GNU/POSIX compatibility gaps or platform-specific behavior.

## Compatibility & Licensing Notes

Implement commands from public documentation and permissively licensed sources only; do not copy GPL implementation code. Maintain pure-Go behavior with no host shell-outs for userland commands. Update `THIRD_PARTY_LICENSES.md` when adding dependencies or adapted code that requires attribution.
