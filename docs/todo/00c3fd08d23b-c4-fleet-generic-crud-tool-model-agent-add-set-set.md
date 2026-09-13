---
id: 00c3fd08d23b
kind: task
title: 'C4 fleet generic CRUD: tool|model|agent add/set --set path=value, --unset, schema, show --field'
seq: 113
status: done
priority: p1
created: 2026-09-12T23:03:30.136601Z
weave: 8
assignee: qiangli
sprint: 161
closed: 2026-09-13T00:15:43.789126Z
resolution: fixed
closed_by: claude-h
---

Fleet generic field CRUD — the rod. In pkg/fleet/cli_write.go:
- --set <dotted.path>=<value> (repeatable) and --unset <dotted.path> on tool|model|agent set, AND on add, so `add <name> --set kind=cli --set cli.launch.exec=...` creates an entry from an empty record with only the mechanism's own defaults (model source=cloud, ring=local); add <file>|- stays.
- Implementation: canonical Marshal -> edit the yaml.v3 Node tree by path (create intermediate maps; lists by index or name= for cli.versions / x_hosts) -> strict re-decode (KnownFields(true)) so an unknown path is refused with the valid-path list for that noun -> existing Save* validation (band range, claimName, canonical model binding). Scalars parsed by target type (bool/int/float/string; list literal [a,b]).
- Discovery: tool|model|agent schema [--json] prints every settable path with type and one-line meaning, generated from the struct tags in types.go and test-ratcheted so a new field cannot be added without appearing; the refusal prints the same table.
- Typed flags ONLY where the mechanism owns semantics: model --band --band-source --id <tool>=<upstream>; tool --hidden; agent --ephemeral. Existing typed flags stay for compatibility; add no others.
- Read side: show <name> --field <dotted.path> on all three nouns (plain scalar, or YAML fragment for a subtree; --json wraps).
Tests: create-from-flags per noun; set/unset every top-level and cli.launch.* path; --set band=9 refused; unknown-path message equals schema output; show --field scalar and subtree; Windows path-neutral.
Gate: go test ./pkg/fleet/...
