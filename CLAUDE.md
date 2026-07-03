# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Overview

`coreutils` is a pure-Go agent userland: Unix-style tools (`git`, and over
time `ls`/`cat`/`sort`/…) implemented with no cgo and no shell-out, so
agentic consumers get **one identical toolset on every platform** — the
load-bearing case is Windows hosts with nothing installed. It is a library
first: consumers embed packages directly (outpost, ycode); a busybox-style
multicall binary is planned, not primary.

This repo is OSS (MIT) and is consumed by other OSS repos. Two hard rules:

1. **Never port GNU source.** GNU coreutils is GPLv3. Implement behavior
   from the GNU manual / POSIX documentation, or adapt code from the
   permissive prior-art clones below — never from GPL code.
2. **Never shell out.** No `os/exec` anywhere. If pure Go can't do it, the
   tool returns a clear error naming what's unsupported. Partial flag
   coverage is fine — silent approximation is not.
3. **Upstream semantics are immutable.** Every flag, option, and argument
   a tool accepts means exactly what the original command's official
   documentation says it means — same spelling, same default, same
   interaction with other flags, same output shape. There are two states
   only: supported-as-documented, or a clear "not supported" error.
   Never repurpose a flag, never approximate, never invent a
   different-but-similar behavior under an upstream name. (New
   capabilities that don't exist upstream are possible, but under
   clearly non-upstream spellings and documented as extensions.)

## The agent contract

Every tool: deterministic output (`LC_ALL=C` semantics, no locale/color/
terminal variance), GNU exit-code conventions (0 ok, 1 failure, 2 usage),
unsupported flags fail loudly naming the flag. This is documented in
README.md and is the review bar for every PR.

## Prior art (`priorart/`, gitignored)

Local clones of permissive open-source reimplementations, for studying
and — where the license allows — adapting. **Conformance is judged
against the original command's official documentation, never against
prior art**: these projects have their own gaps and deviations (u-root
is deliberately flag-partial; aict changes output formats), so anything
copied must be verified against the GNU manual / POSIX and covered by
tests before it counts as supported.

| Clone | Project | Lang | License | Policy |
|---|---|---|---|---|
| `priorart/aict` | aict (agent-oriented coreutils, XML/JSON output) | Go | MIT | copy/adapt |
| `priorart/guonaihong-coreutils` | guonaihong/coreutils | Go | Apache-2.0 | copy/adapt |
| `priorart/u-root` | u-root/u-root (`cmds/core/…`) | Go | BSD-3-Clause | copy/adapt |
| `priorart/coreutils` | microsoft/coreutils | Rust | MIT | reference only |
| `priorart/uutils-coretuils` | uutils/coreutils | Rust | MIT | reference only (best GNU-fidelity reference) |

When adapting code from a copy/adapt clone:

- Keep a provenance header on the file: source repo, path, license.
- Add the source to `THIRD_PARTY_LICENSES.md` (license text included).
  Apache-2.0 sources (guonaihong) additionally require stating changes.
- Strip anything that violates the agent contract while adapting —
  Linux-only assumptions (u-root), non-GNU output modes (aict's
  XML/JSON), locale-dependent behavior.
- aict's XML/JSON output idea is explicitly NOT adopted as default
  behavior — upstream tools don't do it, and rule 3 forbids changing
  output shapes under upstream names. If structured output ever lands,
  it will be an explicitly documented extension flag.

The Rust clones are semantic references: uutils is the most
GNU-faithful reimplementation in existence and is often the fastest way
to resolve "what does GNU actually do here" questions — but code flows
from it only as understanding, never as translation (reference-only by
policy to keep provenance simple, even though its license would allow
more).

## Build & test

```bash
go build ./...
go test ./...          # hermetic; no network, no system git required
go test -short ./...   # skips the slower e2e-ish cases
```

## Architecture

### git/ — the one shipped package so far

