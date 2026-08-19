---
id: 0a3965db9013
kind: task
title: Family alias must not land on a cascade agent
seq: 4
status: todo
priority: p1
created: 2026-08-19T16:54:01.659778Z
---

pkg/fleet/decorate.go: decorateAgents assigns the floating family alias <tool>-<family> to the first agent (canonical-name order) whose Model is the family's newest. Cascade agents carry their BASE's model, so they compete for it.

MEASURED before glm-5.3 landed: `bashy whois ycode-glm` resolved to ycode-cascade-claude-x3 (glm-5.2 -> opus4.8), not to ycode-glm-5.2. The alias that is supposed to mean 'the newest glm on ycode' pointed at an agent that escalates to opus4.8 — a silent quality/cost misroute for anyone addressing the family.

It is latent right now only because the cascades still pin glm-5.2 while the plain agent binds 5.3. It returns the moment the cascades are moved to the newest model, which is the normal end state of every model bump.

FIX: in the family-alias pass, skip agents where IsCascade() (BandSource == "cascade" && Base != ""), so the alias can only land on a plain binding. Test: two agents on the same newest model, one cascade, alphabetically ahead of the plain one; assert the alias resolves to the plain one.
