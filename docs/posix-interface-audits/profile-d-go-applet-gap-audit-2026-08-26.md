# Profile D POSIX-required Go applet gap audit

Date: 2026-08-26  
Audited baseline: `aee4ff5eb827f5e600a8f2e57ba5b8fc34ca0222` (`coreutils` main)  
Sprint: Sprint 82, Profile D zero-blocker POSIX closure

> **Historical reconciliation.** The 90/14/12 availability and 82/22/12
> effective counts below are fixed to the audited baseline `aee4ff5e`, when
> `make` and `bc` were still counted as pinned external providers. They were
> subsequently reclassified as pure-Go applets, so the current generated ledger
> is availability **92 Go / 14 shell / 10 external provider** and effective
> selection **84 Go / 22 shell / 10 external provider**, with exactly ten
> providers (`ar, ctags, ex, localedef, lp, m4, man, nm, strip, vi`). The
> per-command findings in this snapshot remain valid for the 90 applets audited;
> `make` and `bc` were audited under `posix-external-providers.md` at that
> baseline. See `../posix-required-commands.md` for current counts.

## Scope and authority

This is a read-only interface and implementation audit of all 90 commands
classified as `go_applet` in `docs/posix-required-commands.tsv`. Eight of
those implementations (`echo`, `false`, `kill`, `printf`, `pwd`, `test`,
`time`, and `true`) are currently shell-selected; the remaining 82 are the
effective Go owners in Profiles C and D.

The normative certification authority is the Open Group POSIX.1 Issue 7,
2016 Edition. GNU Coreutils 9.11 controls extensions only for commands that
GNU ships. Other extensions use the command's actual official upstream family
as required by `docs/reference-policy.md`. POSIX behavior wins on conflicts.

Primary references:

