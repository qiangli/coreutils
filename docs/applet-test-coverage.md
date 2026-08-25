# Shipped applet test coverage

Status: release policy, audited 2026-08-05.

Every command package imported by `cmds/all` must contain package-local
behavioral tests. `scripts/applet-test-coverage.sh` enforces the structural
minimum and `scripts/crossvet.sh` runs it as part of the release gate. A command
that does not meet the bar is removed from `cmds/all`, which also removes it
from the multicall inventory and help/list output.

The audit started from the staged 143-applet inventory used by the VSC
campaign. Two shipped packages had no package-local tests: `atq` and `atrm`.
Both now have state-backed behavioral tests covering filtering/listing,
ID/name removal, persistence, empty state, invalid operands, and diagnostics.
The `ncal` and `sntp` aliases now execute through their own registered tools in
tests rather than relying only on registration assertions.

The second pass found two privileged implementations with only superficial
coverage:

| command | observed coverage | release decision |
|---|---|---|
| `chroot` | missing-operand path only | withheld from `cmds/all` pending bounded privileged integration tests |
| `runcon` | help output only | withheld from `cmds/all` pending bounded SELinux integration tests |

The current generated inventory contains 142 command packages and 147 applet
names. Every shipped package has a package-local test.
This is a structural floor, not a claim of option-complete conformance: VSC,
differential, platform, and package tests remain separate coverage dimensions.

The five additional applet names over package count are aliases registered by
tested packages: `[` (`test`), `gunzip` and `zcat` (`gzip`), `ncal` (`cal`), and
`sntp` (`ntp`). Alias behavior must be invoked under its own name in tests when
name-dependent parsing or diagnostics exist.

See `applet-matrix.md` for the complete applet-by-applet GNU, POSIX-cert, alias,
and package-test matrix; `applet-matrix.tsv` is its machine-readable counterpart.
The generated `posix-required-commands.md` preserves the 116-name harness
coverage map. The separate `posix-required-command-interfaces.md` is an
incomplete evidence ledger for those names; it does not claim conformance.
