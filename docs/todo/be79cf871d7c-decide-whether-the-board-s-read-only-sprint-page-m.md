---
id: be79cf871d7c
kind: task
title: Decide whether the board's read-only Sprint page may serve a plan document, then link it
seq: 101
status: todo
priority: p2
created: 2026-09-07T21:05:47.361897Z
---

SPLIT OUT of d9edcb58913c by corbel on 2026-09-07 with the evaluation done, so this starts from corrected reasoning.

WHAT IS ALREADY SETTLED. The proxy objection recorded on sprint #135 is FALSE: pkg/webconsole/embed.go injects a <base href> into every served page precisely so that relative URLs need no per-mount configuration, and every other in-console link relies on it. A relative href is prefix-correct on loopback and behind /matrix/h/<host>/app/<name>/ by construction. Do not re-derive this and do not repeat the original reason.

WHAT IS ACTUALLY OPEN, and it is one decision plus one mechanic.

DECISION: may the Sprint page, which the atlas marks CapReadOnly, serve document bytes at all? #135's own acceptance says READ-ONLY STAYS READ-ONLY and that every story adds links out and disclosures, never an action. Serving a file is not an action, but it IS a data plane, and that is a question for the operator rather than an implementer.

MECHANIC: weaveStory.SpecRef is repo-relative and StoryRoots is a list, so a sprint spanning two repos has no single root. Resolve host-side where the roots are known; the single-root case is the common one and the multi-root case must degrade to today's plain reference rather than guess.

GATE. A verifydom case that drives the resolved target on BOTH a bare base and a proxied /matrix/h/<host>/app/<name>/ base and asserts it resolves in both, plus the multi-root case asserting NO anchor. TestDOMSprintShowsItsPlanReference currently asserts zero anchors and must be UPDATED, never deleted -- until this lands it is correct.
