---
id: 8ef9961e39c3
kind: task
title: 'pkg/foreman: the PACKAGE fails intermittently while every test in it passes'
seq: 21
status: todo
priority: p2
created: 2026-09-01T14:31:46.174722Z
sprint: 137
---

`pkg/foreman TestServeControlStopCancelsActiveTurn` failed on ubuntu-latest in
Actions run 33518958296 -- a commit whose entire diff was markdown plus the
ratchet baseline, so no code changed -- and passed on 08e59143 and in the two
pre-ratchet runs 33513814323 / 33515325794.

The interesting observation is local, on darwin. One invocation of

    go test ./pkg/foreman -run TestServeControlStopCancelsActiveTurn -count=5

reported FAIL for the PACKAGE while the named test printed `--- PASS` all five
times, and the next identical invocation was clean with rc=0. A package that
fails while every test in it passes means the failure came from outside the test
bodies: a leaked goroutine outliving its test, a TestMain check, or shared state
touched after the test returned.

Start from that, not from the test name. Do NOT close this by adding a retry
loop inside the test.

Not baselined. scripts/ci-test-gate.sh re-runs an unbaselined failure once and
reports this class as FLAKY, so it blocks nothing -- a holding position, not a
disposition.

The POSIX half of this story (cmds/dd status output, a conformance rule on a
certified utility) was split out to 5db8d65e under the POSIX cert sprint.

## Sprint 137 investigation and implementation

The audit started outside the test bodies. `pkg/foreman` has no `TestMain` or
package exit check. `ServeControl` closed its listener but did not join its
lifetime watcher, accepted connection handlers, or the detached
`Apply`/`saveState` workers. Closing a listener also leaves already accepted
connections open, so idle handlers remained blocked in `Scanner.Scan`.

Two controlled regressions reproduce the lifetime defect against the unchanged
implementation: `TestServeControlJoinsCancelledTurn` holds a cancelled runner in
its cleanup and observes `ServeControl` return while the turn still owns session
state; `TestServeControlClosesIdleConnections` completes a protocol exchange and
then observes its accepted socket survive server shutdown. Both fail without
retries. Late turn persistence can race a caller tearing down the session store
(in tests, `TempDir` cleanup); this is a concrete ownership defect, even when an
individual test's assertions have finished successfully.

The historical evidence is narrower than the story's local report: Actions run
33518958296 retained only a ratchet summary naming
`TestServeControlStopCancelsActiveTurn` as a failing **test**, without its raw
failure output. The current unmodified implementation passed 200 repetitions of
that test locally. We therefore cannot establish the exact cause of either
historical failure from those records alone, and do not claim to have reproduced
the historical package-only output.

`ServeControl` now cancels and joins all work it starts, closes accepted sockets,
and waits for final state persistence before returning. Its handler remains
counted while it launches a turn, preventing a WaitGroup Add/Wait gap. Commands
queued behind an active turn check cancellation under the state lock before
mutating it. A stop ACK is attempted before socket teardown with a 100 ms write
bound; an unread ACK cannot suppress cancellation. The new unread-ACK regression
uses an unbuffered `net.Pipe`, so that backpressure condition is deterministic.
An injected Runner must honor its context and complete cleanup; Go cannot safely
force an arbitrary caller-provided goroutine to terminate.

Validation in the isolated worktree: the two lifecycle regressions failed before
the implementation. Final race-enabled stress passed 100 repetitions of all
eight control/cancellation regressions (800 test PASS events and package PASS).
The full package passed with `-race`; Linux and Windows package vet passed.
Evidence files are retained under the umbrella's ignored `.agents/sprint137/`:
`foreman-regression-red.log`, `foreman-before.json`,
`foreman-after-control.json`, and `foreman-race.log`.
No retry was added inside a test and no baseline entry was added. Story closure
is pending Sprint 137 integration gates and CI evidence.
