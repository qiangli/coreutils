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
  messages; embedders override it at init (`outpost git`, `yc git`).
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

## Roadmap (agreed 2026-06)

1. ~~git relocation~~ (done — this package).
2. `tool/` framework: Tool interface + registry + strict-getopt helper
   (combined short flags, `--long`, unknown flag → exit 2 with the flag
   named), then the Tier-1 file/text tools (~30: ls, cat, cp, mv, rm,
   mkdir, touch, ln, chmod, head, tail, wc, sort, uniq, cut, tr, tee,
   basename, dirname, realpath, stat, du, df, mktemp, date, sleep, seq,
   env, uname, whoami, hostname, base64, sha256sum, timeout, truncate,
   split).
3. Agent-critical non-coreutils tools: grep, find, sed, xargs, diff, tar
   (decided in-scope — agents use these more than half of coreutils
   proper).
4. `shell/` adapter: `interp.ExecHandler` for `mvdan.cc/sh/v3` wiring the
   registry into outpost's matrix shell and ycode's shell runner.
   Precedence: **pure-Go first**, real binaries only via an explicit
   escape hatch — uniformity is the product. The adapter imports sh; sh
   never imports coreutils.
5. `cmd/coreutils/` multicall binary (busybox-style).

Skip-with-clear-error tier (don't implement): chroot, mkfifo/mknod,
who/users/pinky, dircolors, ptx, csplit, tsort, factor, stdbuf, nice,
chcon/runcon.

## Conventions

- Sibling-path consumers: outpost and ycode use
  `replace github.com/qiangli/coreutils => ../coreutils` (umbrella mount
  `dhnt/coreutils`, or a flat standalone sibling — same rule as the other
  qiangli/* deps; see the dhnt umbrella CLAUDE.md).
- New tools land with: implementation + table tests + a `--help` text +
  README catalog line. Cross-platform CI (ubuntu/macos/windows) must pass —
  the windows leg is the product, not an afterthought.
- Dependency budget is deliberately tight: go-git (for `git/`) and the
  stdlib. Adding a dependency needs a written justification in the PR.
