---
id: 365ed78773c4
kind: task
title: A sprint's stage changes tell nobody, and 'every active sprint manager' is not an address anyone can write
seq: 94
status: todo
priority: p1
created: 2026-09-07T18:47:58.246197Z
sprint: 135
---

RAISED BY THE OPERATOR 2026-09-07 while sprint #135 was taken, then widened in the same conversation: should `sprint take` auto-post to mb so every active sprint manager is notified, or should the help text merely advise it? And the same for handoff, end — EVERY STAGE CHANGE of a sprint.

THE NEED IS REAL. Several managers run concurrently on one host against shared repos, a shared gate and a shared pin order. A sprint changing stage — started, taken, handed off, stopped, ended, moved, aborted — is information the others act on. Today every one of those verbs changes state and tells nobody outside the sprint, and `sprint take`'s help, which already teaches "YOU ARE NOW THE SPRINT MANAGER, NOT ITS SOLE WORKER", never mentions announcing.

BUT NEITHER PROPOSED ANSWER IS RIGHT ON ITS OWN, and widening from one verb to all of them makes both worse, not better.

Against an unconditional auto-post per verb:
- `take` is also RECOVERY. Its own help says a STALE lease is taken directly after a SIGKILL or token exhaustion, and a flapping conductor takes repeatedly. mb is append-only and nothing is ever deleted, so a post per call makes a crash loop permanent noise. Multiply that by eight verbs.
- mb is PUBLIC BY CONSTRUCTION and everyone is subscribed to `announce`. `mb send` deliberately ANDs its selectors and says why: "a union would make the wider blast radius the easier thing to type, and on a shared board the wide one is what turns messages into noise nobody reads." Automatic broadcast on routine verbs is that union by another route.
- Posts are capped at -n 5 for readers who have not declared the concern, so the announcements are ALSO the first thing trimmed from a busy reader's view. docs/mb-addressing-model.md records this: only directed posts escape the cap, so a post to "everyone concerned" can be silently trimmed.
- It gives every state-changing verb a second failure mode. Lease claimed, post failed — what is the exit code? An `end` that reports failure because a MESSAGE did not send is worse than no message.

Against help text alone: it is the weakest possible intervention, and docs/fleet-evidence-invariant.md is the reason to distrust it — no success state may be reached by the ABSENCE of evidence. "We advised it in --help" is exactly that.

THE FINDING UNDERNEATH, and it is the part worth building. "All active sprint managers" IS NOT AN ADDRESS. Every `mb send` selector — --to, --band, --tool, --provider, --family, --version — is a property of the agent's BINDING. None is a property of what the agent is DOING. So a manager who wants to reach exactly their peers must post to EVERYONE (over-broad, capped, noisy) or hardcode names (rots the moment a lease moves). The sprint records know precisely who holds every live lease; the messaging layer cannot ask.

WANTED, in this order.

1. MAKE THE SET ADDRESSABLE. A selector for the role, not the binding — e.g. `mb send --role conductor` / `--holding-lease`, resolved from the live sprint leases the way --band resolves from the catalog, so who is a manager "here and there can never drift" (mb send's own words about --band). Live leases only; STALE and unowned are not managers.

2. ANNOUNCE THE THREAD, NOT THE VERBS. Do NOT sprinkle a post call into eight commands — they will drift, and a verb added later will silently not announce. The sprint ALREADY has an append-only stage log: every entry goes through the single seam weaveStoryAppend(s, author, kind, body) in pkg/weave/weave_story.go, 27 callers, and the entries already read "[09-06 11:03] conductor (system): created in doing". Hook the PROJECTION there and announcement becomes a view of the thread rather than a second event source. The two can then never disagree, and any future stage change is announced for free.

3. ANNOUNCE A CHANGE, NOT A CALL. Only entries that record an actual transition announce; notes, checkpoints and comments do not. A re-take by the SAME owner announces nothing — that idempotence is what kills the crash-loop spam. This is why `kind` on the thread entry is load-bearing and why the projection must filter on it rather than on the calling command.

4. THE STATE CHANGE IS THE TRANSACTION. The post never fails the verb. A failed announce is REPORTED on stderr and the start/take/handoff/end still succeeds — and the report must be visible, not swallowed, or this becomes another success state reached by absence. Default on, suppressible with `--announce=false`, carrying a topic so a declared concern lifts the -n cap.

5. UPDATE THE HELP TOO, because that is where the seat's responsibilities are already taught. Announcing belongs beside "you are now the manager", not instead of the mechanism.

DECIDE AND RECORD: which thread kinds are transitions. Today only two kinds appear in the tree ("system", "decision"), so the taxonomy needs stating before it can be filtered on — and stating it is part of this story, not a prerequisite someone else supplies.

RELATION TO f1c221d2. Same shape, third instance: the sprint records hold a fact (who manages what, and what changed) that the surrounding surface cannot express. That story joins a seat to its fleet record for READING; this one makes the set of seats ADDRESSABLE for WRITING and projects the stage log onto the board. If a shared "resolve live sprint leases" helper falls out of doing both, say so in the commit.

SCOPE NOTE. This is CLI and messaging, not the browser Sprint UI this sprint was opened for. Filed here at the operator's request; if the sprint is retitled to the sprint SURFACE rather than the sprint CARD, this and f1c221d2 are why.

GATE. Go tests over fixture sprints and a fixture catalog: the selector resolves exactly the live-lease holders and excludes stale and unowned; each stage transition (start, take, handoff, stop, end, move, abort) announces exactly once; a re-take by the same owner announces nothing; a checkpoint or comment announces nothing; a failing board makes the announce report an error while the state change still commits; --announce=false posts nothing. Plus a help-text assertion, since point 5 is part of the deliverable.
