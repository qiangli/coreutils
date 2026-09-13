---
id: a5f5cfc81287
kind: task
title: 'C2 links become real: [[slug]]/[[kb:]]/[[todo:]]/[[sprint:]] parsed at read time in kb AND todo bodies; kb backlinks; kb doctor (flag, never fix); todo show --links'
seq: 118
status: todo
priority: p0
created: 2026-09-13T01:31:31.63346Z
sprint: 163
---

Goal: links become real — the documented "link graph is vacuous" defect closes without a new field or store.

- Parser: body links of the forms [[slug]], [[kb:slug]], [[todo:<id>]], [[sprint:<n>]] and relative markdown links (pages/x.md, ../todo/y.md) are resolved AT READ TIME (resolve-at-consumption; prose stays a cache; never rewrite a body). Applies to kb records AND todo/issue bodies (pkg/issue record unchanged; the resolver lives in pkg/kb and pkg/todo calls it read-only).
- kb backlinks <slug> [--json]: every record (kb or todo) whose body links to it.
- kb doctor [--ring ...] [--json]: dangling links, orphan pages (no inbound, no outbound, not validated), near-duplicate pairs (reuse NearDuplicate), missing form/description, open-vocab relations (after C5). Flags only. kb doctor must never become kb fix.
- bashy todo show <id> --links renders outbound + inbound through the same resolver.
- kb show records the OPEN exactly as today; backlinks/doctor record nothing.

Files: coreutils/pkg/kb/links.go (new), doctor.go (new), kb.go (subcommands), coreutils/pkg/todo (show --links), tests.
Gate: go test ./pkg/kb/... ./pkg/todo/...; a body with a dangling [[kb:x]] is reported by doctor and left byte-identical; backlinks resolves a todo -> kb cite; TestSearchJSONIsTokenLean still passes (doctor output is separate from search).
Depends on: C1 (form field so notes exist to link).
