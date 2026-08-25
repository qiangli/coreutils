# POSIX Go evidence closure: wave 5A

This unique Wave 5A audit covers exactly `at`, `batch`, `crontab`, `dd`,
`df`, and `diff` against POSIX.1-2016 (Issue 7).  The six rows are Go-owned;
POSIX is normative and existing upstream/non-POSIX behavior remains an
extension only outside `POSIXLY_CORRECT`.

Every row is promoted to `partial`, not `verified`.  Each has hermetic,
package-local evidence for its accepted synopsis/options, operands, stream
routing, durable effects, and primary exit paths.  None supplies translated
`LC_MESSAGES` diagnostics or XSI `NLSPATH` catalog lookup; the scheduling
applets also retain implementation-defined daemon/access-policy boundaries.

## Confirmed correction

`dd` previously wrote its deterministic GNU-style `N bytes copied` status
extension even in POSIX mode.  Issue 7 specifies the POSIX-locale completion
record-count (and applicable truncated-record) lines; Wave 5A now suppresses
only the extra byte-count line whenever `POSIXLY_CORRECT` is present, including
an empty value.  Outside POSIX mode the existing extension is preserved.
`cmds/dd/dd_test.go#TestDdPOSIXStatusOmitsGNUByteCountExtension` discriminates
both modes and both present-value forms.

## Interface verdicts

| Command | Verdict | Audited interface and remaining exact boundary |
| --- | --- | --- |
| `at` | partial | `-f`, `-l`, `-m`, `-q`, `-r`, and `-t`; concatenated timespec operands, standard-input/file job sources, `SHELL`, `TZ`, and `LC_TIME`; owner-isolated list/remove; job persistence; stdout listings, stderr submission confirmations, and success/error status are covered. Queue/list synopsis separation and unsuccessful removal atomicity are pinned. Catalog diagnostics, all locale grammars, and the implementation-defined access directory remain. |
| `batch` | partial | No options/operands; standard input is retained as a shell program with captured environment, cwd, and umask; it is represented as queue `b`, completion-mail enabled, load-governed `at ... now`; stdout is empty and the successful confirmation is stderr. Locale catalogs and the implementation-defined access/load scheduler remain. |
| `crontab` | partial | Standard-input and file replacement, `-l`, `-e`, and `-r`; one-file arity; silent successful installation; exact source round-trip; atomic rejection; `EDITOR`; persisted shell/home/default environment effects; and exit paths are covered. The optional-user interface is deliberately fail-closed, with catalog diagnostics and platform daemon policy residual. |
| `dd` | partial | All Issue 7 operands and conversions listed in the ledger, stdin/stdout defaults, file effects, status/error paths, SIGINT, FIFO handling, `conv=sync`, and XSI codeset conversions have focused coverage. Wave 5A adds POSIX-mode status discrimination. Locale message catalogs remain absent. |
| `df` | partial | XSI `-k`, `-P`, and no-argument `-t`, their conflict, operand diagnostics/continuation, 512/1024 units, free file slots, allocated space, portable rows, and statuses are covered. Platform mount discovery and translated diagnostics remain implementation-dependent. |
| `diff` | partial | `-b`, `-c`/`-C`, `-e`, `-f`, `-r`, `-u`/`-U`, two operands including one standard-input operand, normal/ed/context/unified output, fixed C/POSIX timestamp formatting with `TZ`, directory effects, output failures, and 0/1/2 status paths are covered. Non-C `LC_TIME`, locale collation/message catalogs, and all implementation-defined directory/FIFO cases remain residual. |

## Focused evidence

- `cmds/at/at_test.go`: creation/list/remove, `-f`, queue filtering, empty
  input, timespec/`-t`, locale/timezone, mail, owner isolation, stream-write
  failure, and invalid synopsis/status cases.
- `cmds/batch/batch_test.go`: no-option parser, empty stdin, queue/mail/load
  side effects, stdout/stderr separation, environment/timezone, and errors.
- `cmds/crontab/crontab_test.go`: install/list/remove/edit, source preservation,
  atomic validation, stdin/file effects, access failures, and stream/status.
- `cmds/dd/dd_test.go`, `conv_test.go`, FIFO/fullblock/signal tests: operand,
  conversion, stream, side-effect, error, and interruption coverage; Wave 5A's
  POSIX-status fixture is fully hermetic.
- `cmds/df/df_test.go`: XSI units/format/total/free-slot/operand/status cases.
- `cmds/diff/diff_test.go` and FIFO/path-resolver tests: output dialects,
  operands, streams, timestamps, traversal, write failure, and statuses.

The ledger details the precise per-command fields and test identifiers.  This
audit intentionally leaves global evidence totals and the consolidated Sprint
79 report for root reconciliation.
