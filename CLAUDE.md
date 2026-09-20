# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Overview

`coreutils` is the **certified POSIX package** of the bashy userland: the
116 POSIX-required utility names ∪ the GNU coreutils programs, implemented as
pure-Go applets with no cgo and no shell-out, so agentic consumers get **one
identical toolset on every platform** — the load-bearing case is Windows
hosts with nothing installed. It is a library first: consumers embed packages
directly (bashy, outpost, ycode, yoke); the busybox-style multicall binary
(`cmd/coreutils`) is the certification SUT and links exactly this module.

**Since Sprint 208 (2026-09-18) this repo holds ONLY the required set.**
Everything agentic — the AgentOS hub (`pkg/{fleet,kb,bus,meet,weave,dag,…}`),
the front-door verbs, the non-POSIX applets (`ast`, `browser`, `fetch`,
`jq`, `tar`, `tree`, `which`, …), the pure-Go `git/` client, the managed
externals and embedded engines under `external/`, the MCP server and the
Command Atlas — lives in the flat sibling
[**yoke**](https://github.com/qiangli/yoke) (`github.com/qiangli/yoke`), which
imports this module and never the reverse. Membership is ONE rule, recorded
row by row in the umbrella's `docs/coreutils-required-set.tsv`: a package
stays here iff it is POSIX-required or GNU coreutils
(`atlas.PosixRequired() ∪ atlas.GNUCoreutilsUpstream()`), or is in that
set's import closure. Two seams keep yoke out of the closure — both are
package-level VARIABLES set by yoke at init, not interfaces:
`weavecli.DetectTool` (the agent-CLI marker table lives in yoke's `fleet`)
and `schedule.DefaultAdmission` (the LLM-budget gate lives in yoke's
`llmbudget`). Do not add a third without the same justification.

**Change policy:** this package is STABLE — bug fixes only. The only
feature-level change is the reference coordinate moving (POSIX.1-2016 /
GNU coreutils 9.11 / GNU Bash 5.3). Agentic work lands in yoke; a change
that needs an edit here is a bug fix against the pinned coordinate or it is
out of scope.

This repo is OSS (MIT) and is consumed by other OSS repos. Three hard rules:

The authoritative hierarchy for deciding command behavior is
[`docs/reference-policy.md`](docs/reference-policy.md). In particular, POSIX.1
Issue 7 (2016 Edition) controls certification behavior, GNU Coreutils 9.11
controls extensions only for commands that GNU Coreutils actually ships, and
non-Coreutils commands such as `ps` use their own official upstream reference.

1. **Never port GNU source.** GNU coreutils is GPLv3. Implement behavior
   from the GNU manual / POSIX documentation, or adapt code from the
   permissive prior-art clones below — never from GPL code.
2. **Never shell out.** No tool spawns programs to *implement its own
   behavior* (cat never execs /bin/cat). If pure Go can't do it, the tool
   returns a clear error naming what's unsupported. Partial flag coverage
   is fine — silent approximation is not. The one documented exception:
   command wrappers whose upstream-documented purpose IS running the
   COMMAND operand (env, timeout, time, watch, xargs) spawn that command
   directly, exactly as the GNU binary does — see docs/commands.md's
   NO-list preamble. The **POSIX external providers** are the second,
   narrower exception and are explicitly not Go implementations: the
   multicall owns ten POSIX-required names (m4, man, ctags, ar, nm, strip,
   ex, vi, lp, localedef) and dispatches to a locally built,
   provenance-checked copy of the upstream program. They exist so a
   "bashy-only" certification arm stops silently measuring the host's
   `$PATH`. Pure-Go applets exclusively own `make`, `bc`, `ed`, `patch`,
   `mail`/`mailx`, and `talk`; they have no external-provider pins or fallback. See
   docs/posix-external-providers.md.
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

Local clones of permissive reimplementations (aict, guonaihong/coreutils,
u-root — copy/adapt; microsoft + uutils coreutils — reference only). Table,
provenance rules and the adaptation checklist: `docs/prior-art.md`.
**Conformance is judged against the original command's official
documentation, never against prior art.** Anything copied carries a
provenance header, an entry in `THIRD_PARTY_LICENSES.md`, and tests.

## Build & test

No Makefile — `DAG.md` at the repo root is the agent-first equivalent,
runnable with the `bashy dag` task runner (`bashy dag --list` / `build` /
`test` / `dist`). The equivalent plain-go commands:

```bash
go build -o bin/coreutils ./cmd/coreutils   # the multicall binary (the cert SUT)

# The whole tree is in scope: pure-Go applets, no engines, no externals
# (those are yoke's). No submodules to hydrate.
go vet  ./...
go test ./...

go test ./cmds/ls/       # one command's tests
go test -short ./...     # skips the slower e2e-ish cases

# THE CROSS-OS GATE — run this alongside `go test`, not instead of it.
# ~6-8s warm. Compiles every package AND its tests for each target GOOS.
scripts/crossvet.sh      # or: bashy dag crossvet
```

**CI runs the suite through a RATCHET, not `go test` directly.** Tests for open
stories are committed red on purpose (the mv trailing-slash story's contract
says so in as many words), so a plain gate is permanently red and therefore
reports nothing — a real break is indistinguishable from the known ones.
`scripts/ci-test-gate.sh` compares the failing set against
`test/known-failures.txt`, scoped per GOOS because these failures are NOT
portable (mv and pax pass on darwin and fail on linux). It fails on a NEW
failure, on a baseline entry that has started passing (delete its line — the
baseline only shrinks), and ALWAYS on a package that fails without a failing
test. That last rule is why the gate exists: coreutils CI spent weeks failing
at the build step over an uncloned `../filebrowser` sibling, so no test ran at
all and nobody could tell. Every baseline entry names a story; nothing goes in
without one.

When reproducing a Linux-only failure locally, run the container as an ordinary
user (`--user`), not as root: several tests establish their failure condition
with `chmod 0o000`, which root ignores, so a root container turns them red and
invites a baseline entry for a bug that does not exist. The ratchet caught
exactly that on its first run.

**The bashy apps console has BROWSER end-to-end tests, and UI changes need
them.** `pkg/webconsole`'s other tests read the SERVED BYTES: they can assert a
string is present and nothing more — they cannot see the cascade, the DOM, or a
script that throws. Four UI defects shipped in one day through that gap, the
worst being a fix in the pairing section that stopped the Settings dialog from
opening at all while every byte-level test still passed.
`pkg/webconsole/console_dom_test.go` drives a real Chrome over the launcher,
Settings (both pairing states), the login toggle, the pairing QR, the background
swatches, the Files return control, and every panel — asserting no page threw.
Run it with `go test ./pkg/webconsole -tags verifydom`; CI runs it on the Linux
leg, whose runner image ships Chrome. It is tag-gated so a contributor without a
browser is not blocked.

**A change is gated by BOTH `go test` and `scripts/crossvet.sh`.** `go test` on
your host structurally cannot see a build-tag break: a `//go:build !windows`
file never compiles for windows, so referencing it from an untagged `_test.go`
passes a darwin suite and fails the windows leg. Windows is a shipping target
(the whole point is working where system tools do not), so a host-only green is
not a green. `scripts/crossvet.sh` is the single implementation — the pre-push
hook execs the same script, so the manual and automatic gates cannot drift.
Install the hook once per clone: `git config core.hooksPath scripts/hooks`.

**Registered means tested.** Every command package imported by `cmds/all` must
contain package-local behavioral tests. `scripts/applet-test-coverage.sh` is a
fail-closed release check and is invoked by `crossvet.sh`; a command without a
test must be removed from `cmds/all` (and therefore from multicall listing/help)
until coverage lands. A registration-only assertion is not sufficient for an
alias: execute the alias through its own registered `Tool` so its name,
parsing, diagnostics, and dispatch are covered.

Tests are hermetic: no network, no system git required. `reference/`
(like `priorart/`) is gitignored local source — GNU coreutils, bash,
uutils, hyperfine — kept for conformance/benchmark reference;
`cmds/perfbench` (with its `cmd/perfbench` main) is the dev-only
bashy-vs-GNU A/B perf harness (out of `cmds/all`; see the umbrella's
fidelity-perf harness spec). The certification build proves the boundary
mechanically: `go list -deps ./cmd/coreutils` must name no
`github.com/qiangli/yoke` package.

**Foreign-suite safety:** do not run the full uutils suite natively. Its
adversarial cases include infinite devices and root-equivalent recursive
operands. On 2026-07-24, `split -n /dev/zero` and
`sort /dev/random <missing>` exhausted memory while `chmod`/`chgrp
--preserve-root` variants bypassed string-only root guards. Use only a
disposable non-root container/VM with hard memory/PID/time limits and no
host-root/home mount. See `../docs/conformance-test-landmines.md`; never set the
bashy runner's unsafe overrides on a fleet or steward host.

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
imports the REQUIRED set (141 command packages; `cmds/all/all.go` is the
certified inventory, `docs/commands.md` the plan, and yoke's `pkg/atlas`
the Command Atlas metadata table — its coverage test, which runs in yoke
over yoke's `cmds/all` (which carries this one), fails by name if a
registered tool lacks an entry; keep all three in sync when adding a tool);
shared engines live under `cmds/internal/`
(`hashenc` — checksums/encodings; `session` — utmp session records for
who/users/pinky).
`cmd/coreutils` is the multicall binary (argv[0] dispatch +
`coreutils <tool>`; multicall only — the MCP front is `yoke mcp`). The
agentic applets and the two deliberately-unlisted verbs (`graph`,
`foreman`) are yoke's; yoke's `cmds/all` blank-imports this one and adds
them, so a consumer that wants everything imports
`github.com/qiangli/yoke/cmds/all`. Conventions every new tool
follows: the basename exemplar's shape (cmd.Run wired in init to avoid
init cycles), table tests with output captured after Run, unix-only
behavior behind build tags with clear Windows errors, GNU flags with
no long form pre-parsed manually (never invent long names), numeric
shorthands (-NUM) pre-scanned before pflag. Repo convention: usage
errors exit 2 even where GNU uses 1 (documented deviation).

## Roadmap status

History through 2026-07 (phases A/B/C, the uutils option-parity sprint, the
`env` option-parsing defect): `docs/roadmap-status-2026-07.md`. Live truth:
`docs/commands.md` (incl. the canonical **NO list** — recognized-but-NO names
get a clear error naming command, reason and alternative, never a silent
fallthrough) + `cmds/all/all.go`. Work items come from `bashy sprint`.

## What moved to yoke (Sprint 208)

Read yoke's `CLAUDE.md` for the AgentOS hub, `git/`, `external/`,
`pkg/mirror`, the Command Atlas and every front-door verb. In one line each,
so a reader of THIS file knows where to look:

- `git/` — the pure-Go git client (typed API + `Exec`); `outpost git`, ycode's
  native git tier.
- `mcp/` — the MCP server over the tool registry; `yoke mcp` / `bashy mcp`.
- `pkg/atlas` — the Command Atlas (group/tier/caps/origin per command, the
  116-name and GNU-108 lists, the package census). Data about this repo, owned
  by yoke because it catalogs yoke's surface too.
- `pkg/{fleet,kb,bus,room,meet,weave,dag,foreman,chat,skills,craft,secrets,
  binmgr,…}` — the hub.
- `cmds/{ast,browser,fetch,jq,tar,gzip,tree,which,watch,tokens,tz,ntp,cal,
  hexdump,clip,duration,why,atq,atrm,graph,foreman,resources}` — the
  non-POSIX applets and front-door verbs.
- `external/` — managed externals, toolchain provisioners, the ollama and
  podman forks (+ their submodules), the otel stack module, `pkg/oci`.

The stores stayed: `docs/todo/` (this repo's story ledger — Story-IDs in
old commits resolve here) and `docs/kb/`.

## Conventions

- Sibling-path consumers: yoke, bashy, outpost and ycode use
  `replace github.com/qiangli/coreutils => ../coreutils` (umbrella mount
  `dhnt/coreutils`, or a flat standalone sibling — same rule as the other
  qiangli/* deps; see the dhnt umbrella CLAUDE.md). This module replaces
  only `../sh` and its own `./third_party/goawk`.
- New tools land with: implementation + table tests + a `--help` text +
  README catalog line. Cross-platform CI (ubuntu/macos/windows) must pass —
  the windows leg is the product, not an afterthought. Catch its compile
  breaks at MERGE time, not push time: `scripts/crossvet.sh` (equivalently
  `bashy dag crossvet`) — GOOS=windows/linux/darwin `go vet` over the CI
  scope plus an aix canary; vet cross-typechecks tests too, so no Windows
  box is needed. That script is the one implementation; the pre-push hook
  execs it (`git config core.hooksPath scripts/hooks` installs the hook),
  so there is no second copy to drift. The recurring offenders are a
  unix-only type (`syscall.Stat_t` etc.) in an untagged `_test.go`, and an
  untagged test calling a `//go:build !windows` helper.
- Dependency budget is deliberately tight: pflag/cobra, goawk (`cmds/awk`),
  robfig/cron (`pkg/schedule`), gopsutil, x/{crypto,sys,term,text} and the
  stdlib. Every agentic dependency (go-git, gfy, chromedp, tree-sitter,
  gojq, tiktoken, MCP, podman/ollama forks, …) left with its package for
  yoke. Adding a dependency here needs a written justification in the PR
  AND a required applet that cannot ship without it.
  **License rule (bashy+coreutils ship as a bundled barebone "OS"):** compiled-in
  deps must be **permissive (MIT/BSD/Apache)** — anything whose license would
  *propagate* to the project (GPL/MPL copyleft) is out. External tools we only
  download+run (not link) sit outside this — they're separate binaries on their
  own license. cgo is avoided in core (releases are `CGO_ENABLED=0`) but can be
  relaxed case-by-case for non-core/external pieces when no pure-Go option exists.

