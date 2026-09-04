---
id: 69e10a3afc7e
kind: task
title: 'GATED: seam 1 — bound chat.Invoke output (every agentic turn)'
seq: 45
status: todo
priority: p2
created: 2026-09-04T09:52:41.239162Z
sprint: 123
---

CLOSED until the Yoke gates open. pkg/chat/chat.go:1106 'res.Output, res.ExitCode = out, code' — every invoke/chat/supervise/foreman/delegate/meet turn funnels through this one line, uncapped, on both the pipe (chat.go:454-471) and PTY (chat.go:362-380) paths. Highest leverage seam, and agent-read by definition so no human format contract is at risk. Streaming variant models on Coach, which is already a chained io.Writer in the same MultiWriter.
