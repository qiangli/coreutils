# The weave lifecycle: states, transitions, and the no-limbo invariant

**THE INVARIANT.** Every weave item has a complete lifecycle — *create →
process → close* — and **no item may sit in a state that is not a determinate
step toward closure**.

This is the runtime twin of the fleet-evidence rule ("no success-state is
reached by the *absence* of evidence") applied to LIFECYCLE. Evidence-invariance
protects against believing something worked when nothing proved it did. This one
protects against something never resolving at all — which is how finished work
gets silently lost, and how a fleet's capacity quietly drains into runs that
stopped being runs hours ago.

The invariant is a *checkable property*, not a slogan. The state machine below
is declared in code (`pkg/weave/weave_lifecycle.go`) and asserted by
`TestLifecycleHasNoLimbo`: every non-closed state must have at least one
declared transition, and every state must reach `done` or `abandoned` by some
path. A state added without a way out fails the build.

## States

| State | Meaning | Closed? |
|---|---|---|
| `todo` | filed, unclaimed | no |
| `allocated` | claimed; workspace being provisioned, agent not yet running | no |
| `working` | an agent is running under a supervising wrapper process | no |
| `paused` | deliberately held by a steward; workspace preserved | no |
| `finalizing` | a conductor holds a short lease to record terminal evidence | no |
| `submitted` | the run stopped cleanly **and** weave measured commits ahead | no |
| `failed` | the run stopped without qualifying evidence | no |
| `killed` | stopped by `weave kill` or the runtime/idle watchdog | no |
| `done` | the work landed in base | **yes** |
| `abandoned` | deliberately given up; workspace released | **yes** |

**"Closed" is narrower than "terminal."** `isTerminalState` additionally counts
`submitted`/`failed`/`killed` — that means *the run stopped*, which is not the
same as *the item is finished*. Conflating the two is precisely what let a
submitted item hang forever: nothing was watching, because something else had
already declared it terminal.

## Transitions — and who fires each one

Legend: **A** = automatic (the substrate fires it, no human in the loop);
**R** = fired by the REAPER; **S** = a steward verb.

| From | To | Fired by | |
|---|---|---|---|
| `todo` | `allocated` | `weave start` / autopilot claim | A |
| `todo` | `abandoned` | `weave abandon` | S |
| `allocated` | `working` | the wrapper launches the agent | A |
| `allocated` | `failed` | `weave start` provisioning error | A |
| `allocated` | `failed` | launcher died / provisioning timed out | **R** |
| `working` | `submitted` | wrapper terminal: exit 0 **and** commits ahead | A |
| `working` | `failed` | wrapper terminal: non-zero exit or no commits | A |
| `working` | `killed` | `weave kill` / runtime+idle watchdog | A |
| `working` | `failed` | **wrapper pid dead with no terminal evidence** | **R** |
| `working` | `paused` | `weave pause` | S |
| `working` | `finalizing` | `weave finalize` (conductor claims an idle TUI) | S |
| `paused` | `working` | `weave resume` / `weave start --resume` | S |
| `paused` | `abandoned` | `weave abandon` | S |
| `finalizing` | `submitted` / `failed` | the finalizer records terminal evidence | A |
| `finalizing` | `working` | lease expired, wrapper still alive | **R** |
| `finalizing` | `failed` | finalizer died before terminal evidence | **R** |
| `submitted` | `done` | `weave pull` (merge into base) | S |
| `submitted` | `done` | work already contained in base (merged out-of-band) | **R** |
| `submitted` | `working` | `weave start --resume` (branch kicked back) | S |
| `submitted` | `abandoned` | `weave abandon` | S |
| `failed` / `killed` | `working` | `weave start --resume` | S |
| `failed` / `killed` | `done` | `weave salvage` + `weave pull` | S |
| `failed` / `killed` | `abandoned` | `weave abandon` / `weave prune --stale` | S |

Every non-closed state has either an automatic transition **or** an entry in
`weaveLifecycleNeedsSteward` declaring that its exit is deliberately a human
decision (`paused`, `submitted`, `failed`, `killed`). There is no third
category — "somebody will probably notice" is exactly what
`TestEveryOpenStateIsAutomaticOrAnAvowedStewardDecision` forbids.

## The reaper

`pkg/weave/weave_reaper.go`. Runs on `weave list`, `weave status`,
`weave doctor`, and every heartbeat tick. One queue lock, all rules:

```
allocated, launcher dead        -> failed
finalizing, lease expired       -> working (wrapper alive) or failed
working, wrapper pid dead       -> failed: wrapper-died
submitted, already merged       -> done
submitted, past threshold       -> FLAG needs-steward  (state unchanged)
failed/killed with commits      -> FLAG salvageable    (state unchanged)
```

### Two absolute constraints

1. **It never destroys committed work.** The reaper writes state fields and
   flags. It removes no workspace, no branch, no commit. Disposal stays an
   explicit guarded step (`weave prune`, `weave abandon`). A run reaped to
   `failed` is still resumable — `weave start --resume` reattaches to exactly
   the workspace it left.
2. **It never invents success.** A dead wrapper becomes `failed`, never
   `submitted`: success still requires a clean exit **and** measured commits.
   Where work exists behind a failure, the reaper *surfaces* it (salvageable)
   rather than promoting it.

It is idempotent by construction — each rule's precondition is falsified by the
write it performs — and `TestReaperIsIdempotent` pins that a second pass over a
reaped queue returns no actions and appends no comments.

### Why flag instead of transition?

Two of the six rules only set a flag. That is not timidity; the decisions they
represent are genuinely not a machine's to make.

- **submitted past the threshold** — merging is `weave pull`'s job, and pull owns
  the verify / review / isolation gates. A reaper that merged around them would
  be a reaper that ships unverified work. So it names the decision instead:
  *unmerged for 2h13m; verify failed (exit 1) — `weave reverify` or `weave
  abandon`.*
- **failed/killed with commits** — a crash is a crash. But "failed, and there is
  finished work on the branch" is a determinate steward decision, where plain
  "failed" was an invitation to redo the work for free.

Both flags are **persisted** (`salvageable`, `needs_steward`,
`steward_reason`), unlike the computed `stale`/`blocked` display fields. That is
the whole fix: the pending decision used to exist only for as long as one
command's output was on screen.

### Measure, don't trust the report

`weaveReapSalvageable` re-measures the branch instead of reading
`commits_ahead`. That field is written by the *wrapper* — which, in exactly the
interesting case, is the process that died. A tool that commits its changes and
then crashes on the way out never reaches the evidence-recording step, so the
queue records `commits_ahead=0` while the branch carries a complete feature.
Concluding "no work" from the absence of a report by a process that did not
survive to report is the fleet-evidence mistake in a different costume. The
artifact is on disk. Ask the artifact.

## Inspecting it

```
weave doctor          # run the reaper, then list every open item + what closes it
weave doctor --json   # {"reaped": [...], "open": [{"issue","state","next_steps",...}]}
weave status <id>     # includes a "next:" line for any non-closed item
weave list            # reaps first; footers name salvageable + needs-steward runs
```

`weave doctor`'s audit is the invariant made visible: for every item that is not
closed, the named next step that will close it. An item whose answer is
"nothing" is a limbo — and `weaveNextSteps` reports it in those words rather
than inventing something plausible.

## The observed limbos this closes

Each of these hung indefinitely in production before the reaper existed:

1. **`working` with a dead wrapper pid** — displayed `(stale — wrapper pid
   dead)` for hours (one run at 2h56m, another at 5h26m). The marker was honest
   and completely inert: it described the limbo instead of ending it. Meanwhile
   `weave wait --all` blocked on those runs and the scheduler counted them as
   live capacity.
2. **`submitted` that never merged** — the auto-transition to `done` fired only
   on a *detected* branch merge, so a submission nobody pulled was
   indistinguishable from work in flight, forever.
3. **`failed`/`killed` with committed work** — state said failed, work existed.
   Indeterminate, and the obvious response to a failed run is to run it again.
4. **`allocated` orphans** — workspace created, agent never launched; hung until
   somebody ran `prune --stale`.
5. **`finalizing` whose conductor disappeared** — phantom capacity held by a
   lease nobody was holding.

## Related

- `docs/fleet-evidence-invariant.md` — the evidence half of the same rule.
- `pkg/weave/weave_lifecycle.go` — the declared state machine (source of truth).
- `pkg/weave/weave_reaper.go` — the pass that fires it.
- `pkg/weave/weave_lifecycle_test.go` — the invariant as assertions.
