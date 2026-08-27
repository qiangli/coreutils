# POSIX.1-2017 `ed` completion checklist

Primary contract: [The Open Group `ed` utility, Issue 7 (2018 edition)](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/ed.html).

This checklist records the command-language clauses completed by the pure-Go
applet. It is intentionally scoped to POSIX; implementation-specific GNU/BSD
extensions are not certification requirements.

- [x] `-p string`, `-s`, and the optional file operand
- [x] simple, relative, mark, search, comma, semicolon, and omitted addresses
- [x] `a`, `c`, `d`, `i`, `j`, `k`, `m`, `t`, and one-command toggling `u`
- [x] `g`, `v`, `G`, and `V`, including non-nesting, multi-line command lists,
  and line-identity tracking when earlier commands move or delete marked lines
- [x] `p`, `l`, `n`, `=`, and applicable `p`/`l`/`n` command suffixes
- [x] `s`, remembered BRE/replacement state, occurrence counts, print flags,
  and escaped-newline replacements (including mark relocation)
- [x] `e`, `E`, `f`, `r`, `w`, `q`, `Q`, `h`, `H`, and `P`
- [x] bare `!` plus the `e`/`r`/`w` shell-command forms, previous-command
  recall, current-filename expansion, and quoting of special substitutions
- [x] terminal versus non-terminal command-error behavior
- [x] `SIGHUP` recovery to `ed.hup` and command-input interruption
- [x] POSIX list-mode escapes, LC_CTYPE character handling, end marker, and
  70-column folding
- [x] current-line, dirty-buffer warning, mark relocation, and undo state rules

Known non-POSIX extensions deliberately left out: encrypted files, restricted
mode, extended regular expressions, and GNU diagnostic verbosity controls.
The historical `W` append-write command is implemented as a compatibility
extension, but it is not claimed as part of the Issue 7 contract.
