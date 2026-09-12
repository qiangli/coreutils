---
id: 242625fd5369
kind: task
title: C2 skill show --yaml|--json, add <file.yaml>|-, export --yaml, sync consumes a record
seq: 111
status: todo
priority: p0
created: 2026-09-12T23:03:30.086102Z
sprint: 161
---

Skill record verbs over C1:
- skill show <name> --yaml|--json emits the record; the flagless default stays byte-identical SKILL.md.
- skill add <file.yaml>|- imports a record through the SAME admission gate as add <dir> (install.go), then WriteFolder into the local store — mirror tool add <file>|- (pkg/fleet/cli_write.go:225).
- skill export <name> --yaml writes the record instead of the folder.
- sync.go: when a pulled Content parses as a record, write the full folder; plain SKILL.md content keeps today's path (backward compatible with the current cloudbox).
Tests: cli round-trip (show --yaml | add -); sync consumes both content shapes.
Gate: go test ./pkg/skills/...
