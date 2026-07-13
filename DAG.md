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
bashy dag --json test       # machine-readable envelope for an agent
```

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
files). Run before every push; the pre-push hook (scripts/hooks/pre-push)
runs this automatically once installed.

The `aix` build is a DELIBERATE canary, not a shipping target. A build tag
that says `!windows` is a claim that every other OS is a unix with flock —
and aix and solaris lock through fcntl, so such a tag does not merely
mislabel them, it fails to COMPILE. Locking code is where this keeps
happening (pkg/steward, pkg/policy/coord), and the fail-closed
implementations those packages ship for unsupported platforms are only
reachable if the package builds there at all. `go build` rather than
`go vet`, since aix has no test-runner story and the point is the tag
selection.
Effects: read

```bash
set -e
for os in windows linux darwin; do
  echo "crossvet: GOOS=$os"
  GOOS=$os go vet $(go list ./... | grep -v /external/)
done
echo "crossvet: GOOS=aix (fail-closed-lock canary)"
GOOS=aix GOARCH=ppc64 go build ./pkg/steward/ ./pkg/policy/coord/
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
