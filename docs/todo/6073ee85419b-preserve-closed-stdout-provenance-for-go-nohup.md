---
id: 6073ee85419b
kind: bug
title: Preserve closed stdout provenance for Go nohup
seq: 181
status: done
priority: p0
created: 2026-10-02T04:16:49.799695Z
assignee: codex-gpt6.1-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-02T05:22:07.907916Z
closed_by: codex-gpt6.1-sol
---

Frozen D/C nohup:21 invokes nohup with stderr on a terminal, stdout closed, and both nohup.out paths unavailable; expects 127 without invoking utility. Exact Linux public reducer returns 0. Go runtime checkfds reopens a closed fd 1 to /dev/null before multicall receives os.Stdout, so isClosedFile(os.Stdout) cannot recover pre-runtime state. Design a robust pure-Go-compatible pre-runtime or shell-to-applet provenance path that distinguishes explicitly open /dev/null; test real process descriptors and rerun exact TP21. Cross-ref Sprint354 6a0e07ba82d9 and Sprint355 a481a106ffd0.
