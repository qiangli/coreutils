# coreutils

A pure-Go agent userland: one set of Unix-style tools with identical
behavior on every major platform (Linux, macOS, Windows), no system
binaries required.

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
interpreter's `ExecHandler`, an agent gets a complete Unix-like
environment in a single Go process.

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
- **Pure Go.** No tool ever shells out to a system binary. If the
  pure-Go implementation can't do something, you get an error that
  says so.

## Packages

- `git/` — self-contained git client built on go-git/v5: the typed API
  (`Clone`, `Pull`, `Merge`, …) for CLIs that own their flag parsing,
  and `Exec(ctx, dir, args)` for argv-style callers, with
  `ErrUnsupported` as the fall-back-or-fail signal. Pull and merge
  integrate fast-forwards only (preserving non-conflicting local
  changes, like real git); conflict resolution is out of scope by
  design. Even local-path remotes use go-git's in-process server
  transport — `git-upload-pack` is never spawned.

- `cmds/` — the userland: 74 commands (Phase A of
  [docs/commands.md](docs/commands.md)) covering file operations
  (cp, mv, rm, mkdir, ln, chmod, …), listing (ls, stat, du, df, …),
  text (cat, head, tail, wc, sort, uniq, cut, tr, grep, diff, …),
  system info (date, uname, id, …), checksums (md5/sha\*sum,
  base64/32), and archives (tar, gzip). Each command is its own
  importable package registered into `tool/`'s registry; `cmds/all`
  pulls in everything.
- `tool/` — the framework: registry + per-invocation RunContext
  (stdio, working directory, environment — tools never touch process
  globals) + strict GNU-style flags with automatic `--help`/`--version`.
- `cmd/coreutils` — busybox-style multicall binary (`coreutils ls …`,
  or symlink a tool name to the binary for argv[0] dispatch).

Planned next (see docs/commands.md): Phase B — the rest of the GNU
manual (printf, test, expr, od, dd, …) — then sed/xargs/ps and the
`mvdan.cc/sh/v3` `ExecHandler` adapter.

## Consumers

- [outpost](https://github.com/qiangli/outpost) — `outpost git …` is a
  cobra CLI over `git/`'s typed API.
- [ycode](https://github.com/qiangli/ycode) — `yc git …` dispatches
  through `git.Exec` natively, falling back to a host git binary (and
  then a container) for anything `ErrUnsupported`.

## License

MIT
