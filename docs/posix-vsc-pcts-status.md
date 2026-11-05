# POSIX conformance status — VSC-PCTS

## Latest integrated run: 2026-11-04 (S561)

Frozen candidate: bashy `f1bcd711`, sh `74586fde`, coreutils `211789fa`,
harness `0b2b185d`. Profile B shell isolation: 493/493 PASS group, zero blockers.
Full Profile D: 117 sets, 9,337 test purposes; 9,270 PASS group, 4 FAIL,
54 UNRESOLVED, 9 INSPECT. The **67 non-passes** match the previous full run;
engineering closed with no regression, not a clean full-profile pass.

The required base is 102 targets (92 Go applets + 10 Bashy shell commands),
zero external utility providers, plus Go `gencat`. Full-catalog availability
counts and earlier diagnostic denominators are separate from this claim.

[Source evidence](https://github.com/qiangli/bashy/blob/main/docs/s561/a4-licensed-results.md) retains the exact candidate and artifact record.
[S571](https://github.com/dhnt/umbrella/blob/main/docs/plan-s571-posix-certification-readiness.md) owns S355 authority rulings for 53 symlink setup results,
`man:4` and four `more` failures, plus S377-supported manual decisions for
nine current-journal INSPECT results. Packets remain unsent; full readiness
and certification remain open. Preserve raw codes separately from dispositions.

## Historical utility diagnostics

The records below describe earlier candidates and provider layouts. Their
ranked failure lists are not the current work queue; consult S571.

The utilities half of the POSIX certification effort measures **this**
repository: the coreutils multicall binary is placed on `PATH` ahead of the
distro's tools and scored by The Open Group's VSC-PCTS2016 suite. (The other
half is the shell, `bashy cmd/bash`.)

Results are published for **conformance-work purposes** under Open Group ticket
**#280298**. No "certified" / "passes the Open Group suite" claim is made or
permitted, no Open Group mark is used, and the suite is never redistributed.

## Historical: 2026-08-01, x86_64, both arms on one host

SUT: coreutils `c495aaa` (bashy tag `vsc-pcts-2026-08-01`). Full POSIX08
scenario, 121 test sets, per-tset 10-minute cap.

| arm | PASS | FAIL |
|---|---|---|
| bashy userland (this repo on PATH) | 5,166 | 2,073 |
| GNU baseline (stock toolchain) | 5,625 | 1,837 |

**The number that means something is the delta: +313.** Both arms run identical
tests, so most volume cancels — `m4`, `bc`, `make`, `ctags`, `strip`, `ps` and
`chown` fail *identically* in both arms, being utilities this repo does not
ship or environment properties rather than defects here.

### Remaining work at the 2026-08-01 snapshot, ranked

```
find +40   sed +35   date +34   dd +20   cp +16   pr +15   env +12
ls   +11   awk  +7   rm    +6   cut +6   cat +6   xargs +5  od  +5
mkfifo +5  then +4/+3/+2/+1 across ~45 further sets
```

- **`date` +34 is the biggest single finding and is untriaged.** GNU fails 2
  where we fail 36, and it does not appear in the previous (arm64) measurement
  at all. Either a regression or newly-exercised coverage — that run's container
  had no non-English locale, and `date` output is locale-dependent. **Start
  here.**
- **`find` +40** is the largest absolute delta. (Since addressed, 2026-08:
  `-exec`/`-ok` shipped under the command-wrapper exception, along with the
  POSIX primaries the failure list implicated — time/link/user/group tests,
  -perm, -xdev, -depth, -H/-L, loop detection. Re-measure on the next run.)
- **`sed` +35** is what remains after the `pkg/bre` work halved it. Since `grep`
  closed almost entirely on the same engine, the residue is sed's own feature
  grammar rather than the shared BRE/ERE code.

### The `pkg/bre` cluster is confirmed closed

| tset | before (arm64) | after (x86_64) |
|---|---|---|
| grep | +59 | **+3** |
| expr | +18 | **0** |
| id | +12 | +1 |
| mkdir | +8 | +1 |
| xargs | +18 | +5 |
| sed | +69 | +35 |
| **total** | **+396** | **+313** |

Directional only: the architecture changed *and* the denominator moved
(provisioning a locale, read-only and no-symlink filesystems and real special
files means tests that were formerly skipped now run).

`at`, `diff` and `tail` are excluded as not comparable — they hit the per-tset
cap under the bashy arm but not the GNU arm, so their 0 fails there is
truncation, not success.

## Reproducing

The full procedure lives in the private harness kit
(`github.com/qiangli/vsc-pcts-harness-kit` → `END-TO-END.md`): host, licensed
suite staging, build, the `configure` answer set, SUT wiring, the arm runner and
the analysis. It is private because it concerns a licensed suite; the results
above are publishable, the procedure's dependencies are not.

Host safety is not optional for this work — see
`../docs/conformance-test-landmines.md`. The suite contains adversarial inputs
(`split -n 3 /dev/zero`, `sort /dev/random`, `chmod -R --preserve-root`
bypasses) that are safe only in a disposable, memory- and PID-capped container
with a non-root user and no host-root mount. Several of those are open defects
in this repo and are in scope for the certification work, not merely hazards to
route around.
