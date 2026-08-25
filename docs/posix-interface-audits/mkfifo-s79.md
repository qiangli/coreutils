# `mkfifo` POSIX.1-2008 Issue 7 Audit

Scope: `mkfifo [-m mode] file...` against The Open Group POSIX.1-2008
Issue 7, 2016 Edition utility page:
<https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkfifo.html>.

GNU compatibility is out of scope; only the Issue 7 interface is certified
here. The `-Z/--context` short and long forms are a pre-existing deterministic
no-op on non-SELinux platforms and are not part of the POSIX surface.

## 1. Options and Operands

The single required `-m mode` option is implemented. `file` operands are
processed independently in argument order: a failure — including an existing
entry — draws a `cannot create fifo` diagnostic, records final status `1`, and
processing continues with the remaining operands. Zero operands is a
missing-operand diagnostic at exit `1`.

`--` ends option parsing, so a following dash-prefixed token is an ordinary
pathname even when it is spelled like an option (`mkfifo -- -m` creates a FIFO
literally named `-m` rather than being read as the mode option). A bare `-`
operand has no standard-input meaning for `mkfifo` and names an ordinary FIFO
called `-`. No other token is special.

Evidence:
`TestMkfifoCreatesFIFO`, `TestMkfifoMultipleOperands`,
`TestMkfifoPartialFailureContinues`, `TestMkfifoDashOperandIsPathname`,
`TestMkfifoDoubleDashEndsOptions`, `TestMkfifoErrors`.

## 2. Standard Input, Output, and Environment

POSIX specifies that the standard input is not used. `mkfifo` creates its FIFOs
without reading a single byte of standard input; a populated input reader is
left fully unread across a successful run. Nothing is written to the standard
output on success; the standard error carries only diagnostics.

Evidence:
`TestMkfifoDoesNotConsumeStdin`, `TestMkfifoCreatesFIFO`.

## 3. Mode Grammar and the Creation Mask

`-m` accepts the full chmod-grammar mode value: octal modes from `0` through
`7777` (including set-user-ID, set-group-ID, and sticky bits) and symbolic
clauses with `who` lists, the `+`, `-`, and `=` operators, `rwxXst`
permissions, and single `u/g/o` permission copies. Non-octal numeric strings
and malformed symbolic strings are rejected with an `invalid mode` diagnostic
at exit `1` rather than being approximated. Symbolic `+`/`-` are interpreted
relative to the assumed initial mode `a=rw`, and clauses that omit `who` leave
umask-selected bits unchanged, matching the chmod rule.

Without `-m`, the FIFO is created with `a=rw` (`0666`) modified by the file mode
creation mask: for a standalone Unix process the kernel applies the process
umask through `mkfifo(2)`; for an embedded invocation the embedding shell's
virtual umask (`RunContext.UmaskSet`) is honored instead, without mutating the
process-global umask.

**Creation algorithm and the "less restrictive" rule.** The Issue 7 requirement
is that the created FIFO must never be *less restrictive* (more permissive) than
the requested `-m` value. This implementation preserves the established
creation algorithm: the FIFO is first created with the requested mode already
reduced by the applicable creation mask, and a follow-up `chmod` then *widens*
those bits up to the exact `-m` value. Because the mask can only clear bits, the
intermediate on-disk entry is always at least as restrictive as the final `-m`
target — at no instant is it more permissive than requested — so there is no
window in which the FIFO is exposed more broadly, and the creation mask cannot
leak into the final permission bits. The process-global umask is therefore
neither read nor neutralized on an embedded `-m` call, and no additional syscall
seam is introduced: correctness follows from the create-restricted-then-widen
ordering alone.

Evidence:
`TestMkfifoMode`, `TestMkfifoOctalSpecialBits`, `TestMkfifoSymbolicMode`,
`TestMkfifoSymbolicModeHonorsOmittedWhoUmask`,
`TestMkfifoDefaultModeHonorsVirtualUmask`,
`TestMkfifoDefaultModeHonorsProcessUmask`,
`TestMkfifoSymbolicModeHonorsProcessUmask`, `TestMkfifoErrors`.

## 4. Exit Status and Platform

Exit status is `0` when every requested FIFO was created, and greater than `0`
otherwise: invalid `-m` and missing operand exit `1`; kernel creation and
permission failures exit `1`; unknown options exit `2` per the documented repo
usage-error deviation.

Windows has no POSIX FIFO or mode bits, so every invocation fails loudly per
operand with a clear unsupported error rather than approximating named-pipe or
permission semantics.

Evidence:
`TestMkfifoErrors`, `TestMkfifoPartialFailureContinues`,
`TestMkfifoCreatesFIFO`, `TestMkfifoContextNoop`, `TestMkfifoHelpAndVersion`.

## 5. Residuals

The `evidence_state` remains `partial`. Residuals that keep it partial:

* Diagnostics are fixed English strings; the `LC_MESSAGES` and `NLSPATH`
  message-catalog interfaces are not implemented.
* Windows refuses FIFO creation and `-m` because POSIX FIFO and mode bits are
  unavailable there; this is a platform residual, not an approximation.
* `-Z/--context` is a pre-existing deterministic no-op on non-SELinux
  platforms and is a GNU-fidelity residual outside the Issue 7 surface, not
  POSIX evidence.

## 6. Gate Record

Focused local gate for this issue, run on 2026-08-25:

```sh
POSIXLY_CORRECT=1 go test -count=20 ./cmds/mkfifo
go test -count=20 ./cmds/mkfifo
go test -race -count=5 ./cmds/mkfifo
go vet ./cmds/mkfifo
GOOS=linux   go vet ./cmds/mkfifo
GOOS=darwin  go vet ./cmds/mkfifo
GOOS=windows go vet ./cmds/mkfifo
python3 scripts/posix_manifest.py --check   # MD regenerated from the TSV
```

Results are recorded in the run log. The `posix_manifest.py --check` gate
still reports the pre-existing, unrelated `sh: partial state requires focused
semantic evidence` failure that is present on the branch independent of this
change; the mkfifo section of the rendered interface document is regenerated
from the TSV and is consistent with it.