- [Open Group Issue 7 utility index](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/contents.html)
- [GNU Coreutils 9.11 manual](https://www.gnu.org/software/coreutils/manual/coreutils.html)
- `docs/posix-required-command-interfaces.tsv`
- `docs/posix-interface-audits/sprint-79-consolidated.md`

## Reproducible checks and result

```sh
python3 scripts/posix_manifest.py --check
python3 scripts/applet-matrix.py --check
python3 scripts/posix_manifest.py --require-owned-source-complete
```

The manifest and applet-matrix consistency gates pass. The exact ledger is
116 required names, with availability 90 Go / 14 shell-only / 12 providers
and effective ownership 82 Go / 22 shell / 12 providers. Across the 90 Go
implementations the ledger records 3 `implemented` (`false`, `nice`, `true`),
87 `partial`, and 0 `verified`.

No absent mandatory POSIX option spelling or option-argument parser form was
confirmed. The `kill` dynamic `-signal_name`/`-signal_number` forms require
manual inspection because the conservative static scanner cannot represent
them. This option-token result is not semantic completeness: the clause-level
audit found the blockers below despite green package tests.

## Confirmed Profile D implementation blockers

| Rank | Command | Confirmed missing or incorrect behavior | Corrective lane |
| ---: | --- | --- | --- |
| 1 | `ed` | Required commands and behavior including global/inverse-global forms, join, marks and marked addresses, move/copy, prompt toggle, read, undo, append-write, shell escapes, command suffixes, script-vs-interactive errors, signals/`ed.hup`, and list/address/current-line rules were absent. | Implemented on `feature/posix-ed-completion` at `0818e88ed7faeca770456d043aa3012dc15a1a17`; review/merge and Profile D replay pending. |
| 2 | `mailx` | Startup files, `-n`/`-i`, message states and MBOX disposition, complete message-list grammar, mandatory receive commands and variables, replies/followups, composition escapes, signal/DEAD behavior, prompts, and headers were incomplete. Local-file-only delivery remains the deliberate scope; SMTP is not required. | Independently reviewed and merged to `main` at `20cedcbeb0d357a0a2180c5fd7dafa16d54468b2`; review added command-boundary output-error and short-write handling. Profile D PTY replay remains pending. |
| 3 | `patch` | Mandatory `-D` and `-e` were rejected; `.orig`, reversed/already-applied prompting, filename selection/prompting, reject formats, indentation, and multi-file `-o` behavior were incorrect or incomplete. | Implemented on `feature/posix-patch-completion` at `607f02dfb26abaed5e6317fbe46a05070997a0c8`; review/merge and Profile D replay pending. |
| 4 | `talk` | Local-user-only addressing is permitted, but the required simultaneous screen-oriented interaction remains incomplete: separate regions, character-level display, erase/kill handling, and terminal-capability behavior need closure and PTY evidence. | Next parallel implementation lane. |
| 5 | `pax` | The advertised `cpio` format rejects append/update. Resolve whether the implementation-defined format rule permits this restriction before classifying it as a defect. | Focused specification ruling and differential test. |

The three completed feature branches were based exactly on the audited main
commit and deliberately were not merged into the Profile D run already in
progress. Each reports focused/race tests, the full short repository suite,
manifest/matrix gates, cross-platform vet/build canaries, and push hooks green.
They remain unverified until independent review and Profile D execution.

## Integration and evidence residuals

These are not known missing option parsers, but remain fail-closed certification
or target-platform evidence:

- `at`, `batch`, `crontab`: live scheduler/daemon handoff, load gating,
  policy directories, privilege, and local completion mail.
- `date`, `logger`, `newgrp`, `renice`, `id`, `logname`: privileged clock,
  syslog, credential, session, and scheduler products.
- `df`, `du`, `pax`: mounted cross-device, accounting, special-file, device,
  ownership, and permission products.
- `stty`, `tabs`, `tput`, `more`, `who`, `write`, `talk`: live PTY,
  terminfo, login database, terminal ownership, interruption, and framing.
- Locale-sensitive applets use the carried C/POSIX, selected UTF-8, and
  `de_DE` products. Unsupported installed locales fail closed; arbitrary
  installed-locale breadth remains an integration boundary.
- Lower-risk I/O/error edges remain in the accepted command-specific audits,
  notably `cksum`, `head`, `cmp`, `cp`, `cat`, `tee`, `tsort`, `uname`,
  `basename`, `dirname`, `env`, `ln`, `rmdir`, `sleep`, `split`, `strings`,
  `tail`, and `xargs`.

## GNU Coreutils 9.11 compatibility gaps

These are extension gaps and do not block POSIX Profile D:

- `cp`: `--keep-directory-symlink`; complete `--update[=UPDATE]` values.
- `cut`: `--no-partial`; `--whitespace-delimited=trimmed`.
- `df`: ignored compatibility option `-v`.
- `env`: `--block-signal[=SIG]`, `--default-signal[=SIG]`, and
  `--list-signal-handling`.
- `ln`: `-d`, `-F`, and `--directory`.
- `mv`: `--exchange`, `--no-copy`, and complete `--update[=UPDATE]` values.
- `pr`: `-c`/`--show-control-chars`, `-v`/`--show-nonprinting`, and the bare
  `-S`/`--sep-string` default.
- `split`: `-u`/`--unbuffered`; `-n` values `K/N`, `l/K/N`, `r/N`, and
  `r/K/N`.
- `od`: bare `-S` and `-w` defaults.
- `wc`: `--debug`.
- `rm`: full `--preserve-root=all`; `--one-file-system` behavior.

Accepted-but-approximated extensions violate the repository's upstream
semantics policy even though they are outside Profile D. In particular,
`cp --reflink`, `cp --sparse`, context/progress/default-attribute options,
`mv` context/progress, and `mkfifo --context` must eventually be implemented
as documented or rejected clearly rather than silently acting as no-ops.

## Sprint acceptance for this task

1. Independently review and merge the `ed` and `patch` branch tips (`mailx` is merged).
2. Complete `talk` terminal semantics with PTY evidence.
3. Resolve the `pax` cpio ruling and fix it if POSIX requires append/update.
4. Run focused and cross-platform gates after integration.
5. Rebuild an identical disposable Profile D VM from exact recorded pins and
   require the official Profile D campaign to reach 100% pass with no unresolved
   inspection results.
6. Keep GNU extension work in a separate P1 queue unless an accepted silent
   approximation violates the repository contract.
