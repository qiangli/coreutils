---
id: f86c87357945
kind: task
title: 'C1 pkg/skills: kind: skill Record — lossless folder↔YAML projection, deterministic emit, redact at pack'
seq: 110
status: done
priority: p0
created: 2026-09-12T23:03:30.059846Z
weave: 7
assignee: qiangli
sprint: 161
closed: 2026-09-13T00:15:43.763252Z
resolution: fixed
closed_by: claude-g
---

pkg/skills: the kind: skill record type — a lossless projection of a skill folder (SKILL.md stays the on-disk canonical; the record is for catalog/wire).
- Record struct: name, kind=skill, description, requires, identity (derived), capability (derived), contract{effects, ensure, steps} (a VIEW of the dhnt AST), bindings{check-*, step-*}, files{path: verbatim bytes}.
- RecordOf(Skill) (Record, error): walks the folder (sorted paths, files verbatim), derives identity/capability from the existing DhntInfo (dhnt.go:17-28, capability.go:80).
- MarshalRecord: deterministic yaml.v3 emit — fixed key order, no mtimes — so the cloudbox content_hash fast-path hits.
- ParseRecord([]byte) (Record, error): strict decode, refuse unknown top-level keys.
- WriteFolder(Record, dir): rebuilds the tree.
- Redaction at pack: run pkg/redact over files; refuse naming the offending path (same rule as craft folds).
Tests: folder -> record -> folder byte-identical for all 7 embedded skills and the umbrella overlay; YAML key re-order does not change identity; a record without skill.dhnt has identity "" and capability "" honestly; redaction refusal.
Gate: go test ./pkg/skills/...
