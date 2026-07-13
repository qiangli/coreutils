# The steward seat

`pkg/steward` is the host/user-scoped seat of **authority and continuity**: exactly
one steward per machine-and-account, holding an append-only, evidence-carrying
journal that outlives whoever holds the seat.

It implements the design agreed in the 2026-07-13 three-party meeting. This document
is the self-contained record of what was decided and why; it deliberately does not
depend on any umbrella-private material.

---

## What problem this solves

An agentic host accumulates two kinds of loss, and they are different:

1. **Lost work** — a session dies with an uncommitted diff in the tree. That is
   `pkg/handoff`'s problem, and it is task- and artifact-scoped: capture the working
   tree, restore it into a successor's checkout.

2. **Lost authority and lost truth** — the agent that has been answering for this
   machine vanishes, and with it goes the only account of what was actually done,
   what was decided, and what was merely *claimed*. Nobody can say who is in charge,
   and nobody can distinguish "we verified that" from "an agent said so."

The second is what `pkg/steward` addresses, and it is the harder one, because the
incumbent **never gets to say goodbye**. A steward that crashed, hit a rate limit, or
was simply killed leaves no handoff note. Continuity has to work anyway.

### The two are not the same verb

This distinction is load-bearing and is enforced by a test
(`TestSeatLifecycleTouchesNoRepository`):

|                | `bashy handoff`         | `bashy steward`               |
| -------------- | ----------------------- | ----------------------------- |
| Scope          | one task, one project   | the whole host/user           |
| Moves          | **work** — a diff, a tree | **a mandate** — the seat      |
| Touches a repo | yes, that is the point  | **never**                     |
| Needs the predecessor | yes — it writes the note | **no**                   |

Claiming the steward seat restores no working tree, captures no diff, and touches no
repository. Conflating the two is what made "hand off your work" ambiguous in the
first place: **WORK is a diff, a SEAT is a mandate**, and only one of them should ever
mutate your repo.

---

## The design, and the reasoning behind each part

### One journal. Everything else is a projection.

A single append-only, hash-chained journal (`journal.jsonl`) is the **only** authority.
The board, status, log, conversation, history, and checkpoints are all **read-only
projections** derived by replaying it.

This is not tidiness — it is the fix for the most common way a state machine rots. The
moment a cached view becomes a *writable* second truth, it starts to drift, and the
first time it disagrees with the log nobody can say which one is wrong. A projection
has no state of its own, so it structurally cannot drift.

Concretely: `ProjectBoard(entries)` is a pure function. Same entries → same board →
same digest, on any host, in any process.

### Three authority classes

Not every entry carries the same weight, and pretending otherwise is how a model's
prose gets mistaken for a fact.

| Kind | Authority | Carries |
|---|---|---|
| `effect` / `observation` | **authoritative** | evidence — something happened in the world |
| `decision` | **authoritative** | a rationale — an explicit, durable record of intent |
| `transcript` | **non-authoritative** | an optional hash-linked artifact; nothing derives from it |

**Transcripts are optional by contract.** Delete every transcript artifact on the host
and the board, the status, the history, and every checkpoint must be *bit-identical*.
`TestTranscriptDeletionDoesNotAffectProjections` pins this, precisely so it cannot
quietly stop being true. A decision record is what *binds*; a transcript merely lets a
human go back and see how the room got there.

### Missing evidence yields unknown — never success

The single most load-bearing line in the package:

```go
func (e Entry) EffectiveOutcome() Outcome {
    if e.Outcome == OutcomeSuccess && !e.HasEvidence() {
        return OutcomeUnknown
    }
    return e.Outcome
}
```

An entry claiming success with nothing to point at **does not project as success**. It
projects as `unknown`, on every board, in every checkpoint, forever. The journal still
records the claim faithfully — it is an honest record of what was *asserted* — but no
view will ever promote an unevidenced assertion into a fact.

An LLM writes fluent, confident prose about work it did not do. The only defense that
scales is to refuse to launder an unevidenced claim into a fact.

