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

> **A note on this revision.** An adversarial pass over the first implementation found
> that several of its safety properties were reachable around rather than through. The
> corrections are called out inline as **[revised]**, with the hole each one closes,
> because a design document that quietly presents the fixed version teaches nobody why
> the obvious version was wrong.

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

### Authority classes

Not every entry carries the same weight, and pretending otherwise is how a model's
prose gets mistaken for a fact.

| Kind | Authority | Carries |
|---|---|---|
| `effect` / `observation` | **authoritative** | evidence — something happened in the world |
| `decision` | **authoritative** | a rationale — an explicit, durable record of intent |
| `verification` | **authoritative** | an attestation that somebody went and **checked** an earlier entry |
| `transcript` | **non-authoritative** | an optional hash-linked artifact; nothing derives from it |

Seat events, checkpoints, reconciliations and repairs are a fourth thing: **record
facts** (`Kind.RecordFact`). They are made true *by being written* — the entry does not
describe the acquisition, the entry **is** the acquisition — so there is nowhere to send
an observer to check them. Reconciliation therefore grades world claims and leaves record
facts alone, and they are checked in the ways that actually apply to them instead: the
hash chain on every replay, and independent re-derivation for checkpoints.

> **[revised]** Grading record facts as if they were world claims produced nonsense in
> both directions. Every seat claim carries `success` and points at its own epoch, so the
> host was "degraded" from the moment anyone became steward, forever, with no act
> available to anybody that could clear it — and recording a *clean* reconciliation
> appended an entry that the *next* reconciliation read back as an unverified claim, so
> asking "is everything checked?" is what made the answer "no". A health signal that flips
> because you asked it is not a health signal.

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

### A reference is not a verification — the confidence ladder **[revised]**

The first implementation stopped one step short, and the step it skipped is the one that
matters. It treated *evidence attached to a claim* as establishing the claim, so
`--outcome success -e "command:go test ./..."` projected as **verified**.

But `-e command:go test ./...` records only that somebody **said** they ran the tests. It
does not record that the tests were run, and it certainly does not record that they
passed. An agent can attach a plausible command string, commit hash, or URL to work it
never did **exactly as easily** as to work it did — the reference costs the same either
way. So a reference buys **auditability** and nothing else: it tells a skeptic where to
look. It does not tell them what they will find.

The board therefore has four rungs, and only one act climbs the last one:

| Confidence | What actually happened |
|---|---|
| `unknown` | success claimed with **nothing** to point at |
| `degraded` | an outcome self-declared as degraded |
| `asserted` | success claimed **with references nobody has checked** — the ordinary state of honest work |
| `verified` | a `verification` entry: somebody went and **looked** |
| `refuted` | somebody looked, and the claim was **false** |

`steward verify --seq N --method "<how you checked>"` is the only thing that reaches
`verified`. It binds to the target's **hash**, not merely its sequence number — an
attestation must name the exact bytes it vouched for, or it is an attestation of whatever
ends up at that seq. It demands `--method`, because an unexplained "I verified it" is the
same trust-me claim it was supposed to replace. And it can move a claim **backwards**:
`--result failed` refutes, and the board believes the refutation, because degradation
travels one way.

Most healthy hosts sit at `asserted`, and `steward reconcile` grades that **degraded**
rather than `ok`. That is not pessimism — it is the whole point. Calling a pile of
unchecked references a clean bill of health is precisely how "an agent said so" becomes
"we verified it".

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

### Every authoritative write presents a fencing epoch — and zero is not a value **[revised]**

The first implementation let a write omit the epoch, treating `0` as "whatever tenure I
currently hold". That convenience **was the hole.** An agent that does not know its tenure
ended is *precisely* the agent that will not mention an epoch — so the one caller the
fence exists to stop was the one caller it waved through.

There is now no such shortcut (`ErrNoEpoch`). Every authoritative write presents the epoch
it believes it holds, via `--epoch` or the `$BASHY_STEWARD_EPOCH` that `steward claim`
exports. The gate checks, in this order: the journal is readable, the seat is held, an
epoch was **presented**, that epoch is **current**, and only then, that the actor is the
holder. Checking the epoch *before* identity is what fences the **same logical principal**
holding a stale token: being yourself is not a credential, and a token from a tenure that
ended is stale no matter whose hand it is in.

### Claim vs. takeover

- **`claim`** takes a seat that is **vacant**, or one whose **trustworthy** heartbeat says
  the holder has **lapsed**. The ordinary path. It never negotiates with the incumbent and
  never requires a handoff note: read the journal, decide, write — all under one lock, so
  two agents racing for an empty seat cannot both win.

