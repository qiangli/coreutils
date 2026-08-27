# Profile D `ls` Repair — 2026-08-27

Primary contract: [The Open Group POSIX.1 Issue 7, 2016 Edition `ls`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html).

## Interface acceptance

The POSIX route accepts all 26 required option spellings (`-A`, `-C`, `-F`,
`-H`, `-L`, `-R`, `-S`, `-a`, `-c`, `-d`, `-f`, `-g`, `-i`, `-k`, `-l`,
`-m`, `-n`, `-o`, `-p`, `-q`, `-r`, `-s`, `-t`, `-u`, `-x`, and `-1`).
None has a required option-argument. `TestPOSIXIssue7RequiredOptionSurface`
executes every spelling independently in POSIX mode. Operand acceptance covers
the default `.` operand, multiple pathnames, the `--` special token, and
option-looking pathnames.

The existing clause-focused suite covers the required formatting, metadata,
sorting, recursive traversal, symbolic-link, diagnostic-continuation, block
size, and mutually-exclusive-option behavior. This repair adds direct evidence
for three Profile D findings:

* POSIX `-C` and `-x` use a uniform column pitch within one directory.
* `-f` takes effect at its position: it enables `-a` and disables an earlier
  `-l`, `-r`, `-S`, or `-t`; a later sorting or format option remains effective.
* Once POSIX operand processing begins, a later `-l` or `--` pathname is not
  reinterpreted by the command's order-sensitive scanners.

## Profile D disposition

The paired Bashy/GNU-control run reported the same environmental or harness
outcome for block-allocation fixture totals, inaccessible-root fixtures,
directory access-time observations, signal/core handling, ACL markers, empty
or looping symlinks, and unavailable non-C locales. Those matched-control
items are not converted into applet behavior changes. They remain explicit
rerun or capability residuals.

The actual applet deltas were uniform columns, ordered `-f`, and post-operand
option-looking pathnames. Their focused tests are
`TestPOSIXColumnsUseUniformWidth`,
`TestPOSIXFDisablesEarlierSortLongAndReverse`, and
`TestPOSIXStopsOptionParsingAtFirstOperand`.

## Honest residuals

* Non-C `LC_COLLATE`, `LC_TIME`, and `LC_MESSAGES` behavior still requires
  locale support outside this applet.
* ACL/security-context display depends on host metadata support.
* Signal disposition and core-file behavior belong to the process launcher
  and are not established by an in-process command test.
* Filesystem allocation and directory-atime assertions require a fixture and
  mount whose behavior differs measurably from the matched GNU control run.
