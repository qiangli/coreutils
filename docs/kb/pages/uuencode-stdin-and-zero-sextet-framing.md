---
type: lesson
title: uuencode stdin and zero sextet framing
description: For POSIX uuencode, the header records the input file's access bits and the input '-' operand is a pathname; uudecode treats header '-' and /dev/stdout as standard output, but -o '-' as a pathname. Stage decoded data before overwriting an existing regular file so an input can also name its output.
status: validated
evidence: Validated by focused vectors, same-input/output regression coverage, race tests, and Linux/Darwin/Windows compile checks in Corrective Sprint 79 independent review.
source:
    tool: codex-gpt5.6-terra-e
    host: dragon
    episode: weave-issue-57
created: "2026-08-25T02:42:08Z"
updated: "2026-08-25T03:44:03Z"
---

POSIX specifies the access permission bits of the input file in the header; it
does not prescribe a universal `0644` mode when the input operand is omitted.
The command therefore uses `fstat` when standard input is an `*os.File`. The
library-only extension accepting an abstract `io.Reader` has no file metadata,
so it documents and uses `0666` as its maximum-access fallback.

The `-` input operand to `uuencode` is an ordinary pathname. For `uudecode`, a
header pathname of `-` or `/dev/stdout` selects standard output, while an
explicit `-o -` override is an ordinary pathname; only `-o /dev/stdout` is
special.

Classic zero sextets and the terminating zero-length line are emitted as SPACE;
the decoder also accepts the historical backtick extension. Decoded modes are
limited to the nine rwx access bits, and a failed post-write chmod is diagnosed
without discarding successfully decoded content.

When overwriting an existing regular file, decode to staging storage before
truncating the held output descriptor. This preserves the existing inode and
hard links without requiring write access to the parent directory, keeps
malformed input from destroying valid content, and permits an encoded input
file to name itself as its decoded output.
