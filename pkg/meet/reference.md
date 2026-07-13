# bashy meet — reference

A **multi-participant deliberation session**: several agentic CLIs (claude, codex,
opencode, agy, aider, …) plus a human take turns on a topic, and a dedicated
**notes-only secretary** keeps the minutes, extracts decisions/action-items, and
files the result. It is the *deliberative* front half of the fleet; `weave` /
`conductor` / `sdlc` are the *executive* back half — decide in `meet`, execute there.

`meet` is also **callable by an agent**: a coding agent that wants a cross-vendor
second opinion mid-task runs `bashy meet consult` and reads back a verdict.

## Roles — three, and the separation between them is the design

| Role | Decides | Filled by |
|---|---|---|
| **participant** | **content** — argues, proposes, votes | agents, 1..N |
| **chair** | **process** — poses the agenda, calls on speakers, judges done-ness. Never argues. | an agent (`--chair`), the human, or nobody |
| **secretary** | **nothing** — records, and extracts what was decided | exactly one agent (`--secretary`, default `claude`) |

**These are enforced, not merely documented.** `meet` refuses to start a meeting
where the secretary is also a participant (a recorder with a stake in the record),
where the secretary is also the chair (it could declare the meeting over and then
author the minutes saying so), or where the chair is also a participant (it would
pick itself). A participant seated twice is refused too — a duplicate seat dilutes
a vote and adds no diversity.

Diversity of tool/model among participants is the value (2–4 near-equal proposers is
the sweet spot; more dilutes signal — the Self-MoA guard).

**Initiator** is not a fourth role. It is an *attribute*: whoever convened the
meeting, named among the people already at the table. *Only the initiator may end
it* (see `close`).

## CLI

```
bashy meet start   --topic TEXT [--participant AGENT ...] [--chair AGENT] [flags]
bashy meet consult --topic TEXT --question TEXT [--choice yes --choice no] [--json]

bashy meet tell          <id> "<text>"          # append a human contribution
bashy meet round         <id>                   # one moderated round across participants
bashy meet poll          <id> --question TEXT   # fixed-choice ballot; every seat must answer
bashy meet ask           <id> --question TEXT   # open question; silence = "no comment"

bashy meet list                                 # every meeting; ROOM is the short handle
bashy meet observe       [<room>|<id>]          # attach and WATCH it happen, live (read-only)

bashy meet show          <id>                   # roster, per-participant coverage, artifacts
bashy meet contributions <id> [participant]     # every contribution, in full
bashy meet converge      <id>                   # secretary pass (rewrites synthesis.json)
bashy meet close         <id>                   # converge + initiator confirms + file minutes
bashy meet amend         <id> [--resynthesize]  # regenerate the minutes from the transcript
bashy meet apply         <id> --to FILE --write # append the agreed action items to a doc

bashy meet list | resume <id> | reference
```

Session flags (`start`, `consult`): `--topic` (required) · `--participant`
(repeatable) · `--secretary` (default `claude`) · `--chair AGENT` (optional) ·
`--agenda` (repeatable) · `--context FILE` (repeatable — every participant reads the
same source set) · `--turn-timeout` (default 20m) · `--decision-mode infer|explicit` ·
`--min-turn-chars N` · `--initiator NAME` · `--max-turns` · `--max-stalls` ·
`--out docs|kb|<path>`.

There is **no `--mode` flag.** The turn model is a consequence of who chairs.

`start` adds `--rounds N --non-interactive` (run then auto-close), `--dry-run`
(print the resolved session + attendee gate, launch nothing), and `--yes`.

In the REPL: plain text = a human turn · `@name <text>` = a targeted turn · `/round` ·
`/chair` · `/poll <q>` · `/ask <q>` · `/decision <t>` · `/action owner: task` ·
`/agenda <t>` · `/show` · `/converge` · `/close`.

## Watching a meeting: `meet observe`

A meeting runs unattended for a long time. `observe` lets you look in on one
without joining it — you attend, you do not participate.

```
bashy meet list                 # ROOM  ID  STATUS  PARTICIPANTS  TOPIC
bashy meet observe 2            # attach to room 2
bashy meet observe              # attach to the most recent open meeting
bashy meet observe 2 --participant Sable    # watch one seat
bashy meet observe 2 --json | jq -r 'select(.kind=="decision") | .text'
```

