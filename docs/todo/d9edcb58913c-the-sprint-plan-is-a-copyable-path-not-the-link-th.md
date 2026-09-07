---
id: d9edcb58913c
kind: task
title: The sprint plan is a copyable path, not the link the story asked for
seq: 97
status: todo
priority: p1
created: 2026-09-07T19:51:31.571672Z
sprint: 136
---

THE THIRD CARRIED ITEM FROM SPRINT #135, and the one that was nearly lost: it was recorded as evidence on the plan-link goal and given no story of its own, so unlike 86a6cc12 and 312252ae it had nothing tracking it.

WHAT WAS ASKED. Story 87eb79216d6a: "add the link to the master execution plan for the sprint". The #135 goal text was more specific still — "the card LINKS to the master execution plan with a target that resolves on loopback and behind an outpost proxy alike".

WHAT SHIPPED. The card renders the plan as a labelled, selectable path (`.plan .plan-ref`, click selects it for copying). TestDOMSprintShowsItsPlanReference asserts ZERO anchors. So the goal item was marked [x] against a deliverable that does not do the thing its own text describes. The evidence says so plainly, but a green checkbox with a caveat in its evidence is exactly the shape docs/fleet-evidence-invariant.md warns about, and the honest record is this story.

WHY IT WAS BUILT THAT WAY, because the reasoning is sound and should not be re-derived. weaveStory.SpecRef is REPO-RELATIVE ("docs/plan.md") and a sprint spans repos by definition — StoryRoots is a list. So there is no single root to resolve it against. The Files panel is optional, defaults to scoping HOME, and serves file bytes. And the Sprint page is reached both on loopback and under outpost's /matrix/h/<host>/app/<name>/ prefix. An href built from any of that works on one host and 404s on another, which is worse than a path, and inventing a file-serving route is a data plane a CapReadOnly page must not grow.

SO THE OPEN QUESTION IS NOT "add an anchor". It is: can a plan reference be made REACHABLE without any of the above becoming false? Three candidates, none evaluated:
  (a) Resolve server-side when the sprint has exactly ONE story root, and link into the Files panel only when that panel is enabled and its scope contains the file — rendering the plain path in every other case. Cost: two conditions the reader cannot see, so the same card links on one host and not another.
  (b) A read-only plan endpoint on the board API that serves the spec by sprint id, resolved host-side where the roots are known. Cost: a new data-plane path on a read-only page — needs an explicit decision, not a drive-by.
  (c) Keep the reference and CORRECT THE GOAL LANGUAGE instead, on the finding that a repo-relative reference on a repo-spanning record is not linkable in a location-independent way. This is a legitimate outcome, not a failure.

DECIDE BETWEEN THEM ON EVIDENCE, and record which and why. If (c), say so where a reader of #135 will see it, so the [x] is not left disagreeing with the artifact.

DO NOT ship an href that resolves on loopback and 404s behind the tunnel. That is the specific failure this deliberately avoided, and the existing test asserting zero anchors must be UPDATED rather than deleted if the answer changes.

GATE. Whichever branch is chosen: a verifydom case that drives the resolved target on BOTH a bare base and a proxied /matrix/h/<host>/app/<name>/ base and asserts it resolves in both, or — for (c) — the corrected goal text plus this story closed with the finding recorded.

## DECISION 2026-09-07 (corbel, sprint 136) — (c), and the recorded reason was half wrong

EVALUATED (a), (b) and (c) rather than defaulting to the cheapest. The finding
that matters is a CORRECTION to the reasoning this story was told not to
re-derive.

THE PROXY OBJECTION DOES NOT HOLD. #135 recorded "no href resolves both on
loopback and behind the outpost proxy". That is false, and the machinery that
falsifies it is already in this repo: pkg/webconsole/embed.go injects a
`<base href>` into every served page for exactly this purpose — "new
URL(x, document.baseURI) then needs no per-mount configuration ... which is
what keeps it correct under a route prefix". A RELATIVE href is therefore
prefix-correct by construction, on loopback and under
/matrix/h/<host>/app/<name>/ alike. Every other in-console link already relies
on it.

So the deviation recorded on #135 gives a reason that is not the real one.

WHAT THE REAL BLOCKER IS, and it is only half of what was written: the FILE
ROOT. weaveStory.SpecRef is repo-relative and StoryRoots is a list, so on a
sprint spanning two repos there is no single root to resolve the ref against —
and serving the bytes at all means adding a data-plane route to a page the
atlas marks CapReadOnly. That second point is a governance decision about what
the board page is allowed to become, not a UI detail, and it is not one to
take as a drive-by at the end of a time box while two other lanes share this
repo.

DISPOSITION: (c) for this sprint. The goal language is corrected rather than
the artifact, because a repo-relative reference on a repo-spanning record is
not linkable location-independently WITHOUT deciding to serve file bytes —
which is a separate, real decision. The correction is recorded on #135's
thread, where a reader of that [x] will see it.

BUT (b) IS NOW VIABLE, where before it read as speculative: with the proxy
objection dissolved, a read-only plan endpoint resolved host-side needs only
the single-root question answered and the CapReadOnly decision taken. Filed as
its own story with this evaluation attached, so the next person starts from
the corrected reasoning rather than the original one.

The existing test asserting zero anchors is left UNCHANGED and correct: today
the card renders a reference, and it must not grow an href until that decision
is made.