- **`takeover`** seizes any other seat — live, or unreadable. The recovery path, and
  deliberately the loud one.

#### An unreadable liveness record is NOT claimable **[revised]**

The first implementation treated an *unknown* liveness — no `seat.json`, corrupt file,
wrong holder, wrong epoch — as claimable, reasoning that a missing seat file is what a
crash leaves behind.

It is. But it is also what `rm seat.json` leaves behind, and **from inside the process the
two are indistinguishable.** Deleting one file must not be enough to take a healthy
steward's seat away. Every way of producing an unreadable liveness record is also a way to
produce one *deliberately*.

So the distinction the package now draws is between two very different sentences:

- *"I looked, and the incumbent is late"* — a fact about **the incumbent**. Claimable.
- *"I cannot find or trust the record"* — a fact about **the record**. Not claimable.

Recovering from the second is a **takeover**, which is authorized and recorded. (The
holder's own way out is simply to prove liveness: `steward heartbeat` rebuilds the record
from the journal, which still knows they hold the seat.)

#### Takeover consumes a capability **[revised]**

`--authorized-by <name>` was a **string**. An agent that wanted the seat could type one.

Takeover now consumes a **grant** (`steward authorize`), which is single-use, expiring, and
bound to one grantee and one epoch:

| Bound to | Why |
|---|---|
| a **nonce** | the takeover that spends it names it **in the journal**, so replay refuses a second use — even if the grant file is restored from a backup |
| a **grantee** | not a coupon somebody else can pick up |
| **one epoch** | it authorizes seizing the seat **as it stands now**; if the seat moves on, the grant is dead |
| an **expiry** | 15 minutes by default; a capability that outlives its situation is a backdoor with a nice name |

**Be honest about what this proves — the answer is less than it looks.** This package runs
as you, on your machine, with your filesystem. Anything else running as you — *including
the agent it is meant to restrain* — can write these same files. There is no signature to
check and no second party to ask, so an `--actor` is an **assertion**, not a credential,
and the journal labels it as one forever. What the capability actually buys is that
seizing the seat becomes a separate, deliberate, expiring, single-use act that names a
human and a reason and lands in the permanent record where somebody will see it. That
stops the *ordinary* failure — an agent deciding on its own that the steward looks stuck —
and makes the *extraordinary* one leave fingerprints. It is a real control. It is not
cryptographic proof that a human was in the room, and this package will never claim it is.

Which is why an **unattended** takeover (no terminal) demands a grant carrying an
**external receipt**: the bytes of an out-of-band approval — a PR review, a ticket, a page
ack — copied into the store and pinned by digest. With nobody present, "a human approved
this" is a sentence with no author; a receipt is an artifact somebody can go and audit.

> **[revised]** "Is anyone at the terminal?" was answered with the usual
> `os.ModeCharDevice` shortcut — **which is true for `/dev/null`**, and `/dev/null` is
> exactly how a daemon, a cron job, a CI runner, and a headless agent get their stdin.
> The most unattended processes on the host were being judged the most attended, and could
> spend an operator *assertion* on an unattended seizure. The check now asks the OS whether
> the descriptor is a terminal (`golang.org/x/term`, already a direct dependency), and
> `TestDevNullIsNotATerminal` pins it.

It never asks the incumbent — an incumbent that could be asked would not need to be taken
over — and an unexplained seizure of authority is indistinguishable from a hijack, so the
capability it was performed under lives in the hash-chained journal, not in a status file
a crash could take with it.

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
- **Repair is explicit and human-invoked** (`steward repair`, `--plan` to see it first).

Repair is deliberately not automatic: a log that silently healed itself would be worthless,
since "it repaired itself" and "someone tampered with it" would look identical.

#### What a repair may touch, and what it must refuse **[revised]**

`steward repair` fixes exactly **one** kind of damage: a **torn final append**. That is what
a crash actually leaves — the process died partway through the last line, so the file ends
with an incomplete fragment and **no terminating newline**. Nothing completed is in those
bytes, by definition, since a completed append is fsynced *with* its newline.

**Everything else fails closed** (`ErrNotRepairable`), and the two refusals are the point:

- **Mid-log damage.** If complete lines *follow* the unreadable region, whatever is after it
  was fully written. Truncating from the damage point would destroy completed records.
  (Detected by a newline in the discarded suffix.)
- **A complete record that does not chain.** A parseable entry whose hash, `prev_hash`, seq,
  or epoch is wrong is *not* a torn write. It is a record that was **altered**, or one
  written around a record that was **removed** — the signature of tampering. A tool that
  silently truncated that away would be the attacker's best friend: it would delete the
  evidence and call it a repair.

> *A repair that can only ever remove garbage is a repair. A repair that can remove data is
> a data-loss tool with a reassuring name.*

