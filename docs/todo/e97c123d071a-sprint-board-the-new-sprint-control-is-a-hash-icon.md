---
id: e97c123d071a
kind: task
title: 'Sprint board: the New sprint control is a hash icon and opens the wrong conversation kind'
seq: 86
status: done
priority: p1
created: 2026-09-07T18:28:38.763186Z
sprint: 135
closed: 2026-09-07T19:09:27.693501Z
---

OBSERVED 2026-09-07 on the browser Sprint board.

THE DEFECT, and it is two defects wearing one control. board.js newSprintLink() calls conversationLink("chat", "1", ...). conversationLink draws the "dm" path only when kind === "dm"; every other kind falls through to the hash glyph ("M4 9h16M4 15h16M10 3 8 21M16 3l-2 18"). So the one control that starts a CONVERSATION with an agent is drawn as a CHANNEL marker, which is the icon this same file uses for "Everyone" in the composer's recipient menu. The icon says broadcast; the link means 1:1.

Then the link itself: kind "chat" with ref "1" is not the 1:1 the draft text asks for. The draft says "Help me define its title, project manager, scope, stories..." — that is a conversation with ONE agent, and the Meet app's own vocabulary for that is a dm (see Composer's kind === "dm" branch: one recipient, stated not chosen, no recipient dropdown).

WANTED.
(a) The control draws the SPEECH-BUBBLE path, the same d= the dm branch already uses, so the glyph and the destination agree.
(b) It opens a 1:1 chat, not a channel — meetHref("dm", <agent>) shape, carrying the same NEW_SPRINT_DRAFT.
(c) It stays a DRAFT, never an instruction. The comment on newSprintLink is load-bearing: opening a link must not spend tokens or start work. Whatever agent the 1:1 lands on, nothing is sent until the operator presses send.

OPEN DESIGN QUESTION the implementer must settle and record: a dm needs a counterpart, and "new sprint" has no manager yet by definition. Options are (i) route to the host's steward seat, (ii) route to a named default manager agent from the fleet, (iii) keep a chooser step. Pick one, say why in the commit, and make the failure legible if no such agent exists — a link that silently opens an empty conversation is the defect class this sprint is about.

GATE. A verifydom case that loads the Sprint page, finds the New sprint control, and asserts BOTH the rendered path data (bubble, not hash) and the resolved href (dm kind, draft param present). Red first.
