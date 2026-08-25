# Sprint 79: POSIX required-command interface status

This is the consolidated Sprint 79 status for the command interfaces selected
by Profiles C and D.  The normative target is POSIX.1-2016 (Issue 7), not GNU
Coreutils compatibility.  The report was initially synthesized at coreutils
`c048634` and is now a living status view reconciled with accepted evidence
closures.  Its inputs are the six accepted inventory audits
([1](go-batch-1.md), [2](go-batch-2.md), [3](go-batch-3.md),
[4](go-batch-4.md), [5](go-batch-5.md), [6](go-batch-6.md)), the accepted
[shell-selected audit](shell-selected.md),
[`posix-required-commands.tsv`](../posix-required-commands.tsv), and the
generated [interface ledger](../posix-required-command-interfaces.tsv).

The canonical ledger remains the authority: its 116 rows are 78 Go-owned, 22
shell-owned, and 16 external-provider-owned.  After the first three Go closure
batches and the five-command shell semantic batch, its evidence states are
**2 verified, 35 partial, and 79 unverified**.  An audit's “supportable pass”
finding does not become certification evidence until a stable command-specific
test reference and a separate ledger promotion are accepted.

## Exact in-scope inventory

The availability manifest contains 86 Go applets.  Eight names are selected by
the shell for a direct invocation (`echo`, `false`, `kill`, `printf`, `pwd`,
`test`, `time`, and `true`), leaving exactly **78 effective Go-owned commands**.
The six batches cover them once each:

| Batch | Commands |
| --- | --- |
| 1 | `at awk basename batch cat chgrp chmod chown cksum cmp comm cp crontab` |
| 2 | `csplit cut date dd df diff dirname du env expand expr file find` |
| 3 | `fold getconf grep head iconv id join ln locale logger logname ls mesg` |
| 4 | `mkdir mkfifo more mv newgrp nice nohup od paste pathchk pax pr ps` |
| 5 | `renice rm rmdir sed sleep sort split strings stty tabs tail tee touch` |
| 6 | `tput tr tsort tty uname unexpand uniq uudecode uuencode wc who write xargs` |

The other in-scope set is exactly **22 shell-selected commands**:
`alias bg cd command echo false fc fg getopts hash jobs kill printf pwd read sh
test time true umask unalias wait`.  The 16 external-provider commands are not
part of these interface audits.

## Confirmed Go implementation gaps

The following ranking is an engineering priority derived from the breadth and
likely certification impact of the accepted findings.  It is not a count of
failing VSC-PCTS test purposes.  Locale-only and evidence-only work is separated
below.  Commands can appear in both this table and the locale table when the
findings are independent.

| Priority | Commands | Confirmed required-interface gap |
| --- | --- | --- |
| Critical | `pr` | Header identity and a broad option/layout cluster diverge from Issue 7. |
| Critical | `getconf` | Much of the mandatory sysconf, pathconf, confstr, and minimum-name surface is absent. |
| Critical | `pax` | Broad required archive interface is absent or silently ignored. |
| Critical | `more` | Required options, `$MORE`, terminal paging, and the interactive command set are absent. |
| High | `dd` | No required SIGINT status path, no XSI EBCDIC conversions, and default stderr adds a non-POSIX transfer line. |
| High | `file` | Required `-d`, `-i`, and `-M` are absent; magic parsing is narrow; default symlink and required type-string behavior diverge. |
| High | `at`, `batch` | Missing `-m`/mail and required scheduling forms; blank programs are rejected; `batch` is not equivalent to `at -q b -m now`. |
| High | `chgrp`, `chown` | Required traversal and same-ID effects are now implemented and evidenced; translated diagnostics, Windows runtime support, privileged/kernel-owned set-ID and ctime behavior remain residual. |
| High | `cp` | Required same-file ordering, recursive umask handling, and `-p` set-ID behavior are fixed; locale catalogs, device-node runtime proof, selected error injection, and physical-symlink overwrite identity remain residual. |
| High | `od` | Required `-t` grammar/order and XSI offset processing are incomplete. |
| High | `mkfifo` | Omitted-`who` symbolic umask behavior is now evidenced; Windows runtime support, translated diagnostics, and filesystem-dependent special bits remain residual. |
| High | `mv` | Same-entry/symlink identity, update ordering, prompt/status, empty-directory rename equivalence, and cross-filesystem attributes are substantially fixed; Windows directory replacement, locales, and privileged special-node proof remain residual. |
| High | `newgrp` | Login environment and supplementary-group changes are incomplete; password prompt routing diverges. |
| High | `sed` | Required BRE/back-reference and locale-sensitive address/range semantics remain incomplete. |
| Medium | `date` | The XSI set-date operand `mmddhhmm[[cc]yy]` is absent. |
| Medium | `iconv` | Required `-c` is refused and omitted `-f`/`-t` handling is incomplete. |
| Medium | `id` | Default output does not correctly report distinct real and effective identities. |
| Medium | `logger` | Standard-input semantics and close-error reporting are incomplete. |
| Medium | `logname` | It falls back to the effective user instead of failing when no login name exists. |
| Medium | `mkdir` | `-m` is refused on Windows; this is a clear platform conformance gap. |
| Medium | `nice` | The child can execute before the adjusted priority is applied. |
| Medium | `nohup` | An internal error defaults to 125; Issue 7 requires 127 without depending on `POSIXLY_CORRECT`. |
| Medium | `paste` | Serial mode mishandles the required empty-file newline. |
| Medium | `pathchk` | Required filesystem/pathname constraints are not all enforced. |
| Medium | `touch` | A literal `-` operand cannot name the required file. |
| Medium | `uname` | `-a` includes the GNU `-o` extension rather than only the Issue 7 fields. |
| Low | `cmp` | Default difference output says `byte` where Issue 7 requires `char`. |

