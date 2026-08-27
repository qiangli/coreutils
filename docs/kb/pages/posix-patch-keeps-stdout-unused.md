---
type: lesson
title: POSIX patch keeps stdout unused
description: When auditing the POSIX Issue 7 patch utility, route filename prompts, progress, and diagnostics to stderr because standard output is specified as not used; check short writes on both diagnostic streams and output files so failures produce nonzero status.
status: candidate
source:
    tool: codex-gpt5.6-sol-f
    host: dragon
    episode: weave-issue-6
created: "2026-08-27T09:10:10Z"
---
