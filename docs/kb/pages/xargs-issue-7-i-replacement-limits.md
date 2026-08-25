---
type: lesson
title: xargs Issue 7 -I replacement limits
description: When implementing POSIX Issue 7 xargs -I, enforce that the replacement string appears in no more than five command arguments and that each resulting replacement-bearing argument is at most 255 bytes; -L trailing-blank continuation is a separate rule. Regenerate docs/applet-matrix after adding tests, then run scripts/crossvet.sh.
status: validated
evidence: cmds/xargs/xargs_test.go TestXargsReplaceIssue7Limits and TestXargsReplaceAppliesOnlyToArgumentOperands; crossvet passes windows/linux/darwin/aix
source:
    tool: codex-gpt5.6-luna-o
    host: dragon
    episode: weave-issue-41
created: "2026-08-25T03:13:14Z"
updated: "2026-08-25T03:21:49Z"
---
