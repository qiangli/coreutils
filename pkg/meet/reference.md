# bashy meet — reference

A **multi-participant deliberation session**: several agentic CLIs (claude, codex,
opencode, agy, aider, …) plus a human take turns on a topic, and a dedicated
**notes-only secretary** keeps the minutes, extracts decisions/action-items, and
files the result. It is the *deliberative* front half of the fleet; `weave` /
`conductor` / `sdlc` are the *executive* back half — decide in `meet`, execute there.

## Roles

- **Chair / facilitator** — sets the agenda, poses each item, decides when to
  converge (default = the human).
- **Participants** — the agentic CLIs (and/or humans); each contributes one turn per
  round. Diversity of tool/model is the value (2–4 near-equal proposers is the sweet
  spot; more dilutes signal — the Self-MoA guard).
- **Secretary** — a *dedicated, non-participating* role: it only records, maintains
  the minutes, and on converge/close extracts decisions / action-items / open
  questions *as stated* (never proposes, votes, or decides). Default: `claude`.

## CLI

```
bashy meet start --topic TEXT [--participant AGENT ...] [flags]
bashy meet tell     <id> "<text>"      # append a human contribution
bashy meet round    <id>               # run one moderated round across participants
bashy meet converge <id>               # secretary pass: extract decisions/actions/open-questions
bashy meet close    <id>               # converge + write and file the minutes
bashy meet list | resume <id>          # inspect / reopen sessions
bashy meet reference                   # this document
```

`start` flags: `--topic` (required) · `--assistant` (secretary; default claude) ·
`--participant` (repeatable) · `--agenda` (repeatable) · `--out` (docs|kb|<path>) ·
`--turn-timeout` (default 20m) · `--rounds` + `--non-interactive` (run then auto-close) ·
`--dry-run` (print the resolved session + attendee gate, launch nothing).

In the REPL: plain text = a human turn · `@name <text>` = a targeted turn · `/round` ·
`/decision <t>` · `/action owner: task` · `/agenda <t>` · `/converge` · `/close`.

## How to initiate a meeting

```
bashy meet start \
  --topic "…" \
  --participant claude --participant opencode --participant agy \
  --assistant claude --turn-timeout 20m \
  --agenda "item 1" --agenda "item 2"
```

Run `… --dry-run` first: it prints the operability preflight (which attendees route
their shell through bashy) and warns on a roster past the Self-MoA sweet spot. For a
capability-informed roster, pick seats with `bashy capability best <capability>`. For
a fully unattended run add `--non-interactive --rounds N` (runs the rounds, converges,
and files the minutes).

## What the host provides (robustness features)

- **Context offloading** — each turn's full text is stored under
  `~/.bashy/meet/<id>/turns/`; the transcript replayed to attendees carries a
  head/tail **preview + a `file://` link** (read-on-demand), recent turns in full and
  older turns collapsed to a one-line reference — so the prompt stays bounded no
  matter how many rounds run.
- **Per-turn timeout** (`--turn-timeout`) — a wedged agent can't hang the round.
- **Clean failed turns** — a failed/timed-out turn becomes a short
  `(<agent> unavailable this turn: …)` marker, never the raw CLI banner; the round
  continues.
- **Attendee gate (advisory)** — warns on non-routable participants and oversized
  rosters at start/`--dry-run`.

## State & output

A session is local under `~/.bashy/meet/<id>/`: `state.json`, append-only
`transcript.jsonl`, `turns/*.txt` (full turn text), and the generated minutes.
Published minutes go to `<repo>/docs/meetings/meeting-note-<ts>-<slug>.md` when cwd is
inside a git repo, else `~/.bashy/meet/<id>/minutes.md`. Decisions and action items in
the minutes come from explicit `/decision` / `/action` markers **or** the secretary's
`converge` extraction; the secretary never invents them.
