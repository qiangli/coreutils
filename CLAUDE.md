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
   from the GNU manual / POSIX documentation only.
2. **Never shell out.** No `os/exec` anywhere. If pure Go can't do it, the
   tool returns a clear error naming what's unsupported. Partial flag
   coverage is fine — silent approximation is not.

## The agent contract

Every tool: deterministic output (`LC_ALL=C` semantics, no locale/color/
terminal variance), GNU exit-code conventions (0 ok, 1 failure, 2 usage),
unsupported flags fail loudly naming the flag. This is documented in
README.md and is the review bar for every PR.

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
