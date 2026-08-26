# The POSIX owner gate (`posix-gate`)

The assembled Profile C/D runtime makes a precise claim: every one of the 116
POSIX-required utility names is supplied by exactly one **intended owner** —

- a registered **Bashy Go applet** (88 names),
- the **shell** (14 names: the `sh` entry point plus builtins), or
- one of the fourteen **active pinned POSIX external providers** the multicall
  registers and dispatches (see
  [posix-external-providers.md](posix-external-providers.md)).

That is the **availability** split, 88/14/14. What a POSIX-mode shell
actually **selects** differs for exactly eight names (the seven builtin
overlaps plus the `time` keyword), giving the **effective selection** split
80/22/14. The gate pins both, explicitly, and verifies the effective split
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
| count drift | availability no longer splits 116 = 88/14/14, **or** effective selection no longer splits 80/22/14; both axes are hard pins that must be changed deliberately, in one reviewed place |
| duplicate / ambiguous ownership | a name claimed twice: a shell-owned name that also has a registered tool, an applet that is also pinned as a provider, inventory/manifest provider sets that disagree |
| missing provider pin | a provider row without a full sha256 source pin, version, platform list, or upstream URL |
| missing / broken provenance | a provider whose cached binary is absent, or does not hash to what its `provenance.tsv` records — an unattributable binary is worse than a missing one, because it still produces numbers |
| broken build manifest | the externally supplied build/run manifest (`--manifest`) unreadable, missing a required pin (`profile`, `shell_sha256`, `multicall_sha256`), carrying a malformed digest (a digest is exactly 64 hexadecimal characters), a duplicate pin, or an unknown profile — with no root of trust, nothing else is verified |
| profile mismatch | the gate invoked for one profile with a manifest approved for the other: approved builds do not transfer between profiles |
| unbound provider cache | the staged environment does not name `BASHY_BIN_CACHE`, so provenance cannot be bound to the cache the staged wrapper will actually dispatch from |
| unbound provider dispatch | the approved multicall's own dispatch plan (`posix-providers dispatch-plan`, run with the staged environment) failing, not accounting for exactly the fourteen active providers, or disclosing a resolved executable/version/built digest that differs from the gate's independently verified cache identity — a valid cache the wrapper does not actually dispatch from fails here |
| host PATH fallback | a staged runtime in which a multicall-owned name resolves outside the staged tool directory (or not at all) |
| unapproved executable identity | a staged entry — even one inside the tool directory — whose resolved target does not hash to the manifest's approved multicall digest: a staged symlink to an arbitrary host `/bin` tool fails here |
| host PATH shell / unapproved shell build | the interrogated shell resolving outside the staged directory, or its bytes not hashing to the manifest's approved `shell_sha256` — a forgeable `--version` line or target triplet is never accepted as a build identity |
| wrong shell identity for the profile | a digest-proven shell whose reported identity still disagrees with the profile: both profiles require GNU bash exactly 5.3, and the release flavor decides between them — Profile C requires the stock `-release` flavor and rejects any `-bashy-` build, Profile D requires the Bashy-specific `-bashy-<revision>` marker (e.g. `5.3.0(1)-bashy-dev`) and rejects stock flavors; 5.2, 5.4, a non-shell banner (`bashy, GNU Bash … compatible` is the front-door command, not the shell), or a missing build identifier all reject |
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
posix-gate runtime --profile C|D --manifest FILE --bindir DIR --multicall PATH [--shell NAME]
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
staged certification runtime that cannot supply all fourteen active names is not the
runtime it claims to be; `posix-providers check` keeps its softer per-host
semantics for ordinary provisioning work.

## The runtime gate

Run it from **inside** the staged environment, so the PATH, provider cache,
and POSIXLY_CORRECT it validates are the ones the runtime actually has:

```sh
POSIXLY_CORRECT=1 PATH=/staged/bin BASHY_BIN_CACHE=/staged/cache \
  coreutils posix-gate runtime --profile C \
    --manifest /release/approved-builds.tsv \
    --bindir /staged/bin \
    --multicall /staged/bin/coreutils
```

`--manifest` names the **approved build/run manifest** — `key<TAB>value`
rows, `#` comments allowed, written by whoever produced the approved builds
and supplied to the gate externally:

```
profile	C
shell_sha256	<sha256 of the approved shell build>
multicall_sha256	<sha256 of the approved multicall build>
```

The manifest is the runtime gate's **root of trust**: every digest must be
exactly 64 hexadecimal characters, both digests and the profile are
mandatory, extra rows (builder, date, revisions) are tolerated, and a digest
is never derived from a staged binary — a binary hashing to itself proves
only that it equals itself. A defective manifest, or a manifest approved for
the other profile, rejects before anything else is verified.

It verifies, in order:

1. **Its own environment** — `POSIXLY_CORRECT` must already be set (the
   harness stages `POSIXLY_CORRECT=1`; the gate checks presence, see above).
2. **Approved executable identity** — `--multicall` names the staged
   multicall executable, which must hash to exactly the manifest's
   `multicall_sha256`. Identity is **mandatory**: there is no
   membership-in-a-directory shortcut and no self-derived digest.
