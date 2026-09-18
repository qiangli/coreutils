# coreutils

The **certified POSIX package** of bashy's pure-Go agent userland: the 116
POSIX-required utility names ∪ the GNU coreutils programs, as Go applets
with identical behavior on every major platform (Linux, macOS, Windows).

Everything agentic — the AgentOS hub, the non-POSIX applets (`tar`, `jq`,
`tree`, `which`, `browser`, …), the pure-Go `git` client, managed externals
and embedded engines — lives in the flat sibling
[yoke](https://github.com/qiangli/yoke), which imports this module and never
the reverse (Sprint 208, 2026-09-18). This package is stable: bug fixes
only, feature-level change only when the reference coordinate moves.

This repo exists for **agents**, not humans. Agentic tools (shells,
tool executors, automation harnesses) need a predictable command
vocabulary — `ls`, `cat`, `sort`, `git`, … — that behaves the same
whether the host is a Linux CI runner, a developer's Mac, or a Windows
box with nothing installed. Re-implementing the tools in pure Go (no
cgo, no shell-out) is what makes that possible: the same code path
everywhere, embedded directly into the consuming process.

It pairs with [qiangli/sh](https://github.com/qiangli/sh) (a fork of
`mvdan.cc/sh/v3` with an in-process bash interpreter): sh provides the
shell, coreutils provides the userland. Wired together through the
interpreter's `ExecHandler`, an agent gets a portable Unix-like
environment in a single Go process.

## POSIX certification boundary

This repository is **not a complete POSIX Commands & Utilities
implementation**. In the configured 116-name VSC-PCTS2016 POSIX08 scenario,
the canonical Go inventory supplies 90 names and is absent for 26. The shell
supplies 14 of those 26 and a **pinned POSIX external provider** supplies the
remaining 10, leaving no external-provider gaps in the assembled Bashy
Profiles C/D surface.
See the generated [required-command coverage
map](docs/posix-required-commands.md) and its separate, explicitly incomplete
[interface evidence ledger](docs/posix-required-command-interfaces.md); do not
infer the coreutils gap from the smaller assembled-Bashy number.

A provider is **not** a Go applet, and the matrix counts them separately so
they can never be read as Go coverage: the multicall owns the NAME
(`m4`, `man`, `ctags`, `ar`, `nm`, `strip`, `ex`, `vi`, `lp`, `localedef`)
and dispatches to a copy of the upstream program built locally
from a sha256-pinned source tarball. The pure-Go applets exclusively own `ed`,
`patch`, `mail`/`mailx`, `talk`, `make`, and `bc`; there are no external-provider definitions
or fallback for those names. Owning each active provider name is the point — before it,
those names fell through to the host's `$PATH`, so a "Bashy-only" arm silently
measured the distro's binaries. There is no fallback: an unprovisioned provider
exits 127 with the command that fixes it. See
[POSIX external providers](docs/posix-external-providers.md).

Profile B deliberately uses Bashy with pinned GNU/system utilities and excludes
these Go applets. Profiles C/D place the Go multicall provider first. Any
resulting evidence applies to the exact staged profile and provider manifest,
not to this repository alone. An applet appearing in the command list means its
documented supported subset is implemented and tested; it does not by itself
claim full POSIX or GNU option coverage. See
[the command behavior reference policy](docs/reference-policy.md),
[the portable-userland and provider policy](docs/commands.md#portable-userland-and-certification-provider-policy)
and the generated [applet matrix](docs/applet-matrix.md).

## The agent contract

Every tool in this repo follows the same rules:

- **Deterministic output.** `LC_ALL=C` semantics always; no locale,
  color, or terminal-width variance by default.
- **Reference-compatible where implemented.** POSIX certification behavior
  follows POSIX. Commands that belong to GNU Coreutils use the GNU Coreutils
  9.11 manual and runtime: full behavioral compatibility with 9.11 is the
  project target, while the current claim remains compatibility for the
  documented and tested subset. Other commands use their own documented
  upstream family. See the
  [reference policy](docs/reference-policy.md). Behavior is implemented from
  documentation (or adapted from
  permissively-licensed reimplementations, see
  `THIRD_PARTY_LICENSES.md`), never from GPL sources. A supported flag
  means exactly what the upstream documentation says — same spelling,
  same defaults, same output; meanings are never changed or
  approximated.
- **Clear errors for the rest.** Not every flag is supported. An
  unsupported flag or mode fails loudly, naming the flag, with exit
  code 2 — never silently ignored, never silently approximated.
- **Pure Go implementation.** An applet never delegates its own behavior to a
  similarly named system binary. Command wrappers such as `env`, `time`,
  `timeout`, `watch`, `xargs`, and `find -exec` execute their command operand
  because execution is their documented behavior. Separately managed external
  tools are external providers, not coreutils implementations.

## Packages

- `pkg/posixprovider` + `cmds/posixproviders` — the pinned POSIX external
  providers: the manifest (`pkg/posixprovider/manifest.tsv`, the one canonical
  copy, embedded), a cache-lookup resolver that verifies the cached binary
  against its recorded provenance and **never** downloads or compiles, the
  ten registered provider tools, and
  `posix-providers build|list|check|dispatch-plan` —
  `build` is the only path allowed to fetch and compile, and `dispatch-plan`
  is the introspection surface disclosing the exact verified binary each
  provider name would dispatch to. `BASHY_POSIX_PROVIDERS=off`
  unregisters the provider names. See
  [POSIX external providers](docs/posix-external-providers.md).
  `cmds/posixgate` ships `posix-gate`, the fail-closed effective-owner gate
  over the 116 POSIX-required names (availability 92/14/10, effective
  selection 84/22/10): it proves the assembled runtime selects each name's
  intended owner (Go applet, shell builtin/keyword/entry, or pinned provider)
  and rejects count drift on either axis, ambiguous ownership, missing
  provider pins/provenance, host PATH fallback, a provider cache the staged
  wrapper does not actually dispatch from, and any staged executable — tool
  or shell — whose digest is not the approved build recorded in the
  externally supplied build manifest (Profile C: stock GNU Bash 5.3;
  Profile D: Bashy 5.3). See
  [the POSIX owner gate](docs/posix-owner-gate.md).

- `cmds/` — the required set: 141 registered Go command packages (see the
  generated [applet matrix](docs/applet-matrix.md); ten names are external
  providers rather than Go implementations),
  covering file operations
  (cp, mv, rm, mkdir, ln, chmod, …), listing (ls, stat, du, df, …),
  text (cat, head, tail, wc, sort, uniq, cut, tr, grep, diff, …),
  system info (date, uname, id, …), checksums (md5/sha\*sum,
  base64/32), conditionals (`test` and its `[` spelling — the full
  POSIX primary set, `!`/`-a`/`-o`/parentheses, exit 0 true / 1 false /
  2 malformed), and the POSIX archive/patch/edit family (pax, patch, ed,
  mailx, make, bc). Each command is its own importable package registered
  into `tool/`'s registry; `cmds/all` pulls in the required set, and yoke's
  `cmds/all` adds the rest of the userland on top. Agent-traffic compatibility includes GNU grep
  `-A`/`-B`/`-C` context and ordered `--include` filtering, plus sed
  `addr,+N` ranges and `-i[SUFFIX]` in-place editing.
- `tool/` — the framework: registry + per-invocation RunContext
  (stdio, working directory, environment — tools never touch process
  globals) + strict GNU-style flags with automatic `--help`/`--version`.
- `cmd/coreutils` — busybox-style multicall binary (`coreutils ls …`,
  or symlink a tool name to the binary for argv[0] dispatch). It links
  this module only — it is the certification SUT.

Current priorities and withheld implementations are maintained in
[docs/commands.md](docs/commands.md).

## Consumers

- [yoke](https://github.com/qiangli/yoke) — the agentic userland; imports
  `tool/`, `multicall/`, `shell/` and the shared `pkg/` packages.
- [bashy](https://github.com/qiangli/bashy) — the shell; wires
  `shell.Handler()` so the applets run in-process.
- [outpost](https://github.com/qiangli/outpost),
  [ycode](https://github.com/qiangli/ycode) — through yoke.

## License

MIT
