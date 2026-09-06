---
id: b1f8e659957c
kind: task
title: 'sprint goal rm: a sprint whose stories moved to a successor could never be closed'
seq: 70
status: done
priority: p1
created: 2026-09-06T11:18:31.875384Z
sprint: 130
closed: 2026-09-06T11:30:42.361971Z
---

DEFECT (fixed here). A goal item checks only when every story linked to it is closed (sprintGoalDone), and `sprint move <id> done` refuses over any unchecked item — a refusal --force deliberately does NOT cover, because a plan you did not finish is not a plan you may declare finished.

That left one state with no exit. Move a sprint remaining open stories to a successor sprint — the ordinary way to close a sprint that ran out of time — and its goal items keep pointing at stories that now belong to the other card. They can never close on the old one, `goal link` refuses to re-point them (it requires it.Sprint == id), and there was no way to retire the item. The sprint became permanently unclosable.

OBSERVED on #123 and #126. #126 own continuity had already recorded it: "cannot honestly move to done because five legacy goals remain linked to deliberately deferred stories and the CLI has no unlink operation" — and that manager left the sprint in doing rather than misreport it. #123 hit the same wall from the other direction, with four production goals whose stories carried to #130.

FIX: `bashy sprint goal rm <sprint> <goal> --reason <why>`, the missing repair operation. --reason is mandatory and the removed item is written to the thread VERBATIM (text, gate flag, every story ref with its current status, any recorded evidence), so retiring the index entry loses no record. A reader of a closed card can still see every outcome ever required of it, who retired it, and why.

BOUNDARY, and it is the whole design: retiring an outcome says it is no longer required OF THIS SPRINT — it moved to a successor, or the operator dropped it from scope. It NEVER says the outcome was achieved. The verb must not become a way to make a red sprint look green; that is why the reason is required and the epitaph is complete.

Gate: TestSprintGoalRmRetiresAnOutcomeMovedToASuccessor (asserts the blocked state exists first, then that only the genuinely-unmet item still blocks, then that nothing is lost from the thread record) and TestSprintGoalRmRequiresAReason.

STILL OPEN, filed separately as observation not fix: `sprint move done` enforces the goal check while `sprint end` does not — #127 ended with four unchecked goals. Two closure paths disagreeing about the same question is a defect in its own right; decide which is right before either is relied on.
