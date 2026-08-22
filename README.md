# coreutils

A pure-Go agent userland: one canonical set of Unix-style Go applets with
identical behavior on every major platform (Linux, macOS, Windows).

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
the canonical Go inventory supplies 76 names and is absent for 40. The shell
supplies 14 of those 40, leaving 26 external-provider gaps in the assembled
Bashy Profiles C/D surface. See the generated
[complete required-command matrix](docs/posix-required-commands.md); do not
infer the coreutils gap from the smaller assembled-Bashy number.

Profile B deliberately uses Bashy with pinned GNU/system utilities and excludes
these Go applets. Profiles C/D place the Go multicall provider first. Any
resulting evidence applies to the exact staged profile and provider manifest,
not to this repository alone. An applet appearing in the command list means its
documented supported subset is implemented and tested; it does not by itself
claim full POSIX or GNU option coverage. See
[the portable-userland and provider policy](docs/commands.md#portable-userland-and-certification-provider-policy)
and the generated [applet matrix](docs/applet-matrix.md).

## The agent contract

Every tool in this repo follows the same rules:

- **Deterministic output.** `LC_ALL=C` semantics always; no locale,
  color, or terminal-width variance by default.
- **GNU-compatible where implemented.** Behavior follows the
  [GNU coreutils manual](https://www.gnu.org/software/coreutils/manual/)
  and POSIX — implemented from the documentation (or adapted from
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

- `git/` — self-contained git client built on go-git/v5: the typed API
  (`Clone`, `Pull`, `Merge`, …) for CLIs that own their flag parsing,
  and `Exec(ctx, dir, args)` for argv-style callers, with
  `ErrUnsupported` as the fall-back-or-fail signal. Pull and merge
  integrate fast-forwards only (preserving non-conflicting local
  changes, like real git); conflict resolution is out of scope by
  design. Even local-path remotes use go-git's in-process server
  transport — `git-upload-pack` is never spawned.

- `cmds/` — the userland: 142 shipped Go command packages advertising 147
  applet names (see the generated [applet matrix](docs/applet-matrix.md)),
  covering file operations
  (cp, mv, rm, mkdir, ln, chmod, …), listing (ls, stat, du, df, …),
  text (cat, head, tail, wc, sort, uniq, cut, tr, grep, diff, …),
  system info (date, uname, id, …), checksums (md5/sha\*sum,
  base64/32), conditionals (`test` and its `[` spelling — the full
  POSIX primary set, `!`/`-a`/`-o`/parentheses, exit 0 true / 1 false /
  2 malformed), and archives (tar, gzip). Each command is its own
  importable package registered into `tool/`'s registry; `cmds/all`
  pulls in everything. Agent-traffic compatibility includes GNU grep
  `-A`/`-B`/`-C` context and ordered `--include` filtering, plus sed
  `addr,+N` ranges and `-i[SUFFIX]` in-place editing.
- `tool/` — the framework: registry + per-invocation RunContext
  (stdio, working directory, environment — tools never touch process
  globals) + strict GNU-style flags with automatic `--help`/`--version`.
- `cmd/coreutils` — busybox-style multicall binary (`coreutils ls …`,
  or symlink a tool name to the binary for argv[0] dispatch).

Current priorities and withheld implementations are maintained in
[docs/commands.md](docs/commands.md).

## Consumers

- [outpost](https://github.com/qiangli/outpost) — `outpost git …` is a
  cobra CLI over `git/`'s typed API.
- [ycode](https://github.com/qiangli/ycode) — `yc git …` dispatches
  through `git.Exec` natively, falling back to a host git binary (and
  then a container) for anything `ErrUnsupported`.

## License

MIT
