---
id: 87eb79216d6a
kind: task
title: 'Sprint board: a sprint card does not link to its master execution plan'
seq: 92
status: todo
priority: p2
created: 2026-09-07T18:30:12.960018Z
sprint: 135
---

OBSERVED 2026-09-07. Every sprint has a plan document and the record already holds it: weaveStory.SpecRef is the sprint's spec/handoff doc reference, set by `sprint add --spec` and shown by `sprint show` as the "spec:" line. Sprint #130 carries docs/bashy-yoke-framework.md; sprint #117's plan is docs/bashpp-go127-master-execution-plan.md. The browser card shows none of it.

THE GAP IS TWO LAYERS DEEP, not one. board.Sprint (coreutils/pkg/board/model.go) carries ID, Title, Epic, Column, Continuity, Conductor, Manager, MeetRoomRef, LeaseStale, GateState, LeaseHolder, ContinuityRef, RunRefs, StoryRoots and the three story counts — and NOT SpecRef. So the field is dropped before the payload is built; board.js could not render it today even if it wanted to. Fix the model and the collector first, then the page.

WANTED.
(a) SpecRef travels: weaveStory -> board.Sprint -> the /api/sprint overview payload, as a json field on the sprint.
(b) The card renders it as a link to the plan, near the title where the reader is already looking, labelled so it reads as the plan and not as a generic attachment.
(c) A sprint with no spec renders nothing extra — an empty spec is an ordinary state (most sprints on this board have one; some do not) and must not produce a dead link or an empty row.

DECIDE AND RECORD: what the link RESOLVES TO. SpecRef is a repo-relative path ("docs/bashy-yoke-framework.md") and the browser is not standing in that repo — it may be reached on loopback or through outpost's /matrix/h/<host>/app/<name>/ prefix, which is exactly why board.js builds every URL from document.baseURI via url(). Options: serve the doc through an existing read-only route, or render the path as text the operator can copy. Do NOT invent a new file-serving surface for it, and do NOT emit a href that 404s on a proxied host — a link that is dead on remote and live on loopback is worse than a path.

READ-ONLY. A link out, nothing more.

GATE. verifydom over a fixture with one sprint carrying a spec and one without: assert the link renders with the right target for both loopback and a proxied base, and that the spec-less card gains no empty row. Plus a Go test that the spec survives weaveStory -> board.Sprint -> JSON.
