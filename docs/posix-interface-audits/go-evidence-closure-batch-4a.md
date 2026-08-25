# POSIX Go evidence closure: owned batch 4A

This audit covers `comm`, `csplit`, `cut`, `dirname`, and `expand` against
POSIX.1-2016 (Issue 7). The normative sources are the Open Group command
pages linked below. GNU behavior is not used as POSIX evidence.

Each ledger row now carries the exact applicable operand rules, special-token
semantics, standard-input/-output/-error behavior, side effects, status
contract, and command-specific test references, promoting five previously
`unverified` Go-owned rows to `partial`. All five are deliberately `partial`,
not `verified`: every page lists `LC_MESSAGES` and the XSI `NLSPATH` catalog
behavior, and none of these packages implements translated diagnostics or
message catalogs. Three of the five (`comm`, `csplit`, `expand`) also
retain a genuine locale-category implementation gap named below; `dirname`
retains only the catalog residual, and `cut`'s locale-category gap was
subsequently closed by the issue 736 work recorded in
`cmds/cut/POSIX-issue736-audit.md` (its row below reflects that closure). The command-specific residuals table makes
each remaining boundary explicit.

Review found and fixed one confirmed core-interface defect: in POSIX mode,
`expand` now terminates when an operand cannot be accessed or read instead of
continuing with later operands. GNU-compatible default mode retains GNU's
continue-after-diagnostic behavior. The rest of the batch is focused hermetic
evidence closing gaps named by the accepted Batch 1/2 audits, plus a
transcription of the normative semantics into the ledger.

## Verdicts and exact residuals

| Command | Issue 7 source | Verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `comm` | [Issue 7 comm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/comm.html) | partial | Translated diagnostics absent. `LC_COLLATE` operation is implemented only for the two accepted `de_DE` ISO-8859-1 aliases via `pkg/collate`, which exists only on Linux amd64/arm64; every other installed locale returns exit 2 instead of comparing, and all non-C/POSIX operation fails off that platform (an implementation gap, not merely a message residual). The real collator path is evidenced through the `fakeCollator` seam, not a live installed locale. GNU default order checking (a disorder is diagnosed only after an unpairable line is seen) is a pinned reading of the "results are unspecified" latitude, not a POSIX requirement. `--total`, `--output-delimiter`, `-z`/`--zero-terminated`, `--check-order`, and `--nocheck-order` are GNU extensions beyond the `[-123]` synopsis. The stdout-write-failure and injected-read-error evidence gaps named by go-batch-1 are now closed. |
| `csplit` | [Issue 7 csplit](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/csplit.html) | partial | Translated diagnostics absent. Locale-directed BRE collation and character-class handling are unimplemented (byte/C semantics through `pkg/bre`). The CONSEQUENCES-OF-ERRORS cleanup (files created before an error are removed unless `-k`) is provable only through a filesystem write failure, because this implementation resolves every split point before writing any piece — a resolve-time error creates no files, so cleanup is a no-op there; the new `-k` test forces the write failure with a directory colliding with a piece name. GNU extensions beyond `[-ks] [-f prefix] [-n number]` include `{*}`, `-b`/`--suffix-format`, `--suppress-matched`, `-z`/`--elide-empty-files`, the deprecated `-q`/`--quiet` alias, and the `%i`/`%u` suffix-format aliases. The escaped `\/` and `\%` delimiter forms and `-k` cleanup gaps named by go-batch-2 are now closed. |
| `cut` | [Issue 7 cut](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html) | partial | Translated diagnostics absent. The locale-category gap this batch originally named is closed (issue 736): `-c`, `-b -n`, and the `-d delim` character count now follow the invocation `LC_CTYPE` (`LC_ALL` > `LC_CTYPE` > `LANG` > POSIX default) over a bounded pure-Go inventory — C/POSIX byte semantics, the carried C/POSIX UTF-8 aliases (including multibyte `-d` delimiters), and the `de_DE.ISO-8859-1` certification aliases — always emitting exact original bytes; any other effective `LC_CTYPE` fails with status 1 before input or output. See `cmds/cut/POSIX-issue736-audit.md` for the evidence table. Remaining residuals: translated diagnostics, and the bounded locale inventory itself (other installed locales fail closed rather than being interpreted). `-b` (bytes), the three mutually exclusive forms, list grammar numbered from 1 with no `0`/decreasing range, `-d`/`-s`, and the no-delimiter passthrough are verified. `--complement`, `-O`/`--output-delimiter`, `-z`/`--zero-terminated`, and `-w`/`--whitespace-delimited` are GNU extensions beyond the three POSIX synopsis forms; `-n` is a POSIX byte-mode option and is evidenced. The standard-output write-error evidence gap is now closed. |
| `dirname` | [Issue 7 dirname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html) | partial | Translated diagnostics absent — the only normative surface not covered. POSIX encoding rules make `/` (0x2F) safe to locate bytewise; the focused test directly proves preservation of non-ASCII UTF-8 components, while the portable-encoding conclusion follows from that normative rule rather than a runtime locale matrix. Multiple operands and `-z`/`--zero` are GNU extensions beyond the single-operand `dirname string` synopsis; the pathname algorithm (trailing-slash removal, all-slash → `/`, no-slash → `.`, no canonicalization) is verified. |
| `expand` | [Issue 7 expand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html) | partial | Translated diagnostics absent. Locale-directed blank/column classification is unimplemented: display columns come from fixed UTF-8 width (`go-runewidth`), or from raw byte columns under the `-U` extension, rather than the invocation `LC_CTYPE`. The single-size and ascending-list `-t tablist`, the tab-beyond-last-stop single-space rule, `<backspace>` column decrement (floored at zero), `-` stdin, POSIX immediate termination on operand-access failure, GNU-default continuation, and output failure are evidenced. `-i`/`--initial` and `-U`/`--no-utf8` are GNU extensions beyond `[-t tablist]`. |

