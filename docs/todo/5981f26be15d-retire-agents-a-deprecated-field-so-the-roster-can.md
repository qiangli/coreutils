---
id: 5981f26be15d
kind: task
title: 'Retire agents: a deprecated field so the roster can shrink'
seq: 6
status: todo
priority: p2
created: 2026-08-19T16:54:20.357577Z
---

pkg/fleet: Agent has Ephemeral and ClonedFrom but nothing that says RETIRED. Adding a new agent per model version is the correct upgrade path (see fleet: add glm-5.3), but with no retirement the roster only grows: 39 agents today, three kimis, four gemini-flashes, and nearly every one 'unmeasured'. A selection surface listing four superseded siblings makes --min-band and meet rosters noisier every release.

WANT: 'deprecated: true' (plus optional 'superseded_by:') on Agent and Model.
- 'agents list' hides deprecated by default, shows with --all (the pattern Ephemeral already uses);
- selection (SeatByBand, meet --min-band, weave) skips them;
- resolution STILL works, so old records, transcripts and attestations naming a retired agent keep resolving — that is the whole reason to deprecate rather than delete;
- 'agents rm' stays for the local ring; deprecation is for the baseline, where deleting a row would break resolution of everything ever recorded against it.

Precedent for the semantics: aider was retired by DELETING its bindings, which is exactly the case that breaks old records.
