---
type: lesson
title: uuencode stdin and zero sextet framing
description: For POSIX uuencode, omitted input uses a 0644 header mode and '-' is a pathname; classic zero sextets and the terminating zero-length line use SPACE, while uudecode may accept backtick. Preserve exact decoded modes with a post-write chmod instead of changing process umask.
status: validated
evidence: Validated by focused vectors, race tests, and Linux/Darwin/Windows/AIX vet in Corrective Sprint 79.
source:
    tool: codex-gpt5.6-terra-e
    host: dragon
    episode: weave-issue-57
created: "2026-08-25T02:42:08Z"
updated: "2026-08-25T02:42:13Z"
---
