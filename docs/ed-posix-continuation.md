# Pure-Go `ed`: implemented slice and continuation ledger

`cmds/ed` is a clean-room implementation from the POSIX.1 Issue 7 `ed`
utility description. It does not contain or translate GNU ed source. The
reusable state machine is in `cmds/internal/editor`; the command package owns
the Tool/RunContext, filesystem adapter, signal handling, and the standard's
explicit `!` command-interpreter boundary.

The source-level Issue 7 surface is implemented, including:

- script-mode command and input streams, `-p`, `-s`, and one optional file;
- numeric, `.`, `$`, forward `/BRE/` and backward `?BRE?` addresses, offsets,
  ranges, comma/semicolon current-line behavior, and wraparound search;
- all required addresses, marks, ranges, global selections, and buffer
  operations;
- `e`, `E`, `f`, `r`, `w`, `q`, and `Q` file/modified-buffer state;
- POSIX BRE search and `s` with `&`, `\1`…`\9`, occurrence counts, `g`, and
  `p`/`n`/`l` output flags; and
- POSIX-style `?` command diagnostics, help/prompt modes, byte counts, signal
  recovery, checked output/file writes, and non-zero status after a diagnostic.

The detailed executable clause checklist is
[`cmds/ed/POSIX-issue7-completion.md`](../cmds/ed/POSIX-issue7-completion.md).
The manifest remains partial until the independent Profile C/D integration
replay supplies certification evidence; this source ledger is not itself a
certification claim.

`ed` has no external-provider definition, build recipe, cache entry, or
fallback. The pure-Go applet is its sole shipped owner.

Normative reference: [POSIX.1 Issue 7 `ed`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ed.html).
