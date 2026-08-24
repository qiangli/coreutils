# The POSIX owner gate (`posix-gate`)

The assembled Profile C/D runtime makes a precise claim: every one of the 116
POSIX-required utility names is supplied by exactly one **intended owner** —

- a registered **Bashy Go applet** (86 names),
- the **shell** (14 names: the `sh` entry point plus builtins), or
- one of the sixteen **pinned POSIX external providers** the multicall
  registers and dispatches (see
  [posix-external-providers.md](posix-external-providers.md)).

That is the **availability** split, 86/14/16. What a POSIX-mode shell
actually **selects** differs for exactly eight names (the seven builtin
overlaps plus the `time` keyword), giving the **effective selection** split
78/22/16. The gate pins both, explicitly, and verifies the effective split
against the staged shell's own answers.

`posix-gate` turns the claim into a checkable verdict. Every check is
**fail-closed**: the gate produces positive evidence that the intended owner is
selected, or it rejects, naming each name and cause. There is no "probably
fine" state — an owner the gate cannot verify is a rejection, exactly like an
owner that is wrong.

The gate maintains **no copy of the inventory**. It consumes a generated
projection (`cmds/posixgate/spec_gen.go`) of the canonical expanded POSIX
manifest ([posix-required-commands.tsv](posix-required-commands.tsv)), written
by `scripts/applet-matrix.py`: one row per required name, carrying the
availability owner from the manifest and the effective selector the staged
shell must exhibit. `--check` mode (run by `scripts/crossvet.sh` and the
pre-push hook) fails when the projection is stale, and a test in
`cmds/posixgate` re-derives the projection from the canonical manifest and
compares row by row.

## What it rejects

| Rejection | Meaning |
| --- | --- |
| count drift | availability no longer splits 116 = 86/14/16, **or** effective selection no longer splits 78/22/16; both axes are hard pins that must be changed deliberately, in one reviewed place |
| duplicate / ambiguous ownership | a name claimed twice: a shell-owned name that also has a registered tool, an applet that is also pinned as a provider, inventory/manifest provider sets that disagree |
| missing provider pin | a provider row without a full sha256 source pin, version, platform list, or upstream URL |
| missing / broken provenance | a provider whose cached binary is absent, or does not hash to what its `provenance.tsv` records — an unattributable binary is worse than a missing one, because it still produces numbers |
| unbound provider cache | the staged environment does not name `BASHY_BIN_CACHE`, so provenance cannot be bound to the cache the staged wrapper will actually dispatch from |
| host PATH fallback | a staged runtime in which a multicall-owned name resolves outside the staged tool directory (or not at all) |
| unapproved executable identity | a staged entry — even one inside the tool directory — whose resolved target does not hash to the approved multicall: a staged symlink to an arbitrary host `/bin` tool fails here |
| host PATH shell / unapproved shell | the interrogated shell resolving outside the staged directory, or not identifying as an approved Profile C/D shell (GNU bash or bashy, bash-5 family, with a build identifier) |
| broken classification transcript | the shell's classification transcript not accounting for exactly the 116 expected names: duplicates, extras, missing and malformed rows each reject |
| wrong effective owner in the shell | `type -t` classifying a name differently from what its effective selector requires (see overlaps below) |
| shell not in POSIX mode | `set -o` does not report `posix on` |
| POSIXLY_CORRECT not propagated | `POSIXLY_CORRECT` unset or empty in the gate's own environment, in a shell child, or in a process the shell `exec`s. Presence is the contract — POSIX utilities test whether the variable is set, and bash rewrites the exported `1` to `y` on entering posix mode, so demanding the literal value would reject a correctly staged runtime |
| provider opt-out | `BASHY_POSIX_PROVIDERS=off` in the observed environment: the provider names are unregistered, so the runtime cannot own them — a hard failure for a certification runtime, and the assembled-registry test in `cmds/all` fails (never skips) under it |

## Subcommands

```sh
posix-gate spec        # print the projection + both pinned splits
posix-gate registry    # hermetic: projection vs live tool registry vs provider manifest
posix-gate providers   # every pinned provider resolves from cache, provenance intact
posix-gate runtime --bindir DIR --multicall PATH [--shell NAME] [--multicall-sha256 HEX]
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

Run it from **inside** the staged environment, so the PATH, provider cache,
and POSIXLY_CORRECT it validates are the ones the runtime actually has:

```sh
POSIXLY_CORRECT=1 PATH=/staged/bin BASHY_BIN_CACHE=/staged/cache \
  coreutils posix-gate runtime --bindir /staged/bin \
    --multicall /staged/bin/coreutils \
    --multicall-sha256 <release digest>   # optional external pin
