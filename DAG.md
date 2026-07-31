---
name: coreutils
description: Build/test/lint targets for coreutils as a bashy dag pipeline (agent-first, no Makefile)
---

# coreutils — DAG task file

coreutils has no Makefile — it builds with plain `go` commands. This DAG file is
the agent-first equivalent, runnable with the `bashy dag` task runner:

```bash
bashy dag --list            # available targets
bashy dag build             # build the multicall binary into ./bin
bashy dag test              # test coreutils' own packages (CI scope)
bashy dag crossvet          # cross-OS compile gate (windows/linux/darwin)
bashy dag fmtcheck          # gofmt gate — reports, never rewrites
bashy dag --json test       # machine-readable envelope for an agent
```

**Two gates, not one.** `test` proves the host platform; `crossvet` proves the
ones you are not on. A change is gated by BOTH — `go test` on darwin cannot
see a build-tag break, because the offending file never compiles for windows.
Windows is a shipping target here, so a darwin-only green is not a green.

The default `test`/`vet` scope **excludes the vendored `external/` forks**
(ollama, podman) — they pull cgo + platform-specific backends (MLX, btrfs) and
are upstream's to test; this is exactly the cross-platform CI scope, so the
Windows leg (the product) stays green. `test-all` includes everything for a unix
host with the submodules hydrated.

Resolving the engine: coreutils replaces `mvdan.cc/sh/v3 => ../sh` and the
ollama/podman forks via submodules; inside the dhnt umbrella both are present.

## Tasks

### build
Build the busybox-style multicall binary (`coreutils <tool>` / argv[0] dispatch)
into ./bin. Pure-Go + cross-platform (no external engines), so it builds for
every OS including Windows.
Sources: cmd/, cmds/, tool/, pkg/, git/, shell/, go.mod, go.sum
Generates: bin/coreutils
Effects: write

```bash
set -e
mkdir -p bin
go build -trimpath -o bin/coreutils ./cmd/coreutils
```

### test
Test coreutils' own packages — the cross-platform CI scope (excludes the
vendored external/ forks).
Effects: read

```bash
set -e
go vet $(go list ./... | grep -v /external/)
go test $(go list ./... | grep -v /external/)
```

### crossvet
Cross-OS typecheck of the CI scope WITHOUT needing a Windows/Linux box —
`go vet` compiles every package (tests included) for the target GOOS, which
is exactly the class of break the CI windows leg keeps catching after
darwin-only local work (unix-only types like syscall.Stat_t in untagged
files, or a `//go:build !windows` helper referenced from an untagged test).
`go test` on darwin structurally cannot see that: the file never compiles
for the other platform. Run it alongside `test` before every merge — it is
a gate, not a lint.

The body delegates to `scripts/crossvet.sh`, which is also what the pre-push
hook (`scripts/hooks/pre-push`) execs. One script, two callers: the target
list cannot drift between the manual gate and the automatic one. Read that
script for the target rationale (including the deliberate `aix`
fail-closed-lock canary).
Effects: read

```bash
scripts/crossvet.sh
```

### fmtcheck
The formatting gate. **Reports; never rewrites** — and that is the whole
design, not caution. gofmt changes doc-comment TEXT as well as whitespace:
godoc's legacy typographic substitution turns a pair of ASCII single-quotes
into a curly closing quote. That has already silently corrupted a comment in
this tree which quoted shell syntax, leaving it documenting a form that does
not exist. An auto-fixing gate lands that class of change unreviewed, wearing
the disguise reviewers skim past.

So the gate fails and a human decides. When a file legitimately cannot take
gofmt's output, restructure it — move the literal into an indented doc-comment
code block, where the characters survive verbatim.

The body delegates to `scripts/fmtcheck.sh`, which is also what the CI ubuntu
leg runs. One script, two callers. Tracked `.go` files only; `external/` is
excluded as vendored upstream.
Effects: read

```bash
scripts/fmtcheck.sh
```

### vet
Static check, same scope as `test`.
Effects: read

```bash
go vet $(go list ./... | grep -v /external/)
```

### test-all
Full test including the vendored external/ forks. Needs a unix host with cgo and
the ollama/podman submodules hydrated.
Effects: read

```bash
go test ./...
```

### dist
Cross-compile the multicall binary for every release platform into bin/dist/.
Generates: bin/dist
Effects: write

```bash
set -e
mkdir -p bin/dist
for plat in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64; do
  os=${plat%/*}; arch=${plat#*/}; ext=""
  [ "$os" = windows ] && ext=.exe
  out="bin/dist/coreutils-${os}-${arch}${ext}"
  echo "building $out..."
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -o "$out" ./cmd/coreutils
done
```

### clean
Remove built binaries.
Effects: destroy

```bash
rm -rf bin
```
