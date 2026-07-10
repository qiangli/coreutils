# bashy meet — reference

A **multi-participant deliberation session**: several agentic CLIs (claude, codex,
opencode, agy, aider, …) plus a human take turns on a topic, and a dedicated
**notes-only secretary** keeps the minutes, extracts decisions/action-items, and
files the result. It is the *deliberative* front half of the fleet; `weave` /
`conductor` / `sdlc` are the *executive* back half — decide in `meet`, execute there.

`meet` is also **callable by an agent**: a coding agent that wants a cross-vendor
second opinion mid-task runs `bashy meet consult` and reads back a verdict.

## Roles

- **Chair / facilitator** — sets the agenda, poses each item, decides when to
  converge (default = the human).
- **Initiator** — whoever convened the meeting. *Only the initiator may end it*
  (see `close`). May be a human or an agent.
- **Participants** — the agentic CLIs (and/or humans); each contributes one turn per
  round. Diversity of tool/model is the value (2–4 near-equal proposers is the sweet
  spot; more dilutes signal — the Self-MoA guard).
- **Secretary** — a *dedicated, non-participating* role: it only records, maintains
  the minutes, and on converge/close extracts decisions / actions / risks / open
  questions / corrections (never proposes, votes, or decides). Default: `claude`.

## CLI

```
bashy meet start   --topic TEXT [--participant AGENT ...] [flags]
bashy meet consult --topic TEXT --question TEXT [--choice yes --choice no] [--json]

bashy meet tell          <id> "<text>"          # append a human contribution
bashy meet round         <id>                   # one moderated round across participants
bashy meet poll          <id> --question TEXT   # fixed-choice ballot; every seat must answer
bashy meet ask           <id> --question TEXT   # open question; silence = "no comment"

bashy meet show          <id>                   # roster, per-participant coverage, artifacts
bashy meet contributions <id> [participant]     # every contribution, in full
bashy meet converge      <id>                   # secretary pass (rewrites synthesis.json)
bashy meet close         <id>                   # converge + initiator confirms + file minutes
bashy meet amend         <id> [--resynthesize]  # regenerate the minutes from the transcript
bashy meet apply         <id> --to FILE --write # append the agreed action items to a doc

bashy meet list | resume <id> | reference
```

Session flags (`start`, `consult`): `--topic` (required) · `--participant`
(repeatable) · `--assistant` (secretary; default claude) · `--agenda` (repeatable) ·
`--context FILE` (repeatable — every participant reads the same source set) ·
`--turn-timeout` (default 20m) · `--decision-mode infer|explicit` ·
`--min-turn-chars N` · `--initiator NAME` / `--initiator-kind human|agent` ·
`--out docs|kb|<path>`.

`start` adds `--rounds N --non-interactive` (run then auto-close), `--dry-run`
(print the resolved session + attendee gate, launch nothing), and `--yes`.

In the REPL: plain text = a human turn · `@name <text>` = a targeted turn · `/round` ·
`/poll <q>` · `/ask <q>` · `/decision <t>` · `/action owner: task` · `/agenda <t>` ·
`/show` · `/converge` · `/close`.

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
| `transcript.jsonl` | **append-only** events: turns, votes, human markers, confirmations |
| `synthesis.json` | the secretary's latest pass — rewritten, never appended |
| `turns/*.txt` | each turn's complete text |
| `minutes.md` | when cwd is outside a git repo |
