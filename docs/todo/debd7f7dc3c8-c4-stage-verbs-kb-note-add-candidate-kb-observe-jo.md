---
id: debd7f7dc3c8
kind: task
title: 'C4 stage verbs: kb note add --candidate, kb observe (journal-only), kb validate --from-gate (only runtime path to validated); redaction gate before shareable-ring writes'
seq: 120
status: todo
priority: p1
created: 2026-09-13T01:31:31.691999Z
sprint: 163
---

Goal: the stage verbs a harness wires at Observe / Verify / Persist, so "instructed is not structural" stops being true for the conductor and for hooks.

- kb note add --candidate [--ring agent|repo|host] [--episode <id>] --title ... [--description ...] --body|-f: writes a form: note record with status candidate; reconcile-on-write (NearDuplicate) refuses near-dups unless --force. Runtime writes are ALWAYS candidate.
- kb observe --episode <id> --kind <tool-result|gate|note|...> --ref <payload-ref> [--summary ...]: appends an observation EVENT to journal.jsonl (a stream), never a record. execlog already records commands; this is the door for non-command observations.
- kb validate <slug> --from-gate <gate-run-id>: promotes candidate -> validated ONLY when a gate/judge record that actually ran is found (pkg/gate / bashy judge records); manual --evidence stays for humans. "A span may never promote."
- Redaction gate before any write to a shareable ring (repo/host): coreutils/pkg/redact host rules + the secrets values redactor; a note naming a host is refused from repo/host rings (it is a fact wearing a fold's clothes) and is allowed only in the agent ring.

Files: coreutils/pkg/kb/{kb.go,observe.go,validate.go}, tests.
Gate: go test ./pkg/kb/...; validate --from-gate with no gate record exits non-zero and writes nothing; observe writes one journal line and no page; note add on a near-dup title exits non-zero with the existing slug in the message; a body containing a hostname is refused for --ring repo and accepted for --ring agent.
Depends on: C1.