Self-contained git client on go-git/v5. Two API layers over one
implementation:

- **Typed functions** (`Clone`, `Pull`, `Merge`, `TagCreate`, `ConfigSet`,
  …) returning `*Result` / typed slices — for CLIs that own their flag
  parsing. outpost's `outpost git` cobra tree consumes these. `CLIName`
  (package var, default `"git"`) is the command prefix used in hint
  messages; embedders override it at init (`outpost git`, `bashy git`).
- **`Exec(ctx, dir, args)`** — argv-style dispatch (`args[0]` =
  subcommand) over `execHandlers` covering ~35 porcelain + plumbing
  subcommands. Returns `ErrUnsupported` for unknown subcommands or
  unparsed flags so callers can fall back to a richer tier (ycode falls
  back to host git, then a container) or report a clear error.
  `ExecCommands()` lists the supported set for callers that register
  per-subcommand dispatch.

Files: `git.go` (typed core: clone/init/add/commit/status/log/push/pull/
fetch/branch/checkout/remotes/show/rev-parse), `merge.go` (ancestry +
`ffUpdate` fast-forward engine + `Merge`), `inspect.go` (merge-base,
rev-list, ls-files, blame, grep, diff, show-patch), `configcmd.go`,
`tag.go`, `worktree_ops.go` (reset, rm), `transport.go`, `exec.go`
(dispatcher + pull/clone argv delegates), `exec_read.go` / `exec_write.go` /
`exec_plumbing.go` (argv handlers).

Non-obvious load-bearing details:

- **`Pull` is hand-rolled, NOT go-git's `Worktree.Pull`** — the upstream
  helper integrates the remote's HEAD instead of the current branch's
  upstream, errors when the local branch is merely ahead, and refuses any
  unstaged change. Ours: fetch → upstream resolution (explicit arg >
  `branch.<name>.merge` > same-name) → ancestry analysis → `ffUpdate`.
- **`ffUpdate` preserves non-conflicting local changes** via a manual
  tree-diff apply with an overlap check, because go-git's `MergeReset`
  refuses all dirt and `HardReset` deletes untracked files. Conflicting
  local edits abort with a git-style "would be overwritten" error before
  anything mutates.
- **`transport.go` replaces the "file" protocol** with go-git's in-process
  server (stock go-git execs `git-upload-pack` for local paths — the exact
  dependency this repo must never have). Includes a dotgit-aware loader
  (serves non-bare repos) and a haves filter (go-git's server errors
  "object not found" when the client advertises local-only commits; real
  git servers ignore unknown haves). `InstalledFileTransportIsPureGo()` +
  `TestFileTransportIsPureGo` pin this — do not remove.
- **Diverged histories are an error by design.** No merge-with-conflicts,
  no rebase in the typed layer (Exec has a linear conflict-free replay
  for cherry-pick/rebase inherited from ycode; it returns ErrUnsupported
  on any conflict rather than leaving a half-applied state).
- **`config` branch.\* keys go through go-git's typed `Branches` map** —
  raw-section writes to typed sections (branch/remote/submodule/url) get
  silently dropped by go-git's `Marshal`. remote/submodule/url keys are
  rejected with a clear error.
- macOS tempdir symlinks (`/var` → `/private/var`) break naive
  `filepath.Rel` against go-git's resolved root — `relToRepoRoot` in
  `exec.go` handles it; keep using it for any new path-taking handler.

### History

`git/` was first built inside outpost (`internal/agent/git`), briefly lived
in the sh fork, and landed here when this repo was created as the shared
home for agent tools. The argv `exec_*.go` layer and its tests came from
ycode's `internal/runtime/toolexec/git_native*.go` (3-tier git tool); the
typed layer and its tests from outpost. ycode's e2e suite
(`git_e2e_test.go`) stayed in ycode and exercises this package through
ycode's executor — run it after changing argv-handler behavior.

## Architecture: tool/ + cmds/