`ps` has a broad standalone Issue 7 interface gap, but the accepted audit found
the same implementation on the Profile A/B comparison route and therefore zero
Profile C Bashy-userland delta.  It remains required work for conformance, but
is lower priority for attributing a Profile C-only result than the table above.

### Locale, collation, and message-catalog residuals

These are implementation gaps where the required result changes with locale,
not merely untranslated cosmetic output:

- `awk` lacks `LC_COLLATE` and `LC_NUMERIC` language effects; `comm` implements
  only a very small locale/provider set.
- `fold`, `grep`, `join`, `locale`, and `ls` do not implement the required
  public-locale decoding, matching, collation, keyword, or time behavior.
- `sort`, `tr`, `unexpand`, `uniq`, and `wc` remain byte/C-locale engines for
  required character, blank, collation, or word semantics.
- `rm` does not use `LC_MESSAGES` `yesexpr`; `sed` still has locale-sensitive
  BRE/range defects; `strings` supports only the repository's bounded charset
  providers rather than arbitrary installed locales.
- `at`, `batch`, `cp`, and scheduling/time parsing have required affirmative,
  locale, or `TZ` effects in addition to their core gaps above.
- `du` now closes its audited core interface; translated diagnostics through
  `LC_MESSAGES`/`NLSPATH` remain an explicit catalog residual.
- `write` now implements its accepted terminal-delivery and `LC_CTYPE`
  behavior; translated diagnostics through `LC_MESSAGES`/`NLSPATH` remain an
  explicit catalog residual.

For `csplit`, `cut`, `date`, `dd`, `df`, `diff`, `dirname`, `du`, `env`,
`expand`, `expr`, `file`, and `find`, the accepted Batch 2 audit distinguishes
per-command category effects from translated diagnostics and `NLSPATH`.
`date` and `find` have focused German locale paths; that does not prove all
categories or message catalogs.  The remaining translated-diagnostic/catalog
work is a residual, not a reason to erase verified C/POSIX-locale behavior.
The same fail-closed distinction applies to diagnostics in other batches:
absence of a catalog is an implementation residual, while an implemented but
uncited locale branch is an evidence residual.

## Evidence-only and already-closed findings

The audits found no current core-interface defect for `basename`, `cat`,
`chmod`, `cksum`, `head`, `ln`, `mesg`, `renice`, `rmdir`, `sleep`, `split`,
`stty`, `tabs`, `tail`, or `tee`; they still lack clause-complete stable
evidence.  Batch 6 found `tsort` and `tty` supportable as verified at its audit
level, but their canonical ledger rows remain `unverified`, so this report does
not promote them.