```

It verifies, in order:

1. **Its own environment** — `POSIXLY_CORRECT` must already be set (the
   harness stages `POSIXLY_CORRECT=1`; the gate checks presence, see above).
2. **Approved executable identity** — `--multicall` names the approved
   multicall executable; the gate digests it (and, when `--multicall-sha256`
   pins a digest externally, requires the two to agree). Identity is
   **mandatory**: the flag is required, and there is no
   membership-in-a-directory shortcut.
3. **PATH ownership + identity of every multicall-owned name** — every
   multicall-owned name (86 applets + 16 providers) and `sh` itself must
   resolve, through the environment's own PATH, to an entry inside
   `--bindir`, **and** each multicall-owned entry's resolved target must hash
   to the approved multicall's digest. Staged entries are routinely symlinks
   to the multicall elsewhere — that layout passes precisely because the
   digest matches; a staged symlink (or copy) pointing at an arbitrary host
   tool fails, no matter where it sits.
4. **Shell resolution + identity** — the interrogated shell is a command
   NAME (`--shell`, default `sh`), resolved through the RunContext's staged
   PATH; a host path operand is a usage error and a resolution outside
   `--bindir` is a rejection. The resolved shell's `--version` must identify
   an approved Profile C/D shell — `GNU bash` (Profile C) or `bashy`
   (Profile D) — at a bash-5-family version with a non-empty build
   identifier. No probe runs against an unvalidated shell.
5. **Shell-effective ownership** — one spawn of the staged shell classifies
   all 116 names with `type -t`. The transcript is parsed strictly: exactly
   one well-formed row per expected name (116 unique names); duplicate,
   extra, missing and malformed rows are each rejections. The expected class
   comes from the effective selector, including the deliberate overlaps
   below.
6. **POSIX mode** — the shell's `set -o` must report `posix on`. Being a
   POSIX-capable shell is not the same as being in POSIX mode.
7. **POSIXLY_CORRECT propagation** — the variable must survive, non-empty,
   into a shell child (`"$POSIXLY_CORRECT"` expansion) and across a real
   process boundary (`exec env` — where `env` is whatever the staged PATH
   supplies, already verified to be the approved multicall — must show the
   variable set).

The runtime verdict also re-runs the **registry** gate and the **provider**
gate. For providers it binds provenance to the staged wrapper's actual
dispatch target: the wrapper (the approved multicall, proven in step 3)
resolves its cache from the environment the shell hands it, so the staged
environment must name `BASHY_BIN_CACHE` explicitly — an absent cache root is
a rejection, never a fall-back to the gate process's own default cache, which
the wrapper might never consult.

### Availability vs effective selection: the overlaps

Seven applet-owned names are also regular builtins in a POSIX-mode
bash-family shell — `echo`, `false`, `kill`, `printf`, `pwd`, `test`,
`true` — and `time` is a reserved word. For those names the shell-level
effective owner is the builtin/keyword and the Go applet backs the same name
on PATH for exec-style callers (`env`, `xargs`, `find -exec`). Both facts are
intended, and the gate verifies **both**: the classification probe requires
`builtin`/`keyword` for exactly these names, and the PATH probe still
requires the staged file with the approved digest. These eight names are the
entire 86/14/16 → 78/22/16 difference; the effective pins mean an eighth
builtin overlap (or a lost one) is count drift, not a curiosity.

The 14 shell-owned names split the same way: `sh` must classify as `file`
(and resolve inside `--bindir`); the other 13 (`alias`, `bg`, `cd`,
`command`, `fc`, `fg`, `getopts`, `hash`, `jobs`, `read`, `umask`,
`unalias`, `wait`) must classify as `builtin` and carry no PATH obligation.

### On spawning

The runtime probes spawn the staged shell resolved from `--shell`. That is
not a breach of the repo's no-shell-out rule: like `env`/`xargs`/`watch`,
this tool's documented purpose *is* running the operand it is given — the
gate interrogates the staged shell, and there is nothing else it could
interrogate. Everything else (`spec`, `registry`, `providers`) spawns
nothing.

## Relation to the certification harness

The harness wires its measured PATH from the multicall's own inventory, which
is exactly why registry ownership is load-bearing: an unregistered name is a
name the host silently supplies. This gate is the OSS, harness-independent
statement of that invariant — it knows nothing about any particular harness's
layout and takes the staged tool directory, approved multicall, and shell
name as plain operands. Harness-side wiring details stay out of this
repository by design.

## Files

| Path | Role |
| --- | --- |
| `cmds/posixgate/spec_gen.go` | generated projection of the canonical manifest (availability owner + effective selector per name; `scripts/applet-matrix.py` owns it) |
| `cmds/posixgate/spec.go` | projection vocabulary, validation, pinned counts (both axes), expected classifications |
| `cmds/posixgate/verify.go` | the registry / provider / runtime gates |
| `cmds/posixgate/posixgate.go` | the `posix-gate` applet |
| `cmds/all/posixgate_test.go` | the registry gate run against the real assembled registry in CI (fails, never skips, under the provider opt-out) |
