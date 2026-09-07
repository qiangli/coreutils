---
id: f1c221d2f05e
kind: task
title: 'Sprint status names a manager but cannot say what it is: join the seat to the fleet record'
seq: 93
status: todo
priority: p2
created: 2026-09-07T18:40:25.19013Z
sprint: 135
---

OBSERVED 2026-09-07, answering "which sprints are active and who manages them". Both halves exist and NOTHING JOINS THEM, so the operator runs one command per name to finish the answer.

WHAT ALREADY WORKS, and this story adds no machinery to either side.
- The seat: `sprint status` groups by clock (RUNNING NO LIVE CONDUCTOR / ON THE CLOCK / OPEN NOT ON THE CLOCK / STOPPED) and prints the holder with its three real states — live, STALE, unowned — plus the contact line "sprint N · meet #M · bus conductor.N". Printer is in pkg/weave/weave_story_box.go.
- The agent: `bashy agents show <name>` gives tool/model/launch, `bashy whois <name>` adds binding, source, claim, liveness and the RANKED CONTACT LADDER. The lookups behind them are fleet.Catalog.Agent(name) (pkg/fleet/catalog.go) and principal.Resolver.Resolve(query) (pkg/principal/resolve.go).

THE GAP. The sprint record stores a NAME and only a name — by design, and that stays: pinning a binding into the sprint would rot the moment the agent is re-bound. But `sprint status --json` carries exactly {id, title, epic, column, box_status, cycles, lease_holder, contact} and no tool, model or band, and neither the status nor the tick path resolves a holder against the catalog. So the operator's loop today is:

  bashy sprint status --json | jq -r '.result | (.on_clock, .unowned_delivery, .idle)[]? | select(.lease_holder) | .lease_holder' | sort -u | while read a; do bashy whois "$a"; done

WANTED. One command answers it. Implementer's call which of these it is; pick ONE and say why:
  (a) an opt-in flag on the existing view — `sprint status --agents` / `--resolve` — that decorates each holder with tool:model and band;
  (b) resolution always on in the JSON envelope (cheap, structured, no terminal cost) and behind a flag in the text view;
  (c) a new read-only verb if and only if (a) and (b) are both wrong — and note docs/orchestration-verb-consolidation-audit.md: 28 orchestration verbs already exist, 17 with no recorded justification, so a new verb needs a stated Why and is the LAST option, not the first.

INVARIANTS, and they are the reason this is not a five-line change.
1. REUSE THE RESOLVERS. Call fleet.Catalog.Agent / principal.Resolver. Do not re-derive a binding from a name, and do not parse `agents show` output.
2. AN UNRESOLVABLE HOLDER IS A FINDING, NOT A BLANK. A sprint whose manager is not in the fleet is exactly the state an operator must see — the name was mistyped, the agent was removed, or the seat was taken by something nothing can push to (see sprint #130's story c8bb9009078d, "sprint take accepts an owner nothing can push to"). Report it as unresolved with the name intact; never drop the row and never invent a binding.
3. DO NOT PROBE. sprint tick already states the rule and the cost: a probe is a real headless turn per row, and installed is NOT signed in. This join is a CATALOG READ only. Resolution must not spend provider capacity, and must not be slow enough that anyone stops running `sprint status`.
4. STILL READ-ONLY. sprint status "reports and changes nothing"; it must not refresh a lease, mark mail read, or claim a seat. Resolution cannot become a liveness signal.
5. STALE AND UNOWNED SURVIVE THE JOIN. The three seat states are the point of the view; decorating a holder must not flatten them into "has an agent / has none".

RELATION TO 131a68c5. That story is the SAME missing join on the browser side — a story names work and the worker's identity lives on the run. This one is the CLI side: a sprint names a seat and the seat's identity lives in the fleet. Same shape, different records; do not merge them, and if a shared helper falls out of doing both, say so in the commit.

GATE. A Go test over a fixture catalog: a resolvable holder is decorated with tool:model and band; an unresolvable holder is reported as unresolved WITH its name; an unowned sprint gains no agent fields; a STALE holder is still reported STALE after decoration. Plus a test asserting the resolution path performs no probe and no write — the invariant, not the coverage.
