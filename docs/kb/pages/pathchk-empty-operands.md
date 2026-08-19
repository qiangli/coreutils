---
type: lesson
title: pathchk treats an empty string as an invalid operand
description: Quote empty-operand probes explicitly; GNU Coreutils 9.11 and this implementation reject the empty string in default, -p, and -P modes.
tags:
    - pathchk
    - posix
    - conformance
    - differential-testing
scope:
    repos:
        - coreutils
status: validated
evidence: GNU Coreutils 9.11 differential on Dragon; TestPathchkEmptyPathnameOptions; implementation commit 6f98ca29
source:
    tool: codex-gpt5.6-luna-q
    host: dragon
    episode: weave-issue-641
created: "2026-08-13T13:11:25Z"
updated: "2026-08-19T20:23:36Z"
---

An empty string is an operand, not the same case as supplying no operands.
GNU Coreutils 9.11 returns failure for each of `pathchk ''`, `pathchk -p ''`,
and `pathchk -P ''`. The default mode reports the empty pathname as a missing
file, while `-p` and `-P` report an empty file name. Exact diagnostic wording is
not a POSIX portability guarantee; the stable differential contract here is
that all three invocations fail and write a diagnostic.

Always quote the empty argument in a probe. An unquoted empty variable can
disappear during shell expansion and accidentally exercise the separate
missing-operand usage error instead. Keep all three modes in the regression
table because `-p` and `-P` take different validation paths from the default.
