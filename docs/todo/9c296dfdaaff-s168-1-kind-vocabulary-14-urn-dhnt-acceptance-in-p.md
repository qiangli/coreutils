---
id: 9c296dfdaaff
kind: task
title: 'S168.1 kind vocabulary (14) + urn:dhnt: acceptance in pkg/kb/links.go; LinkUnknown never dropped; ratchet test'
seq: 129
status: done
priority: p1
created: 2026-09-13T22:56:21.66083Z
sprint: 168
closed: 2026-09-14T02:07:54.781848Z
---

S168.1 (W0, seam owner). Plan: dhnt docs/sprint-168-master-execution-plan.md, D1/D2/D3, trap 1+5.
Build NEW stdlib-only leaf pkg/ref: Kind; Kinds() = 15 (kb todo sprint run meet mb bus agent person host tool model skill episode role), ratcheted table test; Parse(s) (Ref, bool) accepting <kind>:<id>, urn:dhnt:<kind>:<id>, and legacy dhnt:<kind>/<name>[@owner] (input only, owner -> Ref.Qualifier); a non-vocabulary prefix (codex:gpt5.6-sol tool:model binding) is NOT a ref; Node{Kind,ID,Title,Status,Where,Open}; Resolver interface; Format/URN helpers.
pkg/kb/links.go consumes it: LinkKind = ref.Kind alias, LinkKB/LinkTodo/LinkSprint kept; ParseLinks yields every kind; [[urn:dhnt:kb:x]] == [[kb:x]]; unknown scheme -> LinkUnknown (never dropped, never a kb slug).
Three ratchets: vocabulary table; ParseLinks per kind; leaf no-cycle test (deps of pkg/kb must not include todo weave chat recall search sota foreman execlog lexicon).
Proof: go test ./pkg/ref ./pkg/kb; kb doctor --json before/after on a real kb shows unknown-scheme rows MOVED (same count), paste the count in the commit body.
