---
id: 69e10a3afc7e
kind: task
title: 'GATED: seam 1 — bound chat.Invoke output (every agentic turn)'
seq: 45
status: done
priority: p1
created: 2026-09-04T09:52:41.239162Z
weave: 13
assignee: qiangli
sprint: 123
closed: 2026-09-05T23:59:00Z
---

OPEN under Sprint 123, which superseded the original Yoke gate. `chat.Invoke`
now reduces every completed unattended turn at its shared result seam. The
complete output is content-addressed under the existing `~/.bashy/chat` state
root before an over-budget view is returned, and that view carries the reducer's
`bashy out <handle>` recovery marker. This covers pipe and PTY runner results
equally; model-visible Stream and EventStream views are buffered until their
redacted bounded artifacts are available.
