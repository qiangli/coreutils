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

Planned (see the roadmap in CLAUDE.md): the file/text tier of GNU
coreutils (`ls`, `cat`, `cp`, `mv`, `rm`, `sort`, `head`, `tail`, …),
the agent-critical siblings that live outside coreutils proper
(`grep`, `find`, `sed`, `xargs`, `diff`, `tar`), a busybox-style
multicall binary, and the `mvdan.cc/sh/v3` `ExecHandler` adapter.

## Consumers

- [outpost](https://github.com/qiangli/outpost) — `outpost git …` is a
  cobra CLI over `git/`'s typed API.
- [ycode](https://github.com/qiangli/ycode) — `yc git …` dispatches
  through `git.Exec` natively, falling back to a host git binary (and
  then a container) for anything `ErrUnsupported`.

## License

MIT
