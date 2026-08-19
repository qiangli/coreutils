---
id: 87a5dfbf5fa4
kind: task
title: 'Guard agents set --model: cascade dependents and stale ledger'
seq: 5
status: todo
priority: p1
created: 2026-08-19T16:54:20.342894Z
---

pkg/fleet/cli_write.go newAgentsSet re-points a binding with no warning about what rides along. MEASURED on 'bashy agents set ycode-glm-5.2 --model glm-5.3':

1. NICKNAME CHURN — nicknames are drawn from MatrixKey() (nicknames.go:56), so re-pointing redraws them. Xavier became Kenji and 'whois Xavier' answered 'names nothing on this host'. Nothing warned.
2. BLAST RADIUS — cascades name their base by AGENT name (base: ycode-glm-5.2), so all four ycode cascades silently moved onto the new model.
3. STALE LEDGER — the ring-copy carries ledger.reliability and ledger.notes verbatim onto a model that has run nothing ('gate PASS twice, 89s/125s'). A success state inherited rather than earned (docs/absence-of-evidence.md).
4. NAME/DISPLAY DRIFT — the record stays named ycode-glm-5.2, displayed 'ycode · glm-5.2', bound to ycode:glm-5.3.

FIX (warn, do not refuse — re-point is legitimate when a provider replaces an id in place):
- if the model CHANGES and the agent has no explicit Nick, print the old and new nickname;
- name every cascade whose Base is this agent, and require --force to proceed;
- clear ledger.reliability (leave notes, they are prose) unless --keep-ledger;
- if the agent's name or display embeds the old model version, say so.

Suggest a --pin/--successor path too: 'agents set X --model M' is usually the wrong verb and 'agents add X-<newver>' is the right one.
