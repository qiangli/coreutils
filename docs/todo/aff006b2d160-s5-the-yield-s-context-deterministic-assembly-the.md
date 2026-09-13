---
id: aff006b2d160
kind: task
title: 'S5 the yield''s context: deterministic assembly, the fact/fold split, and the skill add scrub'
seq: 127
status: todo
priority: p1
created: 2026-09-13T14:36:13.526989Z
sprint: 166
---

SPRINT: #166 (epic yoke). SPEC: docs/bashy-agentic-action-design.md (S0). DEPENDS ON: S4 (pkg/bscript lowering to the kind: skill record).
GATE: (a) the yield's skill part passes redact.Clean - paths stay SYMBOLIC (~, $HOME), never resolved; (b) the context part carries the resolved entities (absolute paths, host/user/home findings, kb hits, craft facts, spacegraph relations) to the LOCAL caller and is never written anywhere by bashy; (c) no secret VALUE appears anywhere in the envelope (leak-vector ratchet: tests assert strings.Contains and never print the value); (d) the embedded kb context envelope is the frozen context_version 1 golden, unchanged; (e) the whole envelope is under the 40 KiB ceiling or spilled through pkg/reduce with the inline recovery command; (f) bashy skill add of a record whose SKILL.md names a hostname is REFUSED; (g) determinism: same input, same env and cwd, twice, gives a byte-identical envelope minus result.

WHAT: coreutils/pkg/bscript/context.go - deterministic context assembly, no model: (1) tokens from sh/syntax (argv words, $VAR names, ~ and paths; values of credential-classified vars never read); (2) craft.Extract(argv) ONLY for schema'd commands (SpecFor: ssh/scp/rsync/psql/git/podman/kubectl/...; for ls -la ~/x it returns false and the facts step is skipped - never For() on a zero entity); (3) lexicon/define classification per token (classifies a credential without echoing it); (4) redact.FromHost() findings as the entity list; (5) recall.Context(Query{Text, Rings: repo,host,agent, Budget}) embedded verbatim (cheap, lazy, no watcher - verified); (6) craft.OpenFacts(...).For(entity) and spacegraph relations, read-only. Fact/fold split is the whole design: skill = fold (shareable after scrub), context = fact (host-local, returned, never stored) - skill-graph-design invariants, facts have no export path.

THE GAP THIS CLOSES: skills/cli.go runAdd (909-943) runs NO identity scrub on add <file.yaml>|- today; hostScrubber() runs only in show and export. Apply the same scrub on the record path of add, refusing with ErrCarriesIdentity and naming the offending path.

NON-SCOPE: the lowering itself (S4); any model; any new store.
