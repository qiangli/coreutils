---
type: lesson
title: agent write reachability needs a real pty line
description: When registering bashy agent sessions for who/write reachability, publish a bashy-owned pty directory entry that points at the real PTY slave and record that bare line in pkg/who. Keep write/mesg POSIX-inert by selecting the agent registry only when cmds/internal/session sees a bashy agent shell; POSIXLY_CORRECT must retain native utmp and controlling-terminal behavior.
status: candidate
source:
    tool: codex-gpt-5.5-u
    host: dragon
    episode: weave-issue-21
created: "2026-08-26T09:11:55Z"
---
