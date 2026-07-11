# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Overview

`coreutils` is a pure-Go agent userland: Unix-style tools (`git`, and over
time `ls`/`cat`/`sort`/…) implemented with no cgo and no shell-out, so
agentic consumers get **one identical toolset on every platform** — the
load-bearing case is Windows hosts with nothing installed. It is a library
first: consumers embed packages directly (bashy, outpost, ycode); the
busybox-style multicall binary (`cmd/coreutils`) is secondary.

This repo is OSS (MIT) and is consumed by other OSS repos. Two hard rules:

1. **Never port GNU source.** GNU coreutils is GPLv3. Implement behavior
   from the GNU manual / POSIX documentation, or adapt code from the
   permissive prior-art clones below — never from GPL code.
2. **Never shell out.** No tool spawns programs to *implement its own
   behavior* (cat never execs /bin/cat). If pure Go can't do it, the tool
   returns a clear error naming what's unsupported. Partial flag coverage
   is fine — silent approximation is not. The one documented exception:
   command wrappers whose upstream-documented purpose IS running the
   COMMAND operand (timeout, time, watch, xargs) spawn that command
   directly, exactly as the GNU binary does — see docs/commands.md's
   NO-list preamble.
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

No Makefile — `DAG.md` at the repo root is the agent-first equivalent,
runnable with the `bashy dag` task runner (`bashy dag --list` / `build` /
`test` / `dist`). The equivalent plain-go commands:

```bash
go build -o bin/coreutils ./cmd/coreutils   # the multicall binary

# CI scope — EXCLUDES the vendored external/ forks (ollama, podman):
# they pull cgo + platform backends and need hydrated submodules.
go vet  $(go list ./... | grep -v /external/)
go test $(go list ./... | grep -v /external/)

go test ./cmds/ls/       # one command's tests
go test -short ./...     # skips the slower e2e-ish cases
go test ./...            # full incl. external/ — unix + submodules only
```

Tests are hermetic: no network, no system git required. `reference/`
(like `priorart/`) is gitignored local source — GNU coreutils, bash,
uutils, hyperfine — kept for conformance/benchmark reference;
`cmds/perfbench` (with its `cmd/perfbench` main) is the dev-only
bashy-vs-GNU A/B perf harness (out of `cmds/all`; see the umbrella's
fidelity-perf harness spec).

## Architecture

### git/ — the pure-Go git client

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
strict GNU flags, automatic --help/--version, GNU-style long-option
abbreviation (unambiguous prefixes expand in `tool.Parse` before
pflag sees them — exact match wins, ambiguity is a GNU-format exit-2
error; `tool/abbrev_test.go`), and the contract error
helpers (UsageError, NotSupported). `cmds/<name>/` is one package per
command (`package <name>cmd`), init-registered; `cmds/all` blank-
imports the full set (135 commands — `cmds/all/all.go` is the shipped
inventory, `docs/commands.md` the plan, and `pkg/atlas` the Command
Atlas metadata table (group/tier/caps per command; its coverage test
fails by name if a registered tool lacks an entry); keep all three in
sync when adding a tool); shared engines live under `cmds/internal/`
(`hashenc` — checksums/encodings; `session` — utmp session records for
who/users/pinky).
`cmd/coreutils` is the multicall binary (argv[0] dispatch +
`coreutils <tool>`). Two commands are deliberately NOT in `cmds/all`:
`cmds/graph` (pulls gfy deps — see the placement invariant below) and
`cmds/foreman` (imports pkg/foreman → pkg/dag, whose tests blank-import
cmds/all — listing it would form an import cycle; bashy registers it
directly in internal/agentos). Conventions every new tool
follows: the basename exemplar's shape (cmd.Run wired in init to avoid
init cycles), table tests with output captured after Run, unix-only
behavior behind build tags with clear Windows errors, GNU flags with
no long form pre-parsed manually (never invent long names), numeric
shorthands (-NUM) pre-scanned before pflag. Repo convention: usage
errors exit 2 even where GNU uses 1 (documented deviation).

## Roadmap status (as of 2026-07)

