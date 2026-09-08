---
id: 312252aeb358
kind: task
title: 'mb has no selector for what an agent is DOING: address the live sprint managers'
seq: 96
status: todo
priority: p2
created: 2026-09-07T19:27:40.311501Z
sprint: 139
---

SPLIT OUT OF 365ed78773c4 on 2026-09-07, with the rest of that story delivered. This is its point 1, held back for a reason that is not effort.

THE GAP, unchanged from the parent story. Every `mb send` selector — --to, --band, --tool, --provider, --family, --version — is a property of an agent's BINDING. None is a property of what the agent is DOING. So a manager who wants to reach exactly their peers must post to EVERYONE (over-broad, capped at -n 5 for anyone who has not declared the concern) or hardcode names (rots the moment a lease moves). The sprint records know every live lease holder; the messaging layer cannot ask.

WHY IT WAS NOT DELIVERED WITH THE REST. The selector cannot be finished inside coreutils. `bus.Audience` and the selector flags are here, but the resolution seam is `bus.FleetSelect`, injected by the HOST — pkg/bus is transport and the roster is policy, which is the right split and should not be broken to land this. The injection lives in `bashy/internal/agentos`, a different repo, which was being actively committed to by the Sprint 117 lane while this sprint ran. dhnt's shared-repo rule is that a bashy/coreutils/sh edit during another lane's arm needs that lane's ack first, and disjoint paths are not sufficient because the gate is shared. Landing a cross-repo change unilaterally to save a round trip is exactly the move that rule exists to stop.

WHAT SHIPPED INSTEAD, so the announcement was not blocked on this: stage changes post under the `sprint` TOPIC. A reader who declares that concern (`bashy bus subscribe --topic sprint`) sees every one uncapped; everyone else sees them under the ordinary cap. That is the mechanism mb already provides for exactly this, and it is honest — but it is opt-in per reader, where a role selector would be addressed by the sender.

WANTED.
(a) `Role` (or `HoldingLease`) on bus.Audience, with `mb send --role conductor`.
(b) The host-side resolution: coreutils exposes the live lease holders — the sprint store is already read by pkg/weave — and bashy's agentos wires it into bus.FleetSelect beside the fleet-catalog resolution it already installs.
(c) LIVE leases only. A STALE lease names a conductor who died without handing off and an unowned sprint names nobody; neither is a manager, and addressing them would be mail nobody reads.
(d) Once it exists, the stage announcement should ADDRESS the peers rather than rely on each of them having declared the concern. That is the actual payoff and the reason to do this.

FIRST STEP IS NOT CODE: get the ack. Post to the bashy lane, or wait for their arm to close.

GATE. A Go test that the selector resolves exactly the live-lease holders and excludes stale and unowned; the existing bashy/internal/agentos seam test extended to cover the new injection; and a manual check that `mb send --role conductor` reaches a second seated manager on this host.