**Degradation travels one way only.** A *failure* without evidence stays a failure. We
never upgrade toward the happy path: the cost of a false "success" is unbounded, and
the cost of a false "failed" is a second look.

Consequently `closed` and `verified done` remain different facts. A workstream closed
with an unevidenced success is closed **and** unknown, and the board says so.

### Authority vs. liveness — and why the split is the whole trick

```
AUTHORITY (who holds the seat, at which epoch)  ← derived from the JOURNAL
LIVENESS  (is the holder still breathing)       ← from seat.json's heartbeat
```

Authority is **recoverable by replay alone**. Delete `seat.json` entirely and the holder
and epoch survive; only liveness is lost, and it honestly degrades to `unknown` rather
than inventing a death. This is what makes crash recovery work with no handoff note, no
goodbye, and no cooperation from the incumbent.

### A stale heartbeat proves only a liveness lapse

This is the signal every lease system misreads. "The heartbeat is old" gets treated as
"the holder is dead" — and then a returning incumbent, which was merely throttled or
mid-thought, silently corrupts the record.

This package never makes that claim. A lapse is a **lapse**: the holder may be
mid-thought, rate-limited, paused at a human prompt, or on a bad network, and may come
back at any moment.

That is *precisely why the epoch exists*.

### The fencing epoch

A successor claiming an expired seat bumps a **monotonically increasing epoch**. The
returning incumbent — still holding the *old* epoch — is **fenced**: its mutations are
rejected, loudly (`ErrFenced`), instead of silently interleaving with the new steward's.

So a lapsed incumbent coming back is not a bug. It is *expected*, and the fence is what
makes it harmless.

Two details that are easy to get wrong, and are pinned by tests:

- **The epoch is checked before identity.** A returning zombie is, by then, no longer
  the holder — so an identity check would reject it as a mere stranger (`ErrNotHolder`)
  and never tell it the one thing it needs to know: *your tenure ended, the world moved
  on, re-read the journal.* Both errors refuse the write, so safety is identical — but
  only one of them explains a zombie to itself, and an agent that misreads "you are not
  the holder" as "I should just claim the seat again" will happily overwrite the steward
  that replaced it.

- **The epoch never descends.** A release does not reset it. An epoch that could go
  backwards would let a fenced holder un-fence itself simply by waiting.

### Claim vs. takeover — and why takeover needs a human

- **`claim`** takes a **vacant or lapsed** seat. The ordinary path. It never negotiates
  with the incumbent and never requires a handoff note: read the journal, decide, write
  — all under one lock, so two agents racing for an empty seat cannot both win.

- **`takeover`** seizes a **live** seat. The emergency path. It requires explicit human
  authorization (`--authorized-by`) and records who authorized it, from whom, and why.

Takeover is gated on a human because *an agent that could decide on its own to take over
would eventually decide to do it to a healthy steward*. It never asks the incumbent —
an incumbent that could be asked would not need to be taken over.

An unexplained seizure of authority is indistinguishable from a hijack, so the
authorization lives in the hash-chained journal, not in a status file a crash could take
with it.

### Checkpoints are caches with receipts

A checkpoint carries the **watermark** it projects and the **chain digest** at that
watermark. That makes it *verifiable* rather than merely trusted: re-project the journal
at the same watermark and compare.

- Same entries, same watermark → same board, always. No clock, no randomness, no ambient
  state leaks into the projection.
- Appending to the journal does **not** invalidate an old checkpoint — the watermark pins
  the history it projected.
- If a checkpoint stops re-deriving, the journal beneath it changed. Given the hash chain,
  that means *someone rewrote history*, which is worth finding out about.
- Delete every checkpoint file and you have lost nothing but the recompute time. The
  *file* is the cache; the journal entry recording that a checkpoint was taken is the
  memory.

The tempting alternative — a checkpoint you can *edit*, that accumulates state the journal
never saw — produces an artifact that is faster to read and impossible to trust. This
package structurally cannot do that.