`tool/` is the framework (see package docs): Tool registry,
RunContext (tools NEVER read os.Stdin/Getwd/Environ — the embedding
shell owns those; every fs operand goes through rc.Path), pflag-based
strict GNU flags, automatic --help/--version, and the contract error
helpers (UsageError, NotSupported). `cmds/<name>/` is one package per
command (`package <name>cmd`), init-registered; `cmds/all` blank-
imports the full set; `cmds/internal/hashenc` is the shared
checksum/encoding engine. `cmd/coreutils` is the multicall binary
(argv[0] dispatch + `coreutils <tool>`). Conventions every new tool
follows: the basename exemplar's shape (cmd.Run wired in init to avoid
init cycles), table tests with output captured after Run, unix-only
behavior behind build tags with clear Windows errors, GNU flags with
no long form pre-parsed manually (never invent long names), numeric
shorthands (-NUM) pre-scanned before pflag. Repo convention: usage
errors exit 2 even where GNU uses 1 (documented deviation).

## Roadmap (agreed 2026-06)

1. ~~git relocation~~ (done — this package).
2. ~~`tool/` framework + Phase A userland~~ (done — 74 commands per
   docs/commands.md Phase A, incl. the grep/find/diff/cmp/tar/gzip/
   strings/hexdump/which extensions; sed/xargs/ps remain Phase C).
3. Phase B: the GNU-manual complement (printf, test, expr, od, dd, …
   per docs/commands.md).
4. ~~`shell/` adapter~~ (done — `shell/Handler()` / `HandlerFunc()` is an
   `interp.ExecHandler` middleware for `mvdan.cc/sh/v3` that dispatches any
   argv[0] naming a registered `tool.Tool` to `Tool.Run`, else falls through
   to PATH. Precedence is **pure-Go first**; a host opts out by simply not
   wiring it — that is how `bashy`'s AgentOS shell turns it on while the
   `bash` drop-in leaves it off. The adapter imports sh; sh never imports
   coreutils.)
5. ~~`cmd/coreutils/` multicall binary~~ (done — busybox-style; the
   `multicall/` package factors out Resolve/Dispatch/Main so bashy and any
   other front-end reuse the same argv[0] dispatch).

