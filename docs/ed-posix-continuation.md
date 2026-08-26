# Pure-Go `ed`: implemented slice and continuation ledger

`cmds/ed` is a clean-room implementation from the POSIX.1 Issue 7 `ed`
utility description. It does not contain or translate GNU ed source and it
never invokes a host editor or shell. The reusable state machine is in
`cmds/internal/editor`; the command package is only the Tool/RunContext and
filesystem adapter.

This is a useful POSIX subset, not a full-conformance claim. The implemented
surface is:

- script-mode command and input streams, `-p`, `-s`, and one optional file;
- numeric, `.`, `$`, forward `/BRE/` and backward `?BRE?` addresses, offsets,
  ranges, comma/semicolon current-line behavior, and wraparound search;
- `a`, `i`, `c`, `d`, `p`, `n`, `l`, and `=` buffer operations;
- `e`, `E`, `f`, `w`, `q`, and `Q` file/modified-buffer state;
- POSIX BRE search and `s` with `&`, `\1`…`\9`, occurrence counts, `g`, and
  `p`/`n`/`l` output flags; and
- POSIX-style `?` command diagnostics, `H` help mode, prompts, byte counts,
  and non-zero status after a diagnostic.

The following work remains before complete POSIX `ed` can be claimed:

- marks and `k`; `j`, `m`, `t`, `u`, `r`, `P`, and command output suffixes;
- `g`, `G`, `v`, and `V`, including command-list parsing and atomic undo;
- multiline replacement, remembered replacement edge cases, and every
  specified null/omitted delimiter interaction;
- shell escape and the `!` forms of `e`, `r`, and `w`. These require an
  explicitly approved command-execution boundary; this applet will not shell
  out implicitly;
- signal/terminal-disconnect recovery, locale-sensitive BRE collation and
  diagnostics, exact list folding, incomplete-final-line handling, and the
  remaining error/current-line corner cases; and
- full Profile C/D and cross-platform behavioral certification evidence.

`ed` has no external-provider definition, build recipe, cache entry, or
fallback. The pure-Go applet is its sole shipped owner.

Normative reference: [POSIX.1 Issue 7 `ed`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ed.html).