The Batch 6 `tput` finding is now stale.  Canonical commits `f304822` and
`0b3f2ae` implement sequential `clear`/`init`/`reset`, continuation after an
unavailable operation, and the required exit semantics; `a4fbb7` refreshes its
test matrix.  `tput` is therefore an evidence/message-locale residual, not a
confirmed core implementation gap at `c048634`.

Other accepted fixes already present in this snapshot are:

| Commands | Canonical commits | Closed finding |
| --- | --- | --- |
| `renice`, `rm` | `2102a00`, `51eeaee` | Numeric-ID renice behavior and protected-directory recursive prompting. |
| `strings`, `tabs` | `a84afe9`, `4879065` | Locale-aware string scanning, default `TERM`, original-byte preservation, and valid U+FFFD handling. |
| `stty` | `1523713` | Atomic POSIX terminal-setting application. |
| `tput` | `f304822`, `0b3f2ae`, `a4fbb7` | Sequential POSIX operations and operation-specific statuses. |
| `df` | `8b4996a`, `64a9840`, `0f8e5fe`, `44fe7dc`, `e64042c` | POSIX/XSI units, portable rows, operand diagnostics, total-space semantics, and file-slot handling. |
| `du` | `b59ac9f`, `1a8cbba` | 512-byte defaults, `-k`, single-space output, dereference ordering, filesystem boundaries, hard-link scope, and output errors. |
| `xargs` | `cbdf03a`, `013ff7d`, `f702258`, `cfd341e` | XSI batching, limits/replacement semantics, and `LC_MESSAGES` affirmative expressions. |
| `who` | `dc1e0b0`, `ec5a8a7`, `cec16e9` | Required records/options, native ABI decoding, fail-closed platforms, `TZ`, and locale behavior. |
| `write` | `e56eb13`, `8e4840a`, `5dc3c12`, `11b8e4d`, `9e0b1aa`, `47b1d36` | Authenticated terminal selection, prescribed routing/framing, character handling, interruption, and native Linux utmp ABIs. |
| `uudecode`, `uuencode` | `2135fab`, `df06a1f`, `d0df145`, `2098b3d` | POSIX formats, pathname/mode semantics, safe overwrite behavior, and existing-output handling. |

These integrations close the implementation findings named here; they do not
promote any ledger row or constitute certification evidence.  `du` retains the
catalog residual above.  `write` deliberately fails closed on Darwin and on
Linux architectures whose native utmp ABI or PID-to-terminal ownership cannot
be authenticated, and also retains the catalog residual above.

### Active work not yet integrated

Active corrections must not be counted as this tree's implementation or
certification evidence:

| Command | Candidate/review state | Remaining gate |
| --- | --- | --- |
| `sed` | F1 integrated at `1a2c042`; broader correction remains active | F1 fixes ERE equivalence classes matching nothing. The separate `LC_COLLATE` routing and trustworthy locale equivalence/collation model remain open; see [`sed-locale-equivalence-review-s79.md`](../sed-locale-equivalence-review-s79.md). |

## Shell-selected Profile B routing and evidence

Profile C selects GNU Bash 5.3 invoked as `sh`; Profile D selects Bashy's
argv0=`sh` strict POSIX route.  `POSIXLY_CORRECT` is exported in the staged
environment.  Direct shell calls select the `sh` entrypoint, 13 shell-only
builtins, seven builtins that overlap Go applets, and the `time` keyword.  The
same-name Go applets remain relevant only to exec-style callers such as `env`,
`find -exec`, and `xargs`.

The accepted Profile B authority is the complete 9,337/9,337 GNU Bash
5.3/Bashy remote pair; the targeted ARM `fc` pair also completed 53/53.  These
are differential and routing evidence, not clause-complete POSIX evidence.
The five-command semantic closure batch added stable tests for `alias`, `echo`,
`false`, `true`, and `unalias`, and fixed strict-POSIX `echo` option parsing.
`false` and `true` are verified; the other three are conservatively partial:

