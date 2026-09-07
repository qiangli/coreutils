---
id: e852de18d41d
kind: task
title: 'Sprint board: an unowned sprint says nothing and offers no way to staff one'
seq: 87
status: done
priority: p1
created: 2026-09-07T18:28:53.012378Z
sprint: 135
closed: 2026-09-07T19:09:27.718136Z
---

OBSERVED 2026-09-07. Sprint #130 is a live example: `bashy sprint show 130` reports "UNREACHABLE: no owner — nobody is accountable and no name can be addressed", and the browser card for it renders NOTHING where a manager would be.

THE DEFECT. board.js sprintEl() computes `const manager = sp.manager || sp.conductor || sp.lease_holder` and appends the manager row only `if (manager)`. Absence renders as absence. But an unowned sprint is not a sprint with one less field — it is a sprint that CANNOT BE ADDRESSED, which is the single most actionable fact on the card, and the board is the only surface where an operator sees it across every sprint at once.

WANTED.
(a) Every unowned sprint card carries an explicit "project manager — unassigned" row, in the same slot and with the same "meta manager" treatment as an assigned one, marked so it reads as a gap rather than as decoration.
(b) That row carries the SAME chat control story 1 fixes, linking into the apps/meet 1:1 chat, so the operator can pick a project manager and work with the chosen agent without leaving the board.
(c) The 1:1 opens with a DRAFT the way New sprint does — but not the new-sprint draft. This conversation is about an EXISTING sprint, so the draft either (i) names the sprint and asks the agent to help choose and install a manager for it, or (ii) is left blank. Implementer's call; if a default draft is used it must carry the sprint id and must remain editable, and it must not be an instruction that spends tokens on open.

DEPENDS ON story e97c123d (the icon and the conversation kind). Land that first or these two rows disagree about what a bubble means.

NOTE the durable/live distinction already in board.Sprint: Manager is the durable project-manager identity, Conductor is the live lease holder. "Unassigned" means no Manager AND no Conductor AND no LeaseHolder. A stale lease is a different state and already has its own label ("lease STALE — <name>"); do not collapse the two.

GATE. A verifydom case over a board fixture containing one owned and one unowned sprint, asserting the unowned card renders the unassigned row with a working dm href carrying the sprint id, and that the owned card is unchanged.