### Corrupt tails: tolerated on read, refused on write

A crash mid-append can leave a torn final line. Replay walks the journal and returns the
**valid prefix**, plus an honest account of what it could not read.

- **Reads carry on.** A journal whose last 40 bytes are garbage still has a perfectly good
  history before the tear, and that history is exactly what a successor needs. Refusing to
  read it would turn a survivable crash into total amnesia — the precise failure this
  subsystem exists to prevent.
- **Writes refuse** (`ErrCorruptTail`) rather than forking the chain around the damage. The
  error states how many valid entries survive, so an operator learns immediately that a
  repair costs them nothing but the torn tail.
- **Repair is explicit and human-invoked** (`steward reconcile --repair-tail`). It truncates
  at the byte just past the last entry that verified, so a valid entry can never be removed.
  It is recorded as a *degraded* reconcile, because data was lost.

Repair is deliberately not automatic: a log that silently healed itself would be worthless,
since "it repaired itself" and "someone tampered with it" would look identical.

### Reconcile is allowed to say "I don't know"

`steward reconcile` is the verb a successor runs **first**, before touching anything.

| Verdict | Meaning |
|---|---|
| `ok` | the journal is intact and every claim in it is established |
| `degraded` | the record is readable, but something in it could not be established |
| `unknown` | the **record itself** is damaged; what survives is valid, what came after cannot be spoken for |

There is deliberately **no `failed`**. The subsystem never reports success in the face of
missing evidence — and it never invents a failure it cannot prove either.

A reconciliation that always produced a clean verdict would be worthless. The only useful
thing it can do is tell you precisely where the record runs out. That is the difference
between inheriting a *system* and inheriting a *story about* a system.

---

## Durability and concurrency

- **Atomic, durable writes.** Temp file → `fsync` the data → `rename` → `fsync` the
  directory. A rename that lands while the contents are still in the page cache can leave a
  correctly-named *empty* file after a crash, so the data fsync is not optional. Journal
  appends are `O_APPEND` + `fsync`: the journal is the only authority there is, and if it can
  lose a write, everything derived from it is a guess.

- **Serialized read/decide/write.** Every acquisition runs the whole read → decide → write
  cycle under one exclusive file lock. This is essential exactly here: `Claim` must *read*
  the journal, decide the seat is free, and *write* its claim — and if two agents interleave
  those three steps, both conclude the seat is vacant and both take it. That is the race the
  singleton contract exists to forbid, reproduced inside the mechanism meant to enforce it.

- **Real locks on every shipped platform.** `flock` on Linux/macOS, **`LockFileEx` on
  Windows**. The older claim registry (`pkg/policy/coord`) documents an honest Windows gap —
  no lock, so two agents can both conclude a project is free — and the steward seat cannot
  afford to inherit that apology: *a singleton enforced by a racy acquisition is not a
  singleton*. `pkg/policy/audit` had already proved `LockFileEx` works for this.

  On any *other* platform (`lock_other.go`) there is no mutual exclusion, and this is stated
  plainly rather than pretended. What still holds there is the fencing epoch: the second
  claim wins the journal, the first holder's epoch is superseded, and its next mutation is
  rejected — so a lost acquisition race degrades into a *detected* fencing error rather than
  two silent stewards.

- **Schema-versioned artifacts.** Every journal entry, seat file, checkpoint, board, and
  reconciliation carries `bashy-steward-v1`.

---

## Layout

```
$BASHY_STEWARD_DIR (default ~/.bashy/steward)
├── journal.jsonl        the ONLY authority — append-only, hash-chained
├── seat.json            liveness cache ONLY (delete it; authority survives)
├── steward.lock         the acquisition lock
├── checkpoints/         materialized projections (caches with receipts)
└── transcripts/         optional, non-authoritative artifacts
```

Host-wide and **cwd-independent**, deliberately. A steward is not a property of a checkout —
it is the human's continuous point of contact across every project on the machine. Keying it
to a repository would produce one steward per clone, which is exactly what the singleton is
meant to prevent.

