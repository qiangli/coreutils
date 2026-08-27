---
type: gotcha
title: Embedded applets need invocation argv0 for POSIX-visible ARGV
description: When a POSIX applet exposes its invocation name to the program language or output, standalone multicall dispatch must pass os.Args[0] through RunContext instead of letting embedded command registration names leak into user-visible ARGV[0]. This closed the Profile D shell sh_07:TP34 awk diagnostic where /vsc/cushim/awk was reported as awk.
status: candidate
source:
    tool: codex-gpt-5.5-s
    host: dragon
    episode: weave-issue-19
created: "2026-08-27T16:58:42Z"
---
