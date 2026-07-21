# Utilization verdict — one thing I could NOT verify live

The engine, the board banner, and the three-case gate are all verified
(see the commit message for the exact test output and the exact live
`resources utilization` verdict against this machine's real fleet
catalog: 32 agents, 3 busy, 29 idle, OPTIMAL at 0 pending).

What I could not produce is a **live end-to-end board run** —
`board.Collect(ctx, ..., DefaultSources(), nil)` on this machine, which
is what `bashy board` and `board.PendingWork` call.

## Diagnosis

It hangs, and the hang is pre-existing and outside this change. Timing
each `DefaultSources()` source in isolation:

    SOURCE weave        HUNG >20s      <-- never returned; killed at 600s and again at 150s
    SOURCE todo         took 3ms       err=<nil>
    SOURCE sprint       took 1ms       err=<nil>
    SOURCE fleet        took 161ms     err=<nil>
    SOURCE resources    took 0s        err=<nil>

Every source this change touches or adds is in the millisecond range.
The weave source (`pkg/board/sources.go`) is the one that never returns
on this host. `go test ./pkg/board/...` stays green because the package
tests inject fixture sources rather than `DefaultSources()`.

I did not chase it further: the live weave scan reaches into
`pkg/weave`, which another agent is working in concurrently and which
this ticket is scoped out of.

## What I changed as a result

`Board.evaluateUtilization` now returns early when `len(b.Agents) == 0`
instead of calling the fleet collector. With no board-supplied agents
the collector falls through to its LIVE branch, which spawns `weave
fleet` / `weave list` subprocesses — that would have made board
finalize (a pure projection of already-loaded sources) able to hang the
same way, on a code path that previously did not exist. A board with no
agents source now reports no verdict rather than a guessed one; the
panel renders "unavailable" and the banner is omitted.

Note this hazard already exists in `fleetPanel()` (`pkg/board/panels.go`),
which passes a nil agent slice into the same collector when `b.Agents`
is empty. I left it alone — it predates this ticket and fixing it is a
behavior change to the existing fleet panel, not to utilization.

## Suggested follow-up (NOT applied — out of scope)

Give the weave board source a bounded context so one slow store cannot
hang the whole board; `Collect` already turns a source error into a
warning, so a deadline degrades to a visible warning rather than a hang:

    // pkg/board/sources.go, in the weave source Load
    ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
    defer cancel()

This is unverified — I did not run it, because reproducing the hang
costs 2.5+ minutes per attempt and the fix belongs with whoever owns
the weave source.
