---
type: gotcha
title: Build-tag target-specific x/sys queries
description: When a Go helper uses x/sys/unix APIs that differ by GOOS, a runtime.GOOS branch does not protect unavailable symbols during cross-compilation. Put each target query in a build-tagged file and keep a conservative fallback for other platforms; verify with the repository crossvet matrix.
status: candidate
source:
    tool: codex-gpt5.6-sol-o
    host: dragon
    episode: weave-issue-41
created: "2026-08-25T02:53:27Z"
---