3. **PATH ownership + identity of every multicall-owned name** — every
   multicall-owned name (88 applets + 14 providers) and `sh` itself must
   resolve, through the environment's own PATH, to an entry inside
   `--bindir`, **and** each multicall-owned entry's resolved target must hash
   to the approved multicall's digest. Staged entries are routinely symlinks
   to the multicall elsewhere — that layout passes precisely because the
   digest matches; a staged symlink (or copy) pointing at an arbitrary host
   tool fails, no matter where it sits.
4. **Provider dispatch binding** — see below: the verified provider cache,
   plus the approved multicall's own disclosed dispatch plan, row for row.
5. **Shell resolution + build identity** — the interrogated shell is a
   command NAME (`--shell`, default `sh`), resolved through the RunContext's
   staged PATH; a host path operand is a usage error and a resolution outside
   `--bindir` is a rejection. The resolved shell's bytes must hash to the
   manifest's `shell_sha256` — the build identity is the digest, because a
   `--version` line or target triplet is forgeable. Only a digest-proven
   shell is then cross-checked against what it reports. Both approved shells
   report the stock `GNU bash, version 5.3…` line — Bashy is a bash-5.3
   drop-in, so its staged shell prints e.g.
   `GNU bash, version 5.3.0(1)-bashy-dev (a0a0315)`, and a
   `bashy, GNU Bash … compatible` banner (the bashy front-door command's) is
   never a shell version line. The cross-check requires GNU bash
   **exactly** version 5.3 (5.2 and 5.4 both reject), the profile's approved
   release flavor — stock `-release` for Profile C (any `-bashy-` build
   rejects, even one whose digest was accidentally pinned), the
   Bashy-specific `-bashy-<revision>` marker for Profile D (stock flavors
   reject) — and a non-empty build identifier. No probe runs against an
   unvalidated shell.
6. **Shell-effective ownership** — one spawn of the staged shell classifies
   all 116 names with `type -t`. The transcript is parsed strictly: exactly
   one well-formed row per expected name (116 unique names); duplicate,
   extra, missing and malformed rows are each rejections. The expected class
   comes from the effective selector, including the deliberate overlaps
   below.
7. **POSIX mode** — the shell's `set -o` must report `posix on`. Being a
   POSIX-capable shell is not the same as being in POSIX mode.
8. **POSIXLY_CORRECT propagation** — the variable must survive, non-empty,
   into a shell child (`"$POSIXLY_CORRECT"` expansion) and across a real
   process boundary (`exec env` — where `env` is whatever the staged PATH
   supplies, already verified to be the approved multicall — must show the
   variable set).

The runtime verdict also re-runs the **registry** gate and the **provider**
gate. For providers (step 4) it binds provenance to the staged wrapper's
ACTUAL dispatch, in two mutually checking halves. First the cache: the
wrapper resolves its cache from the environment the shell hands it, so the
staged environment must name `BASHY_BIN_CACHE` explicitly — an absent cache
root is a rejection, never a fall-back to the gate process's own default
cache, which the wrapper might never consult — and every active provider must
resolve from it with provenance intact. Second the dispatch: a verified cache
sitting there proves nothing about what the wrapper runs, so the gate makes
the approved multicall (digest-proven in step 2 — an unproven binary's
answers about itself are worthless) disclose its own plan with
`posix-providers dispatch-plan`, run with the staged environment. The
observed resolved executable, version, and built digest for every provider
must equal the gate's independently verified identity
(`posixprovider.Resolver.VerifiedIdentity`) for the same name; the plan is
parsed as strictly as the classification transcript (exactly fourteen rows,
no duplicates, extras, or malformed rows). A valid-but-unused cache
alongside a wrapper that would dispatch an arbitrary staged executable fails
here.

### Availability vs effective selection: the overlaps

Seven applet-owned names are also regular builtins in a POSIX-mode
bash-family shell — `echo`, `false`, `kill`, `printf`, `pwd`, `test`,
`true` — and `time` is a reserved word. For those names the shell-level
effective owner is the builtin/keyword and the Go applet backs the same name
on PATH for exec-style callers (`env`, `xargs`, `find -exec`). Both facts are
intended, and the gate verifies **both**: the classification probe requires
`builtin`/`keyword` for exactly these names, and the PATH probe still
requires the staged file with the approved digest. These eight names are the
entire 88/14/14 → 80/22/14 difference; the effective pins mean an eighth
builtin overlap (or a lost one) is count drift, not a curiosity.

The 14 shell-owned names split the same way: `sh` must classify as `file`
(and resolve inside `--bindir`); the other 13 (`alias`, `bg`, `cd`,
`command`, `fc`, `fg`, `getopts`, `hash`, `jobs`, `read`, `umask`,
`unalias`, `wait`) must classify as `builtin` and carry no PATH obligation.

### On spawning

The runtime probes spawn the staged shell resolved from `--shell` and the
approved multicall named by `--multicall` (for the dispatch-plan
disclosure). That is not a breach of the repo's no-shell-out rule: like
`env`/`xargs`/`watch`, this tool's documented purpose *is* running the
operands it is given — the gate interrogates the staged runtime, and there
is nothing else it could interrogate. Everything else (`spec`, `registry`,
`providers`) spawns nothing.

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
| `cmds/posixgate/manifest.go` | the approved build/run manifest: format, validation, profile pins |
| `cmds/posixgate/posixgate.go` | the `posix-gate` applet |
| `cmds/all/posixgate_test.go` | the registry gate run against the real assembled registry in CI (fails, never skips, under the provider opt-out) |