---

## CLI

```
steward status                 who holds the seat, are they alive, what does the board say
steward board [workstream]     the workstreams, and which outcomes are actually established
steward log [--degraded] [--follow] [--json]
                               the journal itself, chronologically
steward conversation           the decisions, and how the room got there
steward history                how the seat changed hands; checkpoints along the way
steward checkpoint [--verify ID] [--list]
                               materialize / verify a reproducible projection
steward reconcile [--record] [--repair-tail]
                               what can and cannot be established

steward claim | take           acquire a vacant or lapsed seat (atomic)
steward takeover --authorized-by <human> --reason <why>
                               seize a LIVE seat; bumps the epoch, fences the prior holder
steward release                vacate cleanly (captures NO repository state)
steward heartbeat              refresh liveness (writes no journal entry)

steward record -m <what> [--outcome …] [-e kind:ref …]
steward decide -m <what> --rationale <why>
steward transcript -m <what> [--file F]
steward workstream open|close <name>
```

`--json` is available on every verb and carries the schema version.

`--degraded` is the query a successor needs first: *what do I not actually know?*

### The shape of a recovery

```console
$ bashy steward status
seat: claude-a1b2 (epoch 3) — lapsed
  heartbeat: 2026-07-13T09:14:22Z (2h11m ago — LAPSED, which proves a lapse and nothing more:
             they may be mid-thought, throttled, or coming back. Claiming FENCES them, safely.)
  journal:   47 entries, intact

$ bashy steward reconcile
reconciliation: DEGRADED

seat:     claude-a1b2 (epoch 3) — lapsed
journal:  47 entries, intact (head sha256:9f…)
board:    4 workstream(s)

UNPROVEN — 2 claim(s) you must not take on faith:
  seq 39   [api] claimed success, effective unknown
           migrated the schema and verified the cutover
           why: claimed success with no evidence — a claim nobody can check is not a fact
  …

$ bashy steward claim --intent "picking up after a lapse"
claimed the steward seat: claude-c3d4 at epoch 4
  the lapsed seat was held by claude-a1b2 — they are now FENCED at the old epoch
```

If `claude-a1b2` comes back, its next write is rejected with `ErrFenced` — loudly, naming
the epoch it presented and the epoch the world is at, and telling it to re-read the journal
before it does anything else.

---

## Relationship to the rest of the AgentOS hub

- **`pkg/handoff`** stays exactly as it is: task/artifact-scoped, repository-touching. The
  steward seat does not replace it, does not restore working trees, and does not capture
  diffs. They compose — a steward may well hand a task off with `bashy handoff`.
- **`pkg/policy/coord`** remains the per-*project* claim registry (one agent per repo). The
  steward seat is per-*host*, and reuses coord's identity rule (`coord.Self`) so a steward, a
  claim, and an audit record all name the same agent the same way. It does **not** reuse
  coord's lock, because coord's Windows no-op is a gap a singleton cannot tolerate.
- **`pkg/principal`** supplies the identity (`principal.Ref`). Holders are compared by
  episode-or-(name, host), never by PID: one logical agent runs many processes (a shell, a
  subagent, a hook), and none of them should be told it is colliding with itself.

---

## Known gaps

- **`--follow` is polling**, not `inotify`. Deliberate: it keeps follow a pure function of the
  journal (replay, skip below the watermark) rather than a second, event-driven code path that
  could disagree with a plain `steward log` — and it makes follow testable without sleeping
  around a filesystem race.
- **No `steward transfer` verb.** Release + claim already covers a planned handover of the seat,
  and the seat is recoverable without either. A dedicated `transfer` would only add a
  cooperative path to a system whose entire premise is that the incumbent may never cooperate.
- **Non-Unix, non-Windows platforms have no acquisition lock** (see `lock_other.go`). The
  fencing epoch still degrades this into a detected error rather than silent dual authority.