And in order, every repair: **authorizes** (the holder, at the current epoch — a damaged
journal is not a licence for a stranger to truncate the host's record), **quarantines** the
exact discarded bytes by digest *before* truncating ("the tool ate it" is not an answer to
"what was in those bytes?"), truncates at the last byte that verified, and writes a
**degraded receipt**. The first implementation wrote that receipt with `_, _ = s.Record(…)`
— so a failed receipt left a **silently shortened journal with nothing in it saying so**,
which is the single worst outcome available to this package. A receipt that cannot be
written is now an **error**, and it says exactly what state the store is in.

### Reconcile is allowed to say "I don't know"

`steward reconcile` is the verb a successor runs **first**, before touching anything.

| Verdict | Meaning |
|---|---|
| `ok` | the journal is intact and every world claim in it has been **checked** |
| `degraded` | the record is readable, but something in it could not be established |
| `unknown` | the **record itself** is damaged; what survives is valid, what came after cannot be spoken for |

There is deliberately **no `failed`**. The subsystem never reports success in the face of
missing evidence — and it never invents a failure it cannot prove either.

Reconcile also **still reports without the seat**, and that is deliberate: it is the verb
you run *before* you hold anything. Failing to journal the report (no seat, no epoch, a
fenced epoch) prints the report anyway and says why it was not written down. Refusing to
print the truth because you could not also record it would break the one command a cold
successor needs, in exactly the situation it exists for.

#### It does not claim to have checked reality **[revised]**

The first implementation's reconcile reported that it had *"compared the journal against
reality"* while comparing the journal against **nothing but itself**: it re-read its own
entries, noticed which ones lacked evidence, and called that a reality check. That is not a
reality check. It is a **spellcheck**.

The core package is generic and knows nothing about git, CI, GitHub, or any other world an
entry might make claims about — it *cannot*, since a host-scoped journal spans every project
on the machine, and baking in every checker would make this package the union of every tool
it records. So it takes **adapters** (`Observer`): a host supplies things that know how to go
and look — *did that commit actually land on main? did that CI run go green? is that service
actually up?* — and reconciliation reports what they **found**.

With no adapter, `RealityCompared` is `false` and the report says so, in prose, out loud:

> *NOTHING was compared against reality: no observation adapter was supplied. Every claim in
> this report stands exactly as the agent that made it left it.*

An observation is what an adapter **found**; it becomes part of the record only when a
steward records it as a verification. A reconciliation that always produced a clean verdict
would be worthless. The only useful thing it can do is tell you precisely where the record
runs out — the difference between inheriting a *system* and inheriting a *story about* a
system.

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

- **A platform with no locking fails every mutation CLOSED.** **[revised]** The first
  implementation shipped a **no-op lock** on other platforms (`lock_other.go`), with an
  apology in the comment — and the apology is what gives it away. *A lock that silently does
  nothing is worse than no lock at all*, because the caller believes the read/decide/write
  cycle is serialized. It is not: two agents interleave, both replay a vacant seat, both
  append a claim, and the host now has two stewards that each believe they are the only one
  — the exact failure the singleton exists to prevent, produced by the mechanism meant to
  enforce it.

  The fencing epoch does **not** save this, either, which is what the old note got wrong:
  both claims mint their epoch from the same replayed head, so they **collide** rather than
  supersede. Neither steward is fenced, because neither one's token is stale.

  So the seat fails closed (`ErrLockUnsupported`): a platform that cannot serialize cannot
  host a steward, and saying so is the only honest option. **Reads still work** — they never
  take the lock. `TestUnsupportedLockFailsEveryMutationClosed` pins that every mutation
  refuses.

- **Schema-versioned artifacts.** Every journal entry, seat file, grant, checkpoint, board,
  and reconciliation carries `bashy-steward-v1`. A **mismatch is never tolerated** on the
  seat cache or a grant: a record this package cannot fully understand is not one it may act
  on.

---

## Layout

```
$BASHY_STEWARD_DIR (default ~/.bashy/steward)
├── journal.jsonl        the ONLY authority — append-only, hash-chained
├── seat.json            liveness cache ONLY (delete it; authority survives)
├── steward.lock         the acquisition lock
├── checkpoints/         materialized projections (caches with receipts)
├── grants/              takeover capabilities (the JOURNAL records their consumption)
├── receipts/            external approval artifacts, pinned by digest
├── quarantine/          bytes a repair discarded — kept, never destroyed
└── transcripts/         optional, non-authoritative artifacts
```

Host-wide and **cwd-independent**, deliberately. A steward is not a property of a checkout —
it is the human's continuous point of contact across every project on the machine. Keying it
to a repository would produce one steward per clone, which is exactly what the singleton is
meant to prevent.

---

## CLI

```
READ (no seat required — reconcile is what you run BEFORE you hold anything)
steward status                 who holds the seat, are they alive, what does the board say
steward scope                  which host/user seat this is, and where it lives
steward board [workstream]     the workstreams, and which outcomes are actually established
steward log [--degraded] [--kind K] [--workstream W] [--actor A] [--since T] [--follow]
                               the journal itself, chronologically
steward conversation           the decisions, and how the room got there
steward history                how the seat changed hands; checkpoints along the way
steward grants                 the capabilities, and whether they can still be used
steward reconcile [--record]   what can and cannot be established
steward repair [--plan]        truncate a TORN FINAL APPEND — and nothing else

SEAT
steward claim | take [--intent W] [--export]
                               acquire a VACANT or LAPSED seat (atomic); exports the epoch
steward authorize --actor <who> [--reason W] [--receipt F --receipt-issuer S] [--ttl D]
                               mint a single-use, expiring takeover capability
steward takeover --grant <id>  seize the seat; bumps the epoch, fences the prior holder
steward release [--note W]     vacate cleanly (captures NO repository state)
steward heartbeat              refresh liveness (writes no journal entry)

WRITE (all fenced: --epoch, or the $BASHY_STEWARD_EPOCH that `claim` exports)
steward record -m <what> [--outcome …] [-e kind:ref …] [--observation]
steward decide -m <what> --rationale <why>
steward verify --seq N --method <how> [--result success|failed|unknown]
steward checkpoint [--note W] | [--verify ID] | [--list]
steward transcript -m <what>                 (non-authoritative; nothing derives from it)
steward workstream open|update|close <name>
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

reality:  NOTHING was compared against reality: no observation adapter was supplied.
          Every claim in this report stands exactly as the agent that made it left it.

UNPROVEN — 2 claim(s) you must not take on faith:
  seq 39   [api] claimed success, effective unknown
           migrated the schema and verified the cutover
           why: claimed success with no evidence — a claim nobody can check is not a fact

ASSERTED, NEVER CHECKED — 3 claim(s) resting on references nobody verified:
  seq 41   [web] shipped the rollout
           verify it: `steward verify --seq 41 --result <success|failed> --method <how>`
  …

$ bashy steward claim --intent "picking up after a lapse"
claimed the steward seat: claude-c3d4 at epoch 4
  the lapsed seat was held by claude-a1b2 — they are now FENCED at the old epoch

  export BASHY_STEWARD_EPOCH=4
```

If `claude-a1b2` comes back, its next write is rejected with `ErrFenced` — loudly, naming
the epoch it presented and the epoch the world is at, and telling it to re-read the journal
before it does anything else.

Had the heartbeat been *missing* rather than *late*, `claim` would have refused
(`ErrLivenessUnknown`) and pointed at `steward authorize` → `steward takeover`: a seat you
cannot read is not a seat you may quietly take.

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

These are gaps, stated as gaps. None of them is quietly papered over in the code.

- **Authorization is not cryptographic.** The largest limitation, and it is structural rather
  than an oversight: this package runs as the user, so anything else running as the user —
  including the agent it restrains — can write its files directly. A grant is durable,
  replay-protected, expiring and auditable; it is **not** proof a human was in the room, and
  it is labelled `operator-assertion` precisely so nobody downstream mistakes it for one.
  Closing this properly needs a signer the agent does not control (an OS keychain, a hardware
  token, a second machine) — a real design, not a flag.
- **`Interactive` is the host's assertion.** A caller can set it. It buys the unattended
  path's receipt requirement, not a proof of attendance.
- **No `Observer` adapters ship here.** The core is generic by necessity, so out of the box
  `steward reconcile` compares the journal against *nothing* — and says so, in prose, rather
  than implying otherwise. The git/CI adapters belong in the hosts that know those worlds.
- **`--follow` is polling**, not `inotify`. Deliberate: it keeps follow a pure function of the
  journal (replay, skip below the watermark) rather than a second, event-driven code path that
  could disagree with a plain `steward log` — and it makes follow testable without sleeping
  around a filesystem race.
- **No `steward transfer` verb.** Release + claim already covers a planned handover of the seat,
  and the seat is recoverable without either. A dedicated `transfer` would only add a
  cooperative path to a system whose entire premise is that the incumbent may never cooperate.
- **Platforms with no file locking cannot host a seat at all** (`ErrLockUnsupported`). Mutations
  fail closed; reads still work. This is a deliberate *reduction* in capability from the first
  implementation, which pretended to lock and did not — see the durability section for why the
  fencing epoch does not rescue that case.