| Commands | Selected route | Profile B evidence and remaining scope |
| --- | --- | --- |
| `sh` (245 TPs), `test` (207), `printf` (67) | entrypoint; builtin; builtin-over-Go | Critical-yield surfaces; aggregate parity does not prove their grammar, expression, formatting, locale, diagnostic, and status matrices. |
| `cd` (45), `command` (37), `fc` (28), `umask` (27) | shell builtins | Targeted fixes/pairs exist (`fc` 53/53), but no command has clause-complete stable shell evidence. |
| `kill` (18) | builtin-over-Go | `kill:TP9` and `kill_NE:TP8` fail identically in A/B and are shared suite/environment results, not Bashy defects; the shell timing fix is already integrated in the shell repository as `031d47e2`. |
| `bg` (17), `pwd` (17), `read` (13), `wait` (13), `fg` (12), `hash` (12), `getopts` (10), `jobs` (0) | shell builtins (`pwd` overlaps Go) | Source and targeted tests exist, but interactive/job, state, lookup, assignment, and diagnostic clauses are not closed.  `jobs` has zero likely testable TPs, not a waived interface. |
| `alias` (13), `unalias` (8) | shell builtins | Focused definition/query/removal, scope, status, error, and stream evidence exists; locale-sensitive diagnostics and all parser-timing consequences remain open, so both are partial. |
| `time` (16) | shell keyword-over-Go | Keyword routing is established; output, signal, status, ambiguity, and locale evidence is incomplete. |
| `echo` (12) | builtin-over-Go | Focused base/XSI, stream, status, and output-error evidence exists and a strict-POSIX option bug is fixed; the Profile D XSI feature-selection and locale branches remain open, so it is partial. |
| `false` (7), `true` (6) | builtins-over-Go | Complete status-only interfaces have independent semantic and routing evidence and are verified. |

This table accounts for all 22 names exactly once.  The detailed per-command
synopsis, route, sources, and evidence limits remain authoritative in the
[shell-selected audit](shell-selected.md).

## Reproducing coverage and ledger counts

Run from the coreutils repository root.  This verifier derives the effective
Go inventory, extracts the six audit headings, checks exact coverage/no
duplicates, checks all 22 shell headings, and prints the unchanged ledger
counts:

```sh
python3 - <<'PY'
import collections, csv, pathlib, re

root = pathlib.Path('.')
availability = list(csv.DictReader(
    open(root / 'docs/posix-required-commands.tsv'), delimiter='\t'))
ledger = list(csv.DictReader(
    open(root / 'docs/posix-required-command-interfaces.tsv'), delimiter='\t'))

overlaps = {'echo', 'false', 'kill', 'printf', 'pwd', 'test', 'time', 'true'}
go = {r['command'] for r in availability
      if r['coreutils_go_applet'] == 'yes' and r['command'] not in overlaps}
shell = {r['command'] for r in ledger if r['effective_owner'] == 'shell'}

seen = []
for path in sorted((root / 'docs/posix-interface-audits').glob('go-batch-*.md')):
    headings = re.findall(
        r'^##(?: [0-9]+\.)? `?([a-z][a-z0-9]*)`?(?:\s|$)',
        path.read_text(), re.M)
    seen += [name for name in headings if name in go]
shell_seen = re.findall(
    r'^### `([^`]+)`',
    (root / 'docs/posix-interface-audits/shell-selected.md').read_text(), re.M)

assert len(go) == 78
assert set(seen) == go and len(seen) == 78
assert not [n for n, count in collections.Counter(seen).items() if count != 1]
assert len(shell) == 22
assert set(shell_seen) == shell and len(shell_seen) == 22
assert not [n for n, count in collections.Counter(shell_seen).items() if count != 1]

print('effective owners:', collections.Counter(r['effective_owner'] for r in ledger))
print('evidence states:', collections.Counter(r['evidence_state'] for r in ledger))
print('coverage: 78 Go + 22 shell; no missing commands or duplicates')
PY
```

Run every effective Go-owned package test without accidentally testing the
eight direct-shell overlaps as Go owners:

```sh
go test $(python3 - <<'PY'
import csv
overlaps = {'echo', 'false', 'kill', 'printf', 'pwd', 'test', 'time', 'true'}
rows = csv.DictReader(open('docs/posix-required-commands.tsv'), delimiter='\t')
print(' '.join('./cmds/' + r['command'] for r in rows
               if r['coreutils_go_applet'] == 'yes' and r['command'] not in overlaps))
PY
)
```

Finally, confirm that producing or refreshing this report did not alter the
generated evidence state:

```sh
git diff --exit-code -- docs/posix-required-command-interfaces.tsv \
  docs/posix-required-command-interfaces.md
```
