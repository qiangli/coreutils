# POSIX external providers

Twelve POSIX-required commands are deliberately not implemented in Go:

```
make  bc  patch  m4  ed  man  ctags  ar  nm  strip  ex  vi
```

They are **external providers**: the multicall owns the name and dispatches to
a copy of the upstream program, built locally from a sha256-pinned source
tarball and checked against its recorded provenance before it runs.

## Why the multicall has to own the name

Profile C of the POSIX certification campaign is "GNU Bash + the Bashy Go
coreutils". Until this mechanism existed those sixteen names were not in
`tool.Names()`, so the shell adapter fell through to `$PATH` and the arm
measured **Ubuntu's** binaries while reporting itself as bashy-only.

The certification harness's `sut-wire.sh` rebuilds `/vsc/cushim` from the
multicall's own inventory, so a registered name is automatically wired into the
measured PATH — and an unregistered one is silently taken from the host. That
makes the registry the measured denominator, which is why registration is the
load-bearing half of this change and why there is **no fallback of any kind**:
an unavailable provider prints why and exits 127.

## Resolving never builds

`posixprovider.Resolve` is a cache lookup. It does not download, does not
compile, does not touch the network, and has no code path that could.

Provisioning is a **prepare-time** activity; running is a **test-time** one.
Fusing them would let a resolve inside a six-hour certification arm decide to
fetch and compile GNU make — injecting network and toolchain variance into
measured evidence, and risking a hang that costs the whole arm.

## Provisioning

```sh
bashy posix-providers list          # what is pinned, and what is provisioned
bashy posix-providers check         # verify provisioning + provenance (non-zero if any is unusable)
bashy posix-providers build make    # fetch pinned SOURCE, verify sha256, build locally
bashy posix-providers build all
```

`build` is the only path that downloads or compiles. It drives
`tools/posix-providers/build.sh`, which verifies the pinned digest **before**
extraction, builds with `$POSIX_PROVIDER_CC` → the pinned Zig toolchain → the
host `cc`, installs into the binmgr cache
(`$BASHY_BIN_CACHE`, else `<UserCacheDir>/bashy/bin`) at
`<cache>/<command>/<version>/<command>`, and writes a `provenance.tsv`
sidecar recording what was built, from which pinned bytes, by which compiler.

The applet is a thin driver over that recipe. Shelling out here is a **build
step**, not a utility implementation — the repo's "never shell out" rule governs
the coreutils themselves (`cat` never execs `/bin/cat`), and compiling GNU make
from source is categorically not that.

`build` needs the recipe on disk. It looks at `$POSIX_PROVIDER_BUILD`, then
walks up from the working directory and from the executable's directory for
`tools/posix-providers/build.sh`. A binary installed away from its checkout must
be told where the recipe is; `list`, `check`, and the providers themselves need
no checkout.

## Provenance is enforced, not advisory

Before returning a path, the resolver checks the cached binary against its
`provenance.tsv`: the command, the version, the pinned source digest, and the
sha256 of the binary itself. A mismatch is an **error**, never a warning. An
unattributable binary in a certification arm is worse than a missing one,
because it still produces numbers.

The cost is one sha256 of the binary per invocation (sub-millisecond for `make`,
a few milliseconds for a binutils tool). That is the price of being able to say
which bytes produced a result.

## Platform gating

The manifest declares platforms per provider; `ed` and `man` are
`linux,darwin` only. The gate is enforced at **run** time, not at registration
time — the name is registered on every platform so the multicall owns it
everywhere, and an unsupported host gets

```
ed: ed 1.20.1 is not supported on windows (manifest declares: linux,darwin)
```

instead of a silent fall-through to whatever `$PATH` holds. It also keeps the
registry — and therefore the measured inventory and the `pkg/atlas` coverage
ratchet — the same shape on every platform.

## Opting out

```sh
BASHY_POSIX_PROVIDERS=off
```

unregisters all sixteen provider names, so plain bashy stays standalone-graceful
on a machine with no provider cache and normal `$PATH` resolution applies again.
Only the exact word `off` (case-insensitive) opts out; the default is to own the
names and fail loudly. The `posix-providers` applet itself is always registered
— it is how you get back out of the un-provisioned state.

## Licence posture

Every provider is copyleft (GPL-2.0, GPL-3.0, or the Vim licence), so download
and build are deliberately separated:

- **download** — upstream SOURCE only, pinned by sha256. Upstream is the
  distributor; its obligations are already discharged.
- **build** — local, from the recipe. A binary you build for yourself is not
  distribution, so no obligation is triggered.
- **we ship** — the manifest and the recipe. Our own work, our own licence.

Never mirror or republish the built binaries. The full posture is in the header
of `pkg/posixprovider/manifest.tsv` and in the umbrella's
`docs/posix-provider-distribution-policy.md`.

## Files

| Path | Role |
| --- | --- |
| `pkg/posixprovider/manifest.tsv` | the ONE canonical pin table (embedded; the recipe reads this same file) |
| `pkg/posixprovider/posixprovider.go` | manifest parsing, platform gating, cache resolution, provenance verification |
| `cmds/posixproviders/` | the sixteen registered provider tools + the `posix-providers` applet |
| `tools/posix-providers/build.sh` | the build recipe (fetch → verify → build → install → provenance) |
| `external/zigcc/` | the pinned portable C toolchain the recipe prefers |
