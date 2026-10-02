---
id: 6a37ee47cafa
kind: bug
title: 'Profile D patch TP33: emit rejected context hunk without filename headers'
seq: 180
status: done
priority: p0
created: 2026-10-02T01:30:56.215887Z
assignee: codex-gpt6.1-sol
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
closed: 2026-10-02T02:26:29.704226Z
closed_by: codex-gpt6.1-sol
---

Frozen D and C patch:33 were UNRESOLVED before patch ran because Go tail rejected historical +3; repair1 replay now reaches patch and FAILs. Profile B GNU provider also FAILs. Suite-free reproduction with the same context diff shows both Go patch and modern GNU patch prepend two filename header lines to the .rej file, while the suite compares only the rejected hunk after tail +3. The Open Group Issue 7 patch description says failed hunks are appended in context-difference format. Repair Go command output narrowly, retain the frozen codes, and require focused exact TP33 PASS on a pushed pinned D candidate.
