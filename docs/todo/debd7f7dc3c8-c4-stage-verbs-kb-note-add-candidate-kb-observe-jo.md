---
id: debd7f7dc3c8
kind: task
title: 'C4 stage verbs: kb note add --candidate, kb observe (journal-only), kb validate --from-gate (only runtime path to validated); redaction gate before shareable-ring writes'
seq: 120
status: assigned
priority: p1
created: 2026-09-13T01:31:31.691999Z
weave: 16
assignee: qiangli
sprint: 163
---

Goal: the stage verbs a harness wires at Observe / Verify / Persist, so "instructed is not structural" stops being true for the conductor and for hooks. Design of record: dhnt docs/kb-rings-forms-stages.md §5, §6; plan D2.

- kb note add --candidate [--ring agent|repo|host] [--episode <id>] --title ... [--description ...] --body|-f: writes a form: note record with status candidate; reconcile-on-write (NearDuplicate) refuses near-dups unless --force. Runtime writes are ALWAYS candidate.
- kb observe --episode <id> --kind <tool-result|gate|note|...> --ref <payload-ref> [--summary ...] [--json]: appends an observation EVENT to journal.jsonl (a stream), never a record, and PRINTS THE EVENT ID (deterministic: sha1 over kind\0episode\0ref\0at, [:16]). execlog already records commands; this is the door for non-command observations.
- kind=gate carries the honest fields of gate.Outcome as flags/JSON: --ran, --passed, --command, --exit-code, --where (plus episode). This event IS the gate reference (plan D2): nothing in the tree persists an id-addressable gate run (gate.Outcome is in-memory; the capability ledger RunRecord has GatePass but no id; weave's observation has GateExit by issue). Writers land in C7 (weave runDrainGate) and B2 (conductor RETRO); pkg/gate never imports kb and kb never imports gate.
- kb validate <slug> --from-gate <observe-event-id>: promotes candidate -> validated ONLY when journal.jsonl holds an event with that id, kind=gate, ran=true and passed=true; otherwise exits non-zero and writes nothing, naming what was missing (no such event / not a gate / did not run / did not pass). Manual --evidence stays for humans. "A span may never promote."
- Redaction gate before any write to a shareable ring (repo/host): coreutils/pkg/redact host rules + the secrets values redactor; a note naming a host is refused from repo/host rings (it is a fact wearing a fold's clothes) and is allowed only in the agent ring.

Files: coreutils/pkg/kb/{note.go,observe.go,validate.go} (new; kb.go gains only the registrations), tests.
Gate: go test ./pkg/kb/...; validate --from-gate with no such event exits non-zero and writes nothing; --from-gate on a gate event with passed=false exits non-zero; observe writes one journal line, no page, and prints an id that is stable across two identical calls; note add on a near-dup title exits non-zero with the existing slug in the message; a body containing a hostname is refused for --ring repo and accepted for --ring agent.
Traps: pkg/kb stays a leaf (scope, bus, git only — redact is allowed only if leaf_test permits it; otherwise the redaction check is a small pure function in kb that the mount can override); tests set BASHY_KB_DIR / BASHY_HOME / YCODE_DATA_DIR to scratch.
Depends on: C1.