## Tests added in this batch

Each test is hermetic and, where it targets an error path, discriminates the
required behavior from the prior or a deliberately broken implementation.

- `cmds/comm/comm_test.go` — `TestCommStandardOutputWriteFailure` (a
  `failingWriter` surfaces the error at the final `bufio` flush → exit 1 with
  `comm: write failed`) and `TestCommInputReadErrorIsDiagnosed` (an `errReader`
  yields a non-EOF error → `comm: -:` diagnostic, exit 1). These close the two
  evidence gaps go-batch-1 named.
- `cmds/csplit/csplit_test.go` — `TestCsplitEscapedDelimiterInRegex` covers
  both `/a\/b/` and `%a\%b%`, and `TestCsplitKeepFilesRetainsOutputOnError`
  (a directory colliding with the second piece name forces a write failure
  after `xx00` is created; without `-k` the created file is removed, with `-k`
  it is retained).
- `cmds/cut/cut_test.go` — `TestCutStandardOutputWriteError` (a `failingWriter`
  → exit 1 with `cut: write error:`).
- `cmds/dirname/dirname_test.go` — `TestDirnamePOSIXSingleOperandByteSafety`
  (non-ASCII directory components survive the bytewise `/` split verbatim).
- `cmds/expand/expand_test.go` — `TestExpandOperandAccessFailureModes` proves
  immediate termination under `POSIXLY_CORRECT` and GNU-compatible continuation
  otherwise; `TestExpandStandardOutputWriteError` proves a `failingWriter`
  yields exit 1 with `expand: write error:`.

## Gate notes

The manifest routing-evidence lane resolves `bashy:` and `sh:` references
against sibling `bashy`/`sh` checkouts next to this repository; both are
present in this Weave workspace, so `scripts/posix_manifest.py --check` and the
`posix_manifest` unit suite validate normally. All 22 pre-existing
`shell_routing_evidence` references are preserved untouched.

After reconciliation with concurrent Wave 4B, the manifest reports 2 verified /
45 partial / 69 unverified. The generated interface document, applet matrix,
global count assertion, and consolidated status are regenerated together.