**Rooms.** A meeting id is a space-time coordinate — unique forever, and far too
long to retype. A **room** is the small number you actually say: room 2. Rooms
work like a shell's job numbers — assigned from the lowest free number among the
*open* meetings, and reused once a meeting closes. A room is a pointer, resolved
when you type it and **never written into a record**: a transcript claiming
"room 2 decided X" would rot the moment room 2 was reused. `observe` also takes
the full id, or any unambiguous prefix of it.

**What you see.** The whole history first, in full — you are joining a
conversation already in progress and need to know what was said. After that you
are watching it live: each agent's answer streams in **line by line as it writes
it**, not all at once, minutes later, when the turn completes.

Granularity is a *line*, not a token. The agent CLIs emit complete lines on
stdout; there is no token channel to subscribe to without going around the
harness straight to a provider API, which would abandon the tools, the sandbox,
and the shell-forcing that make the harness worth having. A line is the finest
grain honestly available.

**Observing is read-only.** It takes no seat, casts no vote, and writes nothing.
Any number of observers can attach to one meeting, and attaching can never
change what the meeting decides. `^C` detaches you; the meeting keeps running.

**Two channels, one truth.** The meeting writes `transcript.jsonl` — the RECORD,
one sanitized event per completed turn, from which the minutes are built and
which is replayed as context to the next agent — and `live.jsonl`, the VIEW:
ephemeral, line-granular, and safe to lose, because everything it carried also
lands, whole, in the transcript. The live channel is a *tee* of the agent's
stdout, so it can never show a watcher something the record will not contain.

## The turn model follows from who chairs

There is no mode flag, because there is nothing a mode flag could say that the
roster does not already say.

**No `--chair`** → strict round-robin: every participant speaks, once, per round.
The human directs (`/round`), or nobody does (`--non-interactive`, `consult`).
Simple and cheap, but rigid — it cannot skip a participant with nothing to add,
cannot call one twice mid-argument, and **cannot notice the meeting is going in
circles**. Step repetition is the largest measured failure mode in multi-agent
systems (~17% of failures across 1600+ traces).

**`--chair <agent>`** → that agent directs. Before each turn it answers five
questions at once — speaker selection is only two of them:

```
SATISFIED: yes|no        has the agenda been fully addressed?
LOOPING: yes|no          are participants repeating points already made?
PROGRESSING: yes|no      did the last turn add something new?
NEXT: <participant>
INSTRUCTION: <what that participant should address>
REASON: <why>
```

Every ledger is recorded, so the whole chain of "who was called on and why" is in
the transcript and the minutes. Three defenses, each against a documented failure:

- **Selector validation ladder.** An LLM asked to name an agent routinely names
  none, one that does not exist, or several. `meet` parses the pick, validates it
  against the roster by **exact** match (loose matching is how `code-reviewer`
  silently becomes `reviewer`), re-prompts with the specific error, and after three
  attempts **degrades to a default speaker and marks the ledger `degraded`**. A
  fallback that hides itself looks like a working selector.
- **Stall counter.** `LOOPING` or `not PROGRESSING` for `--max-stalls` consecutive
  turns triggers a **re-plan**: the chair is asked for a new approach instead of
  calling on another participant to repeat the loop. A second exhausted run of
  stalls stops the meeting rather than spinning. Stall detection runs *before*
  dispatch — that is the whole point.
- **Hard backstops.** `--max-turns` and the caller's deadline always apply.
  `SATISFIED` is **advisory**: an agent claiming completion cannot extend the
  budget, and one that never claims it cannot run forever. Termination belongs to
  the orchestrator, never to a token a model emits — models both fail to stop and
  stop early, at measurable rates.

The chair is a **distinct seat from the secretary**, and `meet` will not let one
agent hold both — see Roles above.

```bash
bashy meet start --topic "…" \
  --participant codex --participant opencode \
  --chair claude --secretary gemini \
  --max-turns 12 --max-stalls 3 --non-interactive
```

A chaired run reports how it ended:

```
chaired: 6 turns, 2 stalls, 1 re-plans, 0 degraded selections — stopped by satisfied
```

`stopped_by` is `satisfied` (the chair's own call), or `max_turns` / `stalled` /
`deadline` (a backstop fired). Only `satisfied` means the meeting finished.

## Turn styles

| Style | Verb | Answer | Silence means |
|---|---|---|---|
| Free-form round | `round` | prose | a failed turn |
| **Poll** (request for comment) | `poll --choice …` | exactly one choice | a failed turn |
| **Open question** | `ask` | prose, optional | **"no comment"** — an abstention, not a failure |

A poll defaults to a `yes`/`no` ballot. A reply that answers but names no choice —
or names two — is recorded `invalid` and **excluded from the tally**, never guessed
at. A poll with a tie, or whose plurality does not clear the non-answers, reports
*no clear result* rather than a winner.

`ask --required` turns an open question back into a mandatory one.

## Agents convening meetings: `meet consult`

The one-shot, blocking, agent-callable form. It convenes the panel, runs the rounds,
optionally polls, synthesizes, files the minutes, and prints a verdict — no REPL, no
confirmation round-trip (the caller *is* the initiator and receives the verdict
synchronously).

```bash
bashy meet consult \
  --topic "Should cert mode bypass the atomizer?" \
  --question "Should --profile cert reject --chunks entirely?" \
  --choice yes --choice no \
  --participant codex --participant claude \
  --context docs/bashy-posix-fleet-implementation-plan.md \
  --deadline 10m --json
```

`--json` emits the machine-readable `Verdict`: `verdict`, `confidence`, `exit_code`,
`summary`, `decisions` (each flagged `inferred` with its `support`), `actions`,
`risks` (the blocking issues), `open_questions`, `corrections`, the `poll` tally,
per-participant `coverage`, and the `minutes` path.

**Read `verdict` before you act on `summary`.** Disagreement is a result, not an
error:

| `verdict` | `exit_code` | Meaning |
|---|---|---|
| `agree` | 0 | decisive, every seat answered, no blocking issues — act on it |
| `agree` | 1 | decided, but a participant raised a blocking issue in `risks` |
| `split` | 2 | the panel genuinely disagreed — you decide |
| `escalate` | 2 | a seat failed, or nothing was decided — **do not act on this alone** |

`confidence` is a statistic, not a self-report: `coverage × vote-share`. A panel where
half the seats crashed can never be `agree`, however loudly the surviving half agreed.
Models are known to be badly calibrated about their own certainty, so `meet` never
asks them. Pass `--fail-on-dissent` to make the process exit non-zero unless the
verdict is a clean `agree`.

**Recursion is refused.** `meet` stamps `BASHY_MEET_DEPTH` into every agent it
spawns, and convening a meeting from inside one is an error. A panelist that wants a
second opinion must say so in its turn and let the chair decide — otherwise panels
fork panels, unboundedly (4 agents deep is 4ⁿ processes).

**Cost and latency.** Convening N agents for R rounds is N×(R+1) CLI invocations.
`--deadline` (default `10m`) is a hard ceiling on the *whole* consult — `--turn-timeout`
alone does not bound it, since N×R turns multiply. Consult two participants and one
round unless you need more.

## Ending a meeting: the initiator confirms

`close` will not file the minutes until the **initiator** agrees the meeting is done.

- **Human initiator** — prompted on the terminal. If stdin is not a terminal, `close`
  refuses and tells you to pass `--yes`; it never silently ends someone's meeting.
- **Agent initiator** — asked through its own CLI, and must answer `CONCLUDE` or
  `CONTINUE`. An unparseable answer defaults to `CONTINUE`. A `CONTINUE` aborts the
  close and the meeting stays open.
- `--yes` skips the prompt. It is **recorded in the transcript**, not silent.

`start --non-interactive` auto-confirms a *human*-initiated meeting (there is nobody
to prompt) and still asks an *agent* initiator.

## Decisions: `infer` vs `explicit`, and the grounding contract

`--decision-mode infer` (the default) lets the secretary record a decision the
meeting clearly converged on, tagged `*(inferred from consensus; agreed: …)*`.
`explicit` records only what a participant declared outright; anything else becomes
an open question.

**An inferred decision must name a proposal and an acceptance.** The secretary is
required to emit `[agreed: name1, name2]`, and an inferred decision naming fewer than
two supporters is **demoted to an open question in code** — not by asking the model
nicely — with the note *"raised, but no recorded agreement — not a decision"*.

This exists because measurement says it must. LLM summaries of *dialogue* are
inconsistent roughly 23% of the time (about 5× worse than on news), and the dominant
error class is "circumstantial inference": a decision that was implied but never made.
Discussing an option is not agreeing to it. Human `/decision` markers are
authoritative, need no support, and are never tagged.

## Minutes

```
# Meeting — <topic>            Initiator · Attendees · Context reviewed · Agenda
## Summary
## Decisions                   explicit first, inferred tagged
## Action items
## Risks
## Open questions
## Corrections / revised framing   ← what the meeting superseded, so stale agenda
                                     language does not read as endorsed
## Polls                       question, tally, per-participant vote + rationale
## Participant coverage        turns / ok / abstain / empty / timeout / error /
                               short / invalid / chars, plus a retry recommendation
## Notes (turns)               every turn IN FULL as a blockquote (capped at 4k
                               chars, with a link to the complete per-turn file)
```

Published to `<repo>/docs/meetings/meeting-note-<ts>-<slug>.md` when cwd is inside a
git repo, else `~/.bashy/meet/<id>/minutes.md`.

**The home directory is redacted to `~` everywhere in the minutes.** Agent CLIs print
their workdir in startup banners and the minutes get committed.

## Amending weak minutes

The transcript is the durable artifact; the minutes are a projection of it. A weak
secretary pass is not fatal:

```bash
bashy meet amend <id> --resynthesize                 # re-run the secretary, rewrite minutes
bashy meet amend <id> --decision-mode explicit       # re-run under a stricter mode
bashy meet amend <id>                                # just re-render (e.g. after a code change)
```

`converge` writes `synthesis.json` (latest pass wins) and **never appends markers to
the transcript**, so amending is idempotent and cannot double-count.

## Failure reporting

Every turn records a status, an exit code, a duration, and a character count:

| Status | Meaning | Retry? |
|---|---|---|
| `ok` | contributed | — |
| `abstain` | declined an optional question | — (a contribution) |
| `timeout` | exceeded `--turn-timeout` | **yes** |
| `error` | non-zero exit / launch failure | **yes** |
| `empty` | ran cleanly, produced nothing | no |
| `short` | below `--min-turn-chars` | no |
| `invalid` | answered, but not with a ballot choice | re-ask, narrower |

`meet show <id>` prints the table; the minutes carry it plus a diagnosis line per
failing seat. Only real failures net down a tool's operability score in
`pkg/capability` — an abstention does not.

## What the host provides (robustness features)

- **Context offloading** — each turn's full text is stored under
  `~/.bashy/meet/<id>/turns/`; the transcript replayed to attendees carries a
  head/tail **preview + a `file://` link** (read-on-demand), recent turns in full and
  older turns collapsed to a one-line reference — so the prompt stays bounded no
  matter how many rounds run.
- **Shared source set** (`--context FILE`) — every participant reads the same files
  before its first turn, so the panel reviews one artifact rather than guessing.
- **Per-turn timeout** (`--turn-timeout`) — a wedged agent can't hang the round.
- **Clean failed turns** — a failed/timed-out turn becomes a short marker, never the
  raw CLI banner; the round continues.
- **Attendee gate (advisory)** — warns on non-routable participants and oversized
  rosters at start/`--dry-run`.
- **Recursion guard** — a meeting cannot be convened from inside a meeting.

## State & output

A session is local under `~/.bashy/meet/<id>/`:

| File | Contents |
|---|---|
| `state.json` | the durable header (roster, initiator, modes) |
| `transcript.jsonl` | **append-only** events: turns, votes, chair ledgers, re-plans, human markers, confirmations |
| `synthesis.json` | the secretary's latest pass — rewritten, never appended |
| `turns/*.txt` | each turn's complete text |
| `minutes.md` | when cwd is outside a git repo |