Done: git relocation; the `tool/` framework + Phase A userland (incl. the
2026-07 file-utilities batch: dd, install, shred, mkfifo, mknod, chcon,
dircolors, dir/vdir); the `shell/` adapter (`shell/Handler()` /
`HandlerFunc()` — an `interp.ExecHandler` middleware for `mvdan.cc/sh/v3`
that dispatches any argv[0] naming a registered `tool.Tool` to `Tool.Run`,
else falls through to PATH; precedence is **pure-Go first**, a host opts
out by not wiring it — bashy's AgentOS shell turns it on, the `bash`
drop-in leaves it off; the adapter imports sh, sh never imports
coreutils); the `cmd/coreutils` multicall binary (the `multicall/`
package factors out Resolve/Dispatch/Main so bashy reuses the same
argv[0] dispatch); and much of the former Phase C — sed, xargs, awk
(goawk), jq (gojq), time/timeout, watch, tree, and the agentic extras
(at/atq/atrm/batch/crontab, browser, fetch, clip, tokens, duration, tz,
ntp, cal, tsort).

Phase B is essentially complete (2026-07): expr, od, nl, fold,
expand/unexpand, cksum, b2sum, basenc, csplit, numfmt, nproc, arch,
tail -f (polling follow), plus the sh-utils sweep (who/users/pinky,
pr, ptx, factor, stdbuf, stty, hexdump, yes, which, …) all shipped.
Remaining (per docs/commands.md): printf, test/[. The 2026-07-07
**uutils option-parity sprint** then closed flag/option gaps against
`reference/uutils-coreutils` across the whole userland (ls, df, du,
ln, tail, sort, stat, checksums, …) — see
`docs/uutils-parity-sprint-2026-07-07.md` for what landed and
`docs/bashy-uutils-option-comparison.md` for the final per-command
gap status. Conformance is still judged against GNU/POSIX docs; the
uutils reference was parity guidance, never translated source.

The not-supported tier is docs/commands.md's **NO list** (canonical —
grouped by reason: needs-exec, unix-only machinery, low agent value,
sysadmin out-of-scope). Recognized-but-NO names get a clear error naming
the command, the reason, and the nearest alternative — never a silent
fallthrough. Note the list evolves: several early "NO ↻ revisit" entries
(timeout, time) and former skips (mkfifo, mknod, dircolors, chcon, tsort)
have since shipped — trust docs/commands.md + `cmds/all/all.go` over any
older skip list.

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

  Newer engines, same pattern (one impl, every host; each pulls its deps
  only into its importers): `pkg/dag` (agent-first task runner — the
  Makefile replacement behind `bashy dag`; this repo's own `DAG.md` +
  `dag-p*.md` are its task files), `pkg/foreman` (process manager over
  dag), `pkg/schedule` (bashy's modern cron, robfig/cron), `pkg/sdlc`
  (the label-driven SDLC control plane), `pkg/secrets` (the
  cloudbox-vault client behind `bashy secrets`), `pkg/skills` (the dhnt
  skill-CNL mechanism), `pkg/kb` (the host-scope shared knowledge base
  behind `bashy kb`: OKF-style wiki pages under `~/.bashy/kb` — the
  collective memory of all agents on the host across all repos;
  reconcile-on-write, supersede-not-delete, candidate→validated ladder;
  weave/foreman auto-inject its top matches at spawn — see
  dhnt/docs/kb-host-knowledge-base-design.md. The `sources` + `transfer`
  verbs structure agent-to-agent knowledge transfer: `sources` probes the
  private-memory stores on the host (Claude memory / ycode memex / weave
  memory / repo graph — best-effort, read-only), `transfer` prints the
  retro-shaped checklist (sources + `xfer:<source>` tag counts + related
  pages + literal commands); both write NOTHING — the hard rule "kb reads
  foreign stores, never writes them" is pinned by tests, and the judgment
  half lives in bashy's embedded `knowledge-transfer` skill — see
  dhnt/docs/kb-knowledge-transfer-design.md), `pkg/chat` + `pkg/ollm` (agent chat / Ollama
  client isolation), `pkg/browser` + `pkg/webinspect` (browser automation
  for `cmds/browser`/`fetch`, three modes: `probe` = attach to a Chrome on
  `--remote-debugging-port`, `solo` = launch a headless Chrome, and `live`
  = the MV3 Chrome extension + WebSocket hub that drives the user's real
  logged-in Chrome — `pkg/browser/live`, migrated verbatim from ycode
  Apache-2.0, run via `bashy browser hub` + `--mode live`; the whole browser
  feature now lives here, not ycode), `pkg/coopauth` (the ONE
  shared cloudbox/outpost cooperative-auth impl), `pkg/bre` (POSIX BRE →
  Go regexp, shared by grep/sed), `pkg/ignore` (opt-in agentic path
  filter shared by grep/find), `pkg/mirror` (see below), `pkg/timezones`,
  `pkg/jobs`, `pkg/agentcmd`, `pkg/oci` (separate module wrapped by the
  podman engine).
- `cmds/ast` — the `ast` code-intelligence command with subcommands
  (`ast symbols` list / `ast search` / `ast refs` / `ast map` repo map /
  `ast query` tree-sitter S-expr) over the `pkg/{treesitter,repomap}` engines,
  reachable through all three surfaces (the former flat list-symbols/…/ast-query
  verbs were collapsed 2026-07, mirroring `graph`).
- `cmds/graph` — the `graph` command with subcommands (one registered tool
  that sub-dispatches `graph <sub>`; the former flat `graph-*` verbs were
  collapsed 2026-07). Two layers:
  - **Read (code-graph):** graph build / graph stats / graph neighbors /
    graph impact / graph path / graph hotspots / graph query over `pkg/codegraph`.
    Fully structural + model-free; a bashy-owned disk cache
    (`.agents/bashy/graph.json`) with mtime staleness makes repeated calls cheap.
    `graph_sha` in the `bashy-graph-v1` envelope is a **source** fingerprint
    (relpath|size|mtime), NOT a graph-content hash — gfy assigns node-ids/edge-order
    non-deterministically per build, so only a source fingerprint is reproducible
    across rebuilds (the caching premise is that the graph is a pure function of
    source).
  - **Write (contribution layer, `contrib*.go`):** graph note / graph link /
    graph observe / graph forget (write) + graph recall / graph notes /
    graph pitfalls (read) — the "agentic wiki, built by agents, for agents." A
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
    `graph` verb reaches the `bashy graph …` front door + in-shell ExecHandler while
    the bare binary and the `bash` drop-in stay gfy-free. See
    `dhnt/docs/bashy-code-graph-agentic-feature.md`.

### external/ — managed externals + embedded forks

`external/` holds three kinds of packages. First, **binmgr-managed
externals** — thin wrappers that `pkg/binmgr`-provision (download →
sha256-verify → cache → exec/supervise) a pinned release, so bashy/outpost
run them without linking them: services (`loom`/Gitea, `actrunner`,
`zot`, `seaweedfs`, `kopia`, `rclone`, `registry`-catalogued k8s/cloud
CLIs), CLIs (`gh`, `helm`, `kubectl`, `act`, `mise`, `curlbin`,
`gitscm` — real git / MinGit on Windows, `meshagent` — execs the outpost
mesh agent without linking it), and **toolchain provisioners**
(`gotoolchain`, `node`, `python` via uv, `rust` via rustup, `java`/Temurin
+ Maven, `clang`, `cmake`) so `bashy <tool>` works on a bare node.
Second, dhnt **front-doors**: `sphere` (the sphere tier) and `tessaro`
(account pairing). Third — distinct from download-and-run — **ollama and
podman are embedded forks we own**, for version/issue control and
identical cross-platform behavior:

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
  **The forks pull the go floor up (currently 1.26.4)** — all consumers
  must match.

The forks are why the canonical test scope excludes `external/` (see
Build & test): they pull cgo + platform backends (MLX, btrfs) and are
upstream's to test; the CI/Windows scope must stay green without them.

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
  the windows leg is the product, not an afterthought. Catch its compile
  breaks BEFORE pushing: `bashy dag crossvet` (GOOS=windows/linux/darwin
  `go vet` over the CI scope — vet cross-typechecks tests too, no Windows
  box needed), enforced automatically once the committed hook is installed:
  `git config core.hooksPath scripts/hooks`. The recurring offender is a
  unix-only type (`syscall.Stat_t` etc.) in an untagged `_test.go`.
- Dependency budget is deliberately tight: go-git (for `git/`), pflag, and
  the stdlib for the userland core. The AgentOS hub adds deps **used only
  by the packages that need them** (per-import compilation keeps them out
  of the bare binary): `mvdan.cc/sh/v3` (`shell/`, `pkg/dag`),
  `modelcontextprotocol/go-sdk` (`mcp/`), `gotreesitter` (`pkg/treesitter`,
  pure Go), `gfy` (`pkg/codegraph` only), `rjeczalik/notify` (`pkg/mirror`
  only — MIT, cgo-free recursive fs watching; the lib Syncthing forked,
  used directly, no Syncthing code), `benhoyt/goawk` (`cmds/awk`),
  `itchyny/gojq` (`cmds/jq`), `chromedp` (`pkg/browser`),
  `tiktoken-go/tokenizer` (`cmds/tokens`), `robfig/cron` (`pkg/schedule`),
  cobra (the front-door verb trees), and `github.com/dhnt/dhnt`
  (`pkg/skills` — a versioned dep, NOT a submodule). Adding a dependency
  needs a written justification in the PR.
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
