---
id: 28434dd75e6f
kind: task
title: 'C6 pkg/lexicon action facet: one projection over command/script/agent/skill + atlas.ProjectEffects'
seq: 115
status: done
priority: p1
created: 2026-09-12T23:03:30.186539Z
weave: 9
assignee: qiangli
sprint: 161
closed: 2026-09-13T00:15:43.814392Z
resolution: fixed
closed_by: claude-i
---

The action facet — the unification as code, projection only.
pkg/lexicon: ActionFacet struct + Concept.Action *ActionFacet (yaml/json tag action,omitempty — a NESTED object, never a top-level or string-valued key). Fill it in the existing per-kind resolvers:
- command (pkg/atlas Entry): kind=command, contract=none, latitude=exact, authority=deterministic, atlas_effects verbatim, effects_declared via a new pure atlas.ProjectEffects([]string) (dhnt6, unprojected) — read->read write->write destroy->destroy net|remote->net spend->spend, exec/cred/priv/persist/pure carried unprojected; executor builtin|coreutils|path|verb; envelope bashy-run-v1.
- skill (pkg/skills DhntInfo): identity, capability, contract dhnt|metadata-checks|none, effects_declared=EffectCap, latitude=worst step, authority=agentic iff any judge step, executor dhnt, envelope attest-jsonl.
- agent (pkg/fleet): identity = tool:model MatrixKey, scope=host for the name claim, latitude=judge, authority=agentic, executor agentlaunch:<tool>, envelope bashy-chat-v1.
- dag target (dag.md in cwd via pkg/dag parser): kind=dag-target (script family), contract=dag, effects_declared=Effects:, latitude=exact, executor dag, envelope dag-v1.
- script files/functions: identity "" honestly (the script-execution plan owns minting one).
Rules: rebuilt per call, no persistence; Location/Host never emitted (existing lexicon ratchet); a TOOL (harness) resolves as a concept but carries NO facet — it is the executor. The three effect concepts (declared / predicted / observed) never share a field.
Tests: every family yields a facet with the documented values (embedded skills, baseline tools, bashy/dag.md); ProjectEffects table-driven; tool concept has Action == nil; JSON has no top-level "action" key.
Yoke note: pkg/lexicon is not in the gate-3 enumerated class; the spec records this as ordinary define surface work — if the operator disagrees, the facet moves into bashy/internal/agentos over the same package APIs.
Gate: go test ./pkg/lexicon/... ./pkg/atlas/...