Skip-with-clear-error tier (don't implement): chroot, mkfifo/mknod,
who/users/pinky, dircolors, ptx, csplit, tsort, factor, stdbuf, nice,
chcon/runcon.

## AgentOS hub

Beyond the GNU userland, coreutils is the shared **AgentOS tool hub**: one
registry, three consumption surfaces, imported by bashy/ycode/outpost.

- `shell/` — the `interp.ExecHandler` adapter above (in-process; the fast
  lane for Go hosts).
- `multicall/` — busybox argv[0] dispatch (any non-Go agent execs
  `coreutils <tool>` / a symlinked name).
- `mcp/` — an MCP server (`NewServer` / `ServeStdio`, over
  `github.com/modelcontextprotocol/go-sdk`) exposing the registry to non-Go
  agents via generic `list_tools` / `run_tool` meta-tools; `coreutils mcp`
  starts it over stdio.
- `pkg/` — importable pure-Go engines relocated from ycode so one
  implementation serves every host: `pkg/treesitter` (AST symbols/search,
  gotreesitter, no cgo), `pkg/repomap` (token-budgeted file→symbol map),
  `pkg/codegraph` (gfy-backed graph — only its importer pulls gfy's
  document-parsing deps; the bare binary stays free of them), and
  `pkg/weave` + `pkg/weavecli` (the filesystem-based multi-agent workspace
  orchestrator — pure-filesystem, depends only on weavecli/cobra/pty, no
  Gitea/loom; `NewWeaveCmd()` is the host-agnostic entry point), and
  `pkg/binmgr` (the shared managed-external-binary mechanism: `Ensure(Tool)`
  downloads → sha256-verifies → caches a per-platform release binary, and
  `Start`/`Launch`/`Process.Stop` supervise it with an optional health probe.
  Stdlib-only. `GitHubSpec` resolves GitHub releases; `URLSpec` resolves
  non-GitHub vendors (e.g. act_runner on dl.gitea.com). Both bashy — the "OS of
  binaries" host — and outpost — the lean mesh supervisor — call it in-process to
  run wrapped tools (loom/Gitea, Zot, SeaweedFS, Kopia, **act_runner** — the
  Gitea CI executor, `external/actrunner`) without compiling those heavy binaries
  into either; it is the download half that complements `external/`'s
  exec-an-already-present-binary wrappers. See
  dhnt/docs/external-binary-builtins.md).
- `cmds/yc` — the code-intelligence verbs (list-symbols / search-symbols /
  find-references / repo-map / ast-query — flat, no yc prefix) over those engines, reachable through all three surfaces.
- `cmds/graph` — the graph verbs (flat, `graph-` stem). Two layers:
  - **Read (code-graph):** graph-build / graph-stats / graph-neighbors /
    graph-impact / graph-path / graph-hotspots / graph-query over `pkg/codegraph`.
    Fully structural + model-free; a bashy-owned disk cache
    (`.agents/bashy/graph.json`) with mtime staleness makes repeated calls cheap.
    `graph_sha` in the `bashy-graph-v1` envelope is a **source** fingerprint
    (relpath|size|mtime), NOT a graph-content hash — gfy assigns node-ids/edge-order
    non-deterministically per build, so only a source fingerprint is reproducible
    across rebuilds (the caching premise is that the graph is a pure function of
    source).
  - **Write (contribution layer, `contrib*.go`):** graph-note / graph-link /
    graph-observe / graph-forget (write) + graph-recall / graph-notes /
    graph-pitfalls (read) — the "agentic wiki, built by agents, for agents." A
    durable, append-only JSONL store at the repo root
    (`.agents/bashy/graph/contrib.jsonl`, resolved by walking up to `.git` so all
    agents in the repo share one store). O_APPEND = concurrency-safe multi-agent
    writes without a lock; reads replay the log applying forgets (soft-delete) +
    last-writer-wins per deterministic content id. Deliberately SEPARATE from the
    code-graph cache so contributions survive a rebuild (the clobber hazard).
    Provenance (`by`/`at`/`source`/`confidence`/`episode`) on every record. Pure
    stdlib — no new deps, no gfy pull (bare binary stays clean). `bashy-graph-contrib-v1`
    envelope. See `dhnt/docs/repo-knowledge-graph-design.md` (P1) +
    `dhnt/docs/execution-knowledge-graph-design.md`.
  - **Placement invariant:** `cmds/graph` is **NOT in `cmds/all`** (the read layer
    pulls gfy's document-parsing deps, which must stay out of the bare
    `cmd/coreutils` multicall binary — verified: `go list -deps ./cmd/coreutils`
    has zero gfy pkgs). Blank-imported only by bashy's `internal/agentos`, so the
    verbs reach the `bashy graph-*` front door + in-shell ExecHandler while the bare
    binary and the `bash` drop-in stay gfy-free. See
    `dhnt/docs/bashy-code-graph-agentic-feature.md`.

### Embedded forks: ollama + podman (AgentOS Phase 4, 2026-06-27)

Distinct from the download-and-run tools above, **ollama and podman are embedded
forks we own** — for version/issue control and identical cross-platform behavior:

- **`external/ollama`** — in-process ollama server + embedded runner; consumes
  the `qiangli/ollama` fork (`external/ollama/src` submodule, `replace
  github.com/ollama/ollama => ./external/ollama/src`). `NewManagedOllamaCmd()` is
  the **isolated** front-door bashy mounts (own bashy-owned port, never 11434;
  models under `~/.agents/bashy/ollama`).
- **`external/podman/engine`** — the in-process libpod/buildah engine + machine
  lifecycle (relocated from ycode's `internal/container`), consuming the
  `qiangli/podman` fork (`external/podman/src` submodule, `replace
  go.podman.io/podman/v6 => ./external/podman/src`) and the `pkg/oci` wrapper
  module (`replace github.com/qiangli/coreutils/pkg/oci => ./pkg/oci`).
  `engine.NewPodmanCmd()` is the front-door: pass-through to the embedded podman
  with `CONTAINER_HOST` pinned to an **isolated `bashy` machine** (never the
  host/ycode engine). `$BASHY_PODMAN_SYSTEM=1` defers to a host podman. The helper
  binaries (podman/vfkit/gvproxy) build via `scripts/embed-{podman,vfkit,
  gvproxy}.sh` into gitignored `*_embed/*.gz` blobs, consumed only under the
  `embed_*` build tags (default build uses the stub → host/PATH fallback).
  **This pulls the go floor to 1.26.2** (all consumers must match).

Hosts: `bashy` (the AgentOS shell binary) wires `shell.Handler()` so the
whole userland + code-intel verbs run in-process, and mounts `pkg/weave` as `bashy weave`
(`ycode weave` is now a deprecation stub pointing here). ycode's loom MCP
substrate (`pkg/loom`, gitea backend — separate from `pkg/weave`) routes its
client-side git through `coreutils/git` (pure-Go-first, host-git fallback).

## Conventions

- Sibling-path consumers: outpost and ycode use
  `replace github.com/qiangli/coreutils => ../coreutils` (umbrella mount
  `dhnt/coreutils`, or a flat standalone sibling — same rule as the other
  qiangli/* deps; see the dhnt umbrella CLAUDE.md).
- New tools land with: implementation + table tests + a `--help` text +
  README catalog line. Cross-platform CI (ubuntu/macos/windows) must pass —
  the windows leg is the product, not an afterthought.
- Dependency budget is deliberately tight: go-git (for `git/`), pflag, and
  the stdlib for the userland core. The AgentOS hub adds, used only by the
  packages that need them: `mvdan.cc/sh/v3` (`shell/`),
  `github.com/modelcontextprotocol/go-sdk` (`mcp/`), `gotreesitter`
  (`pkg/treesitter`, pure Go), and `gfy` (`pkg/codegraph` only — kept out of
  the bare binary by per-import compilation), and `github.com/rjeczalik/notify`
  (`pkg/mirror` only — **MIT**, cgo-free, native-recursive cross-platform fs
  watching; the third-party lib Syncthing forked, used here directly, no
  Syncthing code). Adding a dependency needs a written justification in the PR.
  **License rule (bashy+coreutils ship as a bundled barebone "OS"):** compiled-in
  deps must be **permissive (MIT/BSD/Apache)** — anything whose license would
  *propagate* to the project (GPL/MPL copyleft) is out. External tools we only
  download+run (not link) sit outside this — they're separate binaries on their
  own license. cgo is avoided in core (releases are `CGO_ENABLED=0`) but can be
  relaxed case-by-case for non-core/external pieces when no pure-Go option exists.

## pkg/mirror + external/rclone — the directory mirror

`pkg/mirror` is a continuous one-way directory mirror (node B keeps a live replica
of a dir on node A). It reuses Syncthing's *architecture* — a recursive fs watcher
+ a periodic full-scan backstop + delta transfer — from **all-permissive parts and
no Syncthing code**: `rjeczalik/notify` (MIT) for the recursive watch, a
binmgr-managed `rclone` (MIT, `external/rclone`) for `rclone sync` (delta + mirror
semantics), and our own debounce/backstop/lifecycle orchestration. `bashy mirror
--source <dir> --dest <rclone-target>`; over the mesh, the replica runs `bashy
rclone serve webdav <dir>` exposed as a mesh service and the source points `--dest`
at it. `external/rclone` is also a transparent passthrough (`bashy rclone …`).
