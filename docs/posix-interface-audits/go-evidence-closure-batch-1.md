# POSIX Go evidence closure: owned batch 1

This audit covers `basename`, `cat`, `cksum`, `env`, `head`, `rmdir`,
`sleep`, `tee`, `tty`, and `tsort` against POSIX.1-2016 (Issue 7). The
normative sources are the command pages linked below. GNU-only behavior is not
used as POSIX evidence.

The ledger rows now contain the exact applicable synopsis, options, operands,
stream behavior, side effects, status contract, and command-specific test
references. All ten rows are deliberately `partial`, not `verified`: every
page carries `LC_MESSAGES` and the XSI `NLSPATH` catalog behavior, while these
packages have no translated diagnostic/catalog implementation or focused
evidence. The command-specific residuals below make the remaining boundary
explicit rather than laundering C/POSIX-locale coverage into full Issue 7
closure.

| Command | Issue 7 source | Current verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `basename` | [Issue 7 basename](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/basename.html) | partial | Translated diagnostics and message-catalog routing remain absent. The permitted null-string and exactly-`//` choices now have stable tests. |
| `cat` | [Issue 7 cat](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html) | partial | Injected mid-stream stdin read failures, directory and dangling-symlink operands, `/dev/null`, and FIFO-through-symlink streaming are now stable, each with operand continuation and exact diagnostics; repeated `-` proves one continuing stdin stream. Diagnostics remain unlocalized in non-C locales (superseded localization product gap, not an interface failure). Remaining: Windows special-file behavior and the process-level SIGPIPE/default broken-pipe disposition, which must not be recorded as silent success (XCU cat ASYNCHRONOUS EVENTS is Default). |
| `cksum` | [Issue 7 cksum](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cksum.html) | partial | Locale diagnostics remain absent; focused long-length CRC, special-file, and injected read-error evidence remains open. Mixed failed/successful operand continuation is now tested. |
| `env` | [Issue 7 env](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html) | partial | Its environment construction, modified-PATH lookup, direct invocation, streams, and status partition are covered; translated diagnostics/catalogs remain absent. |
| `head` | [Issue 7 head](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/head.html) | partial | Locale diagnostics and injected input-read failure evidence remain open. A focused output-write-error test now closes that status/diagnostic branch. |
| `rmdir` | [Issue 7 rmdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rmdir.html) | partial | Locale diagnostics remain absent; focused permission-failure and stdin-nonconsumption evidence remains open. Empty-directory, operand order, `-p`, and error continuation are covered. |
| `sleep` | [Issue 7 sleep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sleep.html) | partial | Locale diagnostics and process-boundary SIGALRM/other-signal evidence remain open. A real integral one-second lower-bound test now proves the mandatory suspension behavior. |
| `tee` | [Issue 7 tee](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tee.html) | partial | Locale diagnostics, a portable streaming probe, input-read/close errors, and the default SIGINT boundary remain open. This batch fixes the confirmed missing stdout-write diagnostic and adds Linux `/dev/full` evidence that one opened-file write failure does not stop stdout or another file. |
| `tty` | [Issue 7 tty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tty.html) | partial | Locale-sensitive nonterminal output is absent, and the `!linux && !darwin && !windows` fallback incorrectly reports every POSIX terminal as non-terminal. Linux/Darwin PTY, non-terminal, write-error, and operand evidence is retained without claiming portable closure. |
| `tsort` | [Issue 7 tsort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tsort.html) | partial | Locale diagnostics plus injected input-read and output-write error evidence remain open. Pair grammar, identical-item presence, stdin/file selection, ordering, and error status are covered. |

The `tee` correction is intentionally bounded. Issue 7 gives only a write
failure on a successfully opened *file operand* the special continue rule.
A non-signal standard-output write failure therefore follows Utility
Description Defaults: diagnose it and return non-zero. SIGPIPE retains its
separate asynchronous-event behavior.

No `tty` portability change is included here. A correct implementation needs
a real ttyname-equivalent strategy and tests on each additional supported
POSIX target; returning a guessed name would be worse than retaining the
explicit partial verdict.
