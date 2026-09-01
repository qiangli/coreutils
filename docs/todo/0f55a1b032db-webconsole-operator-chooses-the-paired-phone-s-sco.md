---
id: 0f55a1b032db
kind: task
title: 'webconsole: operator chooses the paired phone''s scope in Settings'
seq: 30
status: todo
priority: p1
created: 2026-09-01T20:41:53.951989Z
assignee: ci-repair
sprint: 101
---

A pass was always board/mb/relay, and 'bashy apps pair --allow ...' was the only way to widen it -- so a phone that hit 'terminal is not in the scope' got a refusal and no way to act on it.

Settings now offers a checkbox per panel the console actually serves. The shell and the filesystem stay OFF by default: default-deny is the point, and a grant made by ticking a visible box is a different thing from one made by a default nobody read.

Safe because the decision is bounded on every side rather than trusted: the page already required an OS login, the chooser is the signed-in operator (isOperator), the server validates names against the panels it serves (ValidateScope), an absent or malformed body keeps the default, and scope is only ever a NARROWING of the operator's own session.
