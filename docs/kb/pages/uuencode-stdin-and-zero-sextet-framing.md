---
type: lesson
title: uuencode stdin and zero sextet framing
description: For POSIX uuencode, the header records the input file's access bits and '-' is a pathname; classic zero sextets and the terminating zero-length line use SPACE, while uudecode may accept backtick. Preserve decoded access bits with a post-write chmod instead of changing process umask.
status: validated
evidence: Validated by focused vectors, race tests, and Linux/Darwin/Windows/AIX vet in Corrective Sprint 79.
source:
    tool: codex-gpt5.6-terra-e
    host: dragon
    episode: weave-issue-57
created: "2026-08-25T02:42:08Z"
updated: "2026-08-25T02:42:13Z"
---
POSIX specifies the access permission bits of the input file in the header; it
does not prescribe a universal `0644` mode when the input operand is omitted.
The command therefore uses `fstat` when standard input is an `*os.File`. The
library-only extension accepting an abstract `io.Reader` has no file metadata,
so it documents and uses `0666` as its maximum-access fallback.

The `-` input operand and decoded header/`-o` output name are ordinary
pathnames. Only the POSIX `/dev/stdout` output pathname is special.

Classic zero sextets and the terminating zero-length line are emitted as SPACE;
the decoder also accepts the historical backtick extension. Decoded modes are
limited to the nine rwx access bits, and a failed post-write chmod is diagnosed
without discarding successfully decoded content.
