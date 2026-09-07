---
id: 131a68c509d4
kind: task
title: 'Sprint board: a story chip shows closed-or-not, never who is working it'
seq: 90
status: todo
priority: p1
created: 2026-09-07T18:29:41.0671Z
sprint: 135
---

OBSERVED 2026-09-07. On a sprint card, every open story looks identical. board.js storyIsClosed() sorts stories into exactly two buckets — done/closed/cancelled/canceled versus everything else — so a story that has a worker agent running right now renders the same as one nobody has touched. The one question a scan of the board is asking ("what is actually moving?") is the one it cannot answer.

Note stateClass() already knows more than storyIsClosed uses: NEEDS (submitted, review, failed, blocked), LIVE (working, doing, allocated, running), past. It IS applied to the chip; what is missing is that a todo's `status` alone does not say a worker was assigned, and the assignment lives on the RUN, not on the story.

WANTED.
(a) Distinct visual states on the story chip and the story row, in the sprint card, for at least: unstarted, assigned/in-progress, needs attention (submitted/review/failed/blocked), and closed. Reuse the existing needs/live/past classes and CSS rather than inventing a palette.
(b) Clicking a story shows the working detail alongside the body openStory already renders: the worker AGENT name, its MODEL, its band, the run state and how long it has been running.
(c) Design and implementation are the implementer's call; what is fixed is that the board must not claim a state it cannot source.

THE HARD PART, and it is a data problem before it is a CSS problem. board.Todo carries ID/Number/Title/Status/Priority/Scope/Due/SprintID and NO run link. board.Run carries Agent, Model, Band, State, StartedAt, AgeSeconds, Stale and SprintID — but nothing that names the story it is executing. So story-to-worker correlation does not exist in the payload today. Settle it explicitly:
  (i) find or add the durable link (weave's sprintRun is {repo, id, queue, born}; the sprint's own story index and the run label are the only current bridges), or
  (ii) if no reliable link exists, say so in the UI — an unlinked run is reported as unlinked, never guessed onto a story.
A chip that guesses which agent owns which story is worse than a chip that admits it does not know: an operator acts on it.

READ-ONLY. This adds a disclosure, never a control. No claim, no assign, no kill from this page.

GATE. verifydom over a fixture with one story in each state and at least one linked run, asserting the four classes render distinctly and that the detail pane names the agent and model. Include a fixture with a run that CANNOT be correlated, asserting the UI says so rather than attaching it to the nearest story.
