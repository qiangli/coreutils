# The POSIX owner gate (`posix-gate`)

The assembled Profile C/D runtime makes a precise claim: every one of the 116
POSIX-required utility names is supplied by exactly one **intended owner** —

- a registered **Bashy Go applet** (86 names),
- the **shell** (14 names: the `sh` entry point plus builtins), or
- one of the sixteen **pinned POSIX external providers** the multicall
  registers and dispatches (see
  [posix-external-providers.md](posix-external-providers.md)).

`posix-gate` turns that claim into a checkable verdict. Every check is
**fail-closed**: the gate produces positive evidence that the intended owner is
selected, or it rejects, naming each name and cause. There is no "probably
fine" state — an owner the gate cannot verify is a rejection, exactly like an
owner that is wrong.

The intended-owner inventory itself is the generated
[posix-required-commands.tsv](posix-required-commands.tsv); the gate embeds a
copy that `scripts/applet-matrix.py` writes from the same rows, so the gate,
the docs, and the registry are pinned to one another (`--check` mode, run by
`scripts/crossvet.sh`, plus a byte-equality test in `cmds/posixgate`).

## What it rejects

| Rejection | Meaning |
| --- | --- |
| count drift | the inventory no longer holds 116 names split 86/14/16; the pins are hard constants that must be changed deliberately, in one reviewed place |
| duplicate / ambiguous ownership | a name claimed twice: a shell-owned name that also has a registered tool, an applet that is also pinned as a provider, inventory/manifest provider sets that disagree |
| missing provider pin | a provider row without a full sha256 source pin, version, platform list, or upstream URL |
| missing / broken provenance | a provider whose cached binary is absent, or does not hash to what its `provenance.tsv` records — an unattributable binary is worse than a missing one, because it still produces numbers |
| host PATH fallback | a staged runtime in which a multicall-owned name resolves outside the staged tool directory (or not at all) |
| wrong effective owner in the shell | `type -t` classifying a name differently from what its owner requires (see overlaps below) |
| shell not in POSIX mode | `set -o` does not report `posix on` |
| POSIXLY_CORRECT not propagated | `POSIXLY_CORRECT` unset or empty in the gate's own environment, in a shell child, or in a process the shell `exec`s. Presence is the contract — POSIX utilities test whether the variable is set, and bash rewrites the exported `1` to `y` on entering posix mode, so demanding the literal value would reject a correctly staged runtime |

## Subcommands

```sh
posix-gate spec        # print the canonical owner inventory + pinned counts
posix-gate registry    # hermetic: inventory vs live tool registry vs provider manifest
posix-gate providers   # every pinned provider resolves from cache, provenance intact
posix-gate runtime --shell PATH --bindir DIR [--same-target]
                       # the full staged verdict (includes registry + providers)
```

Exit status: 0 every gate passed, 1 any rejection, 2 usage. Rejections go to
stderr one per line (`FAIL [check] name: detail`) followed by a one-line
verdict, so a harness log carries both the verdict and every cause.

`registry` is hermetic — no cache, no network, nothing spawned — and runs in
CI: `cmds/all/posixgate_test.go` executes the same gate against the real
assembled registry on every `go test`, so ownership drift fails before any
certification arm measures it.

`providers` deliberately differs from `posix-providers check` in one respect:
a platform the manifest does not declare is a **failure** here, not a skip. A
staged certification runtime that cannot supply all sixteen names is not the
runtime it claims to be; `posix-providers check` keeps its softer per-host
semantics for ordinary provisioning work.

## The runtime gate

Run it from **inside** the staged environment, so the PATH and
POSIXLY_CORRECT it validates are the ones the runtime actually has:

```sh
POSIXLY_CORRECT=1 PATH=/staged/bin \
  coreutils posix-gate runtime --shell /staged/bin/sh --bindir /staged/bin
```

It verifies, in order:

1. **Its own environment** — `POSIXLY_CORRECT` must already be set (the
   harness stages `POSIXLY_CORRECT=1`; the gate checks presence, see above).
2. **PATH ownership** — every multicall-owned name (86 applets + 16
   providers) and `sh` itself must resolve, through the environment's own
   PATH, to an entry inside `--bindir`. Resolving anywhere else is the
   host-fallback bug this gate exists to reject; resolving nowhere is a name
   the runtime cannot supply. Staged entries are routinely symlinks to the
   multicall elsewhere — only the *directory* is identity-checked, never the
   link target, except under `--same-target`, which additionally requires all
   multicall-owned names to resolve (through symlinks) to one identical
   executable.
3. **Shell-effective ownership** — one spawn of the staged shell classifies
   all 116 names with `type -t`. The expected class comes from the intended
   owner, including the deliberate overlaps below.
4. **POSIX mode** — the shell's `set -o` must report `posix on`. Being a
   POSIX-capable shell is not the same as being in POSIX mode.
5. **POSIXLY_CORRECT propagation** — the variable must survive, non-empty,
   into a shell child (`"$POSIXLY_CORRECT"` expansion) and across a real
   process boundary (`exec env` — where `env` is whatever the staged PATH
   supplies, already verified in step 2 to be the staged applet — must show
   the variable set).

### Builtin overlaps and `time`

Seven applet-owned names are also regular builtins in a POSIX-mode
bash-family shell — `echo`, `false`, `kill`, `printf`, `pwd`, `test`,
`true` — and `time` is a reserved word. For those names the shell-level
effective owner is the builtin/keyword and the Go applet backs the same name
on PATH for exec-style callers (`env`, `xargs`, `find -exec`). Both facts are
intended, and the gate verifies **both**: the classification probe requires
`builtin`/`keyword` for exactly these names, and the PATH probe still
requires the staged file. The overlap set is pinned — an eighth name coming
back as a builtin is an ownership violation, not a curiosity.

The 14 shell-owned names split the same way: `sh` must classify as `file`
(and resolve inside `--bindir`); the other 13 (`alias`, `bg`, `cd`,
`command`, `fc`, `fg`, `getopts`, `hash`, `jobs`, `read`, `umask`,
`unalias`, `wait`) must classify as `builtin` and carry no PATH obligation.

### On spawning

The runtime probes spawn the shell named by `--shell`. That is not a breach
of the repo's no-shell-out rule: like `env`/`xargs`/`watch`, this tool's
documented purpose *is* running the operand it is given — the gate
interrogates the staged shell, and there is nothing else it could
interrogate. Everything else (`spec`, `registry`, `providers`) spawns
nothing.

## Relation to the certification harness

The harness wires its measured PATH from the multicall's own inventory, which
is exactly why registry ownership is load-bearing: an unregistered name is a
name the host silently supplies. This gate is the OSS, harness-independent
statement of that invariant — it knows nothing about any particular harness's
layout and takes the staged shell and tool directory as plain operands.
Harness-side wiring details stay out of this repository by design.

## Files

| Path | Role |
| --- | --- |
| `cmds/posixgate/posix-required-commands.tsv` | embedded owner inventory (generator-owned copy of `docs/posix-required-commands.tsv`) |
| `cmds/posixgate/spec.go` | inventory parsing, pinned counts, overlap sets, expected classifications |
| `cmds/posixgate/verify.go` | the registry / provider / runtime gates |
| `cmds/posixgate/posixgate.go` | the `posix-gate` applet |
| `cmds/all/posixgate_test.go` | the registry gate run against the real assembled registry in CI |
