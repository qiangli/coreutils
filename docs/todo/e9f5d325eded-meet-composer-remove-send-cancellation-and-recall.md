---
id: e9f5d325eded
kind: task
title: 'Meet composer: remove send cancellation and recall, restore the plain send'
seq: 89
status: todo
priority: p0
created: 2026-09-07T18:29:23.764992Z
sprint: 135
---

OPERATOR DECISION, 2026-09-07: the improved send never worked. Take it out. This is a REMOVAL story, not a repair — a dedicated sprint owns fixing it properly later, and nothing here is a judgement that the idea was wrong.

WHAT IS BEING REMOVED. The hold/cancel/recall machine added by coreutils aa52f3e3 ("take a message back, and let a seat act") and adjusted by acb7d927 (ten-second recall window) and 0247d085 (send a second message):
- use-meet-room.ts: PendingSend, RecallOutcome, DEFAULT_HOLD_MS / RECALL_WINDOW_MS / holdMs(), the pending + pendingRef + recalling state, the hold timer, the dispatch/expiry split, cancelSend, and the recall / recallDM API calls it drives.
- composer.tsx: the `pending` / `heldFor` / `recalling` / `onCancel` props, the `confirming` retraction dialog, the holding and inFlight branches, and the send button's dual identity as a cancel button.
- App.tsx: whatever it threads between the two.

WHAT REPLACES IT — the simple one that worked.
(a) Click or Enter sends. Immediately. Nothing is held in the browser.
(b) An ACCEPTED indicator: the message was taken by the host. This is the honest claim — accepted is not answered — and the existing `queued` notice already says "Working." for the case where an agent has been woken and has not replied yet. Keep that.
(c) The response is shown when it is available. This is the half that matters and the half the operator could not reach past the recall UI.
(d) Errors stay visible. The existing error banner keeps its behavior.

DECIDE AND RECORD: whether the server-side recall endpoint stays. The UI stops calling it either way; leaving a reachable endpoint with no caller is acceptable ONLY if the future sprint is named in a comment on it. Do not leave a half-wired protocol with no note saying who owns it.

DO NOT LOSE THE REASONING. The comments deleted here explain a real design (why the confirm is on the delivered branch and not the held one; why writing the next message is itself the decision not to recall). Move that reasoning into the story that reopens the work, or into a short note beside the send path — it was expensive to write and the next attempt will need it.

GATE. A Playwright case: send a message and assert it is delivered with no hold and no countdown; assert the accepted indicator appears; assert a response renders when the host returns one; assert no cancel, recall or retraction control exists anywhere in the composer. Plus the verifydom pass over the /meet/ tabs.
