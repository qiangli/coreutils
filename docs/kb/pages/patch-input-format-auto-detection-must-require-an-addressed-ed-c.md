---
type: lesson
title: Patch input-format auto-detection must require an addressed ed command
description: When auto-detecting a patch input form, require an addressed ed command and let any ---/+++/***/@@ or normal-diff command line settle the format first — WHEN touching cmds/patch or pkg/patch detection, or auditing POSIX patch filename/format determination
status: candidate
evidence: 'pkg/patch/ExtractEdScript accepted an address-less a/c/d/i line, so any unified diff whose context contained a bare ''a'' line was applied as an ed script: nothing patched, ''File to patch:'' prompt, exit 2. diff -e always emits an address. Fixed 2026-08-28 on agent/weave-issue-34 with edScriptStartRE plus a looksLikeDiffListing guard; regressions in cmds/patch/profile_d_test.go. Same audit: POSIX -p counts a leading <slash> RUN as one component, and mailx msglist ''n-m'' is a range only when both sides are numeric (a hyphenated login is an address).'
source:
    tool: claude-opus5-h
    host: dragon
    episode: weave-issue-34
created: "2026-08-28T12:53:02Z"
---
