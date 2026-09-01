---
id: b5af634ddde6
kind: task
title: 'webconsole: browser e2e suite for the bashy apps console'
seq: 28
status: todo
priority: p1
created: 2026-09-01T19:54:30.947912Z
assignee: ci-repair
sprint: 101
---

Byte-level tests cannot see the cascade, the DOM, or a script that throws. Four UI defects shipped through that gap in one day; the worst was a fix in the pairing section that stopped the Settings dialog opening entirely while every existing test still passed.

console_dom_test.go drives real Chrome over: launcher, Settings in both pairing states (all six sections built), login eye toggle, pairing QR + Refresh, background swatch actually applying, Files return control (mark/box/target), and every panel including relay and terminal -- each asserting the page threw nothing.

Proven to bite: reintroducing the insertBefore bug fails it with the exact NotFoundError. Wired into CI on the Linux leg (the runner image ships Chrome); tag-gated so a contributor without a browser is not blocked.
