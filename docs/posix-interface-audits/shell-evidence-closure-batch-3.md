# POSIX shell evidence closure: wave 3

Wave 3 covers the four shell-selected interfaces `printf`, `read`, `test`,
and `umask`. POSIX.1-2016 Issue 7 is the normative source; GNU Bash 5.3 is
used only to preserve Bash behavior where POSIX is unspecified or silent.
The Profile B routing tests independently prove that Bashy selects these
shell builtins rather than the registered Go applets where both exist.

All four ledger rows move from `unverified` to `partial`. The focused evidence
is clause-oriented and discriminating, but it does not claim translated
diagnostics, arbitrary non-C locale behavior, or every exceptional OS state.

| Command | Evidence | Remaining boundary |
| --- | --- | --- |
| `printf` | `TestPrintfIssue7Interface` covers required format arity, escapes, ordinary conversions, flags/width/precision, format reuse, missing arguments, `%b` and `\\c`, quote-prefixed numeric values, diagnostics, output failure, and stdin preservation. | Non-C `LC_NUMERIC`/multibyte behavior and translated diagnostics remain. Empty/missing `%c` NUL output is explicitly Bash compatibility because POSIX permits either no byte or NUL. |
| `read` | `TestReadIssue7Interface` covers IFS field assignment, last-variable remainder, excess variables, trimming, backslash continuation/escaping, `-r`, stdin, and statuses. | Locale/message-catalog and interactive `PS2` edges remain. Unterminated non-text input, omitted-variable `REPLY`, and invalid identifiers are classified as Bash compatibility/extensions, not POSIX evidence. Review exposed and fixed the real Bash mismatch: invalid identifiers now return 1, matching Bash 5.3. |
| `test` | `TestTestIssue7Interface` covers zero-, one-, two-, and three-argument grammar, string/integer/file primaries, XSI-obsolescent logic/grouping, the `[` closing token, diagnostics, statuses, and stdin preservation. | The full filesystem-primary/platform product and locale-sensitive diagnostics remain. |
| `umask` | `TestUmaskIssue7Interface` covers octal and symbolic operands/output, both required output round trips, current-shell mutation, invalid operands/options, and stdin preservation. | Child-process inheritance and translated diagnostics remain. The four-digit default display is explicitly Bash's allowed choice; only reusability is normative. |

The accepted shell commits are `0784fc70` (evidence) and `d758a1c4`
(review corrections plus Bash-compatible `read` status), published on the
`sh` integration branch.
