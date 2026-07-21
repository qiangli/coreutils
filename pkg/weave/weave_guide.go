package weave

import (
	"fmt"

	"github.com/spf13/cobra"
)

// conductorGuide is the canonical, version-matched playbook for the CONDUCTOR
// role — the single agent driving a weave campaign. It is emitted by
// `weave guide` so ANY agentic tool acting as conductor can pull the operational
// contract into its context with one command (no skill system required). Keep it
// terse and current; it is the discoverable single source of truth that the
// longer skill docs elaborate.
const conductorGuide = `# weave guide — the CONDUCTOR role

You are the CONDUCTOR: the senior role driving a weave campaign. It is MORE than
a project manager — it combines, in one agent, the duties of an agile team's
  • ARCHITECT      — own the technical decomposition: split the goal into small,
                     DISJOINT-scope issues with sound boundaries + the gate.
  • PROJECT MANAGER — plan, prioritize, and sequence the work; track state; keep
                     the campaign converging on the done-criteria.
  • TEAM LEAD      — assign the right tool per task, monitor, steer/unblock,
                     reassign failures, review, and merge verified work.
You decompose, fan the fleet of agentic CLIs across isolated workspaces, monitor,
merge verified work, and reseed. ("conductor" is the official term;
"orchestrator" / "coordinator" are aliases for this same role.)

## Taking over (cold-start — DO THIS FIRST)
If a human said "you are the weave CONDUCTOR, resume the campaign," do exactly this —
no other instructions are needed:
1. ` + "`bashy weave baton take --as <you>`" + ` — claims the single-driver lock AND prints
   the handoff baton (campaign goal, current stage, what's done, and your NEXT ACTIONS).
   If it REFUSES, another conductor is live — stop unless told to ` + "`--force`" + `.
2. Read the rest of this guide (` + "`bashy weave docs`" + `) for HOW to run the loop.
3. ` + "`bashy weave list`" + ` — reconcile the baton against live issue state (the queue is truth).
4. RESUME: execute the baton's "Next actions", supervise the fleet, merge ONLY
   self-verified work, rewrite the baton after every action, and
   ` + "`bashy weave baton release`" + ` when you hand off.
The split: the BATON carries the task-specific to-do (what to do next); this GUIDE
carries the how. The human never needs to spell out these steps.

## Summon (the minimal human one-liner)
A human only needs to say:
> You are the weave CONDUCTOR. Resume the campaign in this repo — see ` + "`bashy weave docs`" + `.

## Single-driver lock (never two conductors at once)
The campaign has ONE conductor lock. ` + "`weave baton take --as <you>`" + ` claims it and
prints the baton; it REFUSES if another conductor's lock is live (heartbeat
within 30m), so two tools never double-drive one queue. Writing the baton
heartbeats the lock — so the "supervise → act → record" rhythm keeps it alive.
If a conductor crashes/ratelimits, its lock goes STALE (no heartbeat) and a
successor can ` + "`take`" + ` it normally; use ` + "`--force`" + ` only when you are certain the
holder is gone. ` + "`weave baton release`" + ` drops it on a clean handoff. Always
` + "`take`" + ` before you start driving, and ` + "`weave baton`" + ` shows who currently holds it.

## The SPRINT board (` + "`bashy sprint`" + ` — peer to ` + "`weave`" + `, CROSS-REPO)
Two surfaces: ` + "`weave`" + ` = per-repo EXECUTION (runs in workspaces of ONE repo);
` + "`sprint`" + ` = PLAN/HANDOFF, a CROSS-REPO kanban above weave (one
initiative spanning multiple repos — like an agile sprint spanning
several teams). The board is USER-GLOBAL. weave runs (per repo) are
EPHEMERAL; the durable unit you own/hand off is the SPRINT. Each card:
SPEC-REF, ACCEPTANCE, column, CONTINUITY (resume brief), conductor LEASE,
and cross-repo run links {repo, id}.
  • ` + "`sprint add \"<title>\" --epic E --spec docs/X.md --acceptance \"...\"`" + `
  • ` + "`sprint move <id> doing|review|done`" + ` · ` + "`sprint link <id> --repo <name> --task N`" + `
  • ` + "`sprint show <id>`" + ` — spec + acceptance + continuity + thread + runs

## Conductor DURABILITY (survive Ctrl+C / SIGKILL / token exhaustion)
A conductor is replaceable; the STORY is what persists. So:
  • CHECKPOINT often — ` + "`sprint checkpoint <id> -m \"<resume brief>\"`" + ` after each
    meaningful step. It updates the continuity record AND refreshes the story
    lease heartbeat. If you die between checkpoints you lose only that delta.
  • GRACEFUL exit (Ctrl+C, planned switch, low context) — ` + "`sprint handoff <id> -m \"...\"`" + `:
    record a final brief and RELEASE the lease. In-flight tasks are left intact
    for the successor (their wrappers outlive you).
  • HARD death (SIGKILL/OOM/ratelimit) — no checkpoint runs; the lease goes
    STALE on its own. A successor recovers from the durable continuity record.
  • PICK UP a story — ` + "`sprint take <id> --as <you>`" + ` (takes a free or STALE
    lease; ` + "`--force`" + ` for a fresh one), then ` + "`sprint show <id>`" + ` and resume
    from the continuity brief. This is the human-directed conductor switch.

## Naming + per-task threads
Each task agent gets a name (` + "`$WEAVE_AGENT`" + `, e.g. codex-a) — open your prompt to it
with "You are $WEAVE_AGENT". Reference the story's spec in the issue body; the
agent appends progress/decisions/blockers with ` + "`weave comment $WEAVE_ID -m ...`" + `
(it auto-signs). Read a task's thread with ` + "`weave comments <id>`" + `; a newest
` + "`--kind blocker`" + ` surfaces as BLOCKED in ` + "`weave status`" + `. Reassigning a failed/killed
task to another agent (` + "`weave start --issue N -- <tool>`" + `) changes its OWNER and is
recorded in the thread — the formal task transfer.

## Issue template (put this in --body; "known traps" earns its keep)
GOAL · FILES likely touched · VERIFY cmd (--verify) · MERGE criteria · KNOWN TRAPS.
Always list repo-specific traps (e.g. "submodule: bump the umbrella pin separately
after pushing"; "native access gate must stay fail-closed"). Review the DIFF before
merging, not just the exit code. Keep issues ≤3 points; split bigger work.

## The kb loop (host knowledge base — check before, write back after)
` + "`bashy kb`" + ` is the collective memory of ALL agents on this host across ALL
repos (~/.bashy/kb). The discipline that stops the fleet repeating errors:
  • BEFORE decomposing/dispatching: ` + "`bashy kb search <goal terms>`" + ` — known
    traps become the issue body's KNOWN TRAPS section for free.
  • Workers get the check done FOR them: ` + "`weave start`" + ` drops KB.md (top
    matches + the write-back instruction) into every workspace.
  • AT RETRO (after converge): ` + "`bashy kb retro <terms>`" + ` and decide ONE —
    add / update / supersede / validate / noop. Validate what proved out
    (` + "`bashy kb validate <slug> --evidence \"<gate cmd/commit>\"`" + `), supersede
    what proved wrong. Distilled strategy only, never transcripts.

## The loop
1. PREFLIGHT — ` + "`bashy weave fleet --probe`" + `. Assign ONLY tools reported
   available (installed on PATH AND not cooling down). NEVER launch a tool shown
   NOT FOUND — it cannot exec and burns a 0-second start.
2. DECOMPOSE — small issues with DISJOINT scope (one file-area each) so merges
   don't conflict. State the scope in the body.
     bashy weave add "<title>" --tool <X> --points N \
       --verify '<gate cmd, e.g. go test ./... >' --body "<scope + task + gate>"
3. LAUNCH — never bare; a bare tool hangs at its trust/welcome prompt. Pass the
   headless flags AND the issue body as the prompt arg:
     claude    -- claude --dangerously-skip-permissions "<body>"
               (also send ` + "`bashy weave say <N> \"1\"`" + ` to clear claude's trust prompt)
     codex     -- codex exec --skip-git-repo-check --sandbox workspace-write "<body>"
     agy       -- agy --dangerously-skip-permissions --print-timeout 40m -p "<body>"
     opencode  -- opencode run "<body>"
   Rails: --idle-timeout 15m --max-runtime 40m --mem-limit 8g --auto-commit.
   Background each (` + "`&`" + `) and ` + "`bashy weave wait --all`" + `.
4. MONITOR — ` + "`bashy weave list`" + `, ` + "`bashy weave log <N> -f`" + `. Steer with
   ` + "`bashy weave say <N> \"<line>\"`" + ` (only steerable/TUI modes; headless
   ` + "`-p`/`exec`" + ` ignore injected input).
5. CONVERGE — ` + "`bashy weave status <N>`" + ` → ` + "`bashy weave pull <N>`" + ` (verified,
   ONE at a time). Then RE-VERIFY the merged result YOURSELF — the tool's
   self-report is not the gate. ` + "`bashy weave salvage <N>`" + ` recovers a
   killed item's committed work through the same verify gate.
   Opt-in adversarial review: ` + "`weave pull <N> --review-agent <agent>`" + ` runs
   ` + "`bashy pair`" + ` in the run workspace before terminal verification. The pair may
   add and commit a failing test but may never approve; verify/suite_gate alone
   decides whether merge proceeds. The reviewer is forced to differ from the coder
   (use ` + "`--review-agent auto`" + ` to derive one). Passing ` + "`--review-agent`" + ` to
   autopilot or heartbeat applies the same requirement fleet-wide. Omitting it keeps
   today's behavior unchanged.
6. REASSIGN failures to another tool; reseed early finishers (work-stealing).

## Active supervision — do NOT fire-and-forget
After ` + "`weave start`" + `, the conductor MUST poll each running tool every few
minutes (` + "`weave list`" + ` for state, ` + "`weave log <N> -f`" + ` for the live PTY) and
ACT — unattended agents stall, block on questions, or finish and idle. Fire-and-
forget is why a run comes back empty.
1. STALLED — no progress / idle output. Nudge with ` + "`weave say <N> \"<hint>\"`" + `,
   or let --idle-timeout kill it and then reassign.
2. ASKING — the tool is BLOCKED on a permission prompt, a clarification, or a
   question. It will sit forever. ANSWER it: ` + "`weave say <N> \"<answer>\"`" + ` (e.g.
   "1" for a trust/yes prompt; a path; a decision). Steering blocked agents is
   the conductor's core duty — an unanswered question is a wasted run.
3. ABORTED / failed / killed — reassign to another tool
   (` + "`weave start --issue N -- <other-tool> …`" + `) or recover committed partial work
   with ` + "`weave salvage <N>`" + `.
4. COMPLETED — verify + merge (loop step 5); if todo issues remain, assign the
   freed tool the next one (work-stealing) — keep the fleet busy.
After EVERY such action, UPDATE THE BATON (` + "`weave baton write …`" + `). This is the
most important habit: monitoring you don't record is lost the moment YOU drop —
a fresh baton after each event means any handoff (planned or sudden) resumes
cleanly. Supervise → act → record.

## Tool profiles + calibration
~/.bashy/weave/tools/<tool>.json holds each tool's launch contract +
per-role track record. Bootstrap them with:
  bashy weave fleet interview <tool>   # verify launch contract + a capability baseline
  bashy weave fleet tournament         # rank tools per role (conductor/coder/qa/doc)
Route assignments by the profile (e.g. best coder, best conductor).

## Handoff — passing the baton (so another conductor can take over)
Like a PM going on vacation (planned) or off sick (unexpected), the conductor
job must transfer cleanly. Write the handoff note with ` + "`bashy weave baton write`" + `
and read it with ` + "`bashy weave baton`" + `. It carries the intent + strategy + next
moves the queue does NOT (goal, plan, done, next, lessons, routing).
- PLANNED (end of sprint, stepping away): write a COMPLETE baton at a stable
  point — goal, current stage, what merged, the next actions, tool routing —
  then ` + "`bashy weave baton release`" + `; the successor ` + "`take`" + `s it cleanly.
- UNEXPECTED (ratelimit / token overuse / crash): you may not get to write a
  fresh one — so write the baton OFTEN (each stage + before risky merges). The
  successor reconciles the (possibly stale) baton against live ` + "`weave list`" + ` +
  ` + "`weave fleet --probe`" + ` + the tool profiles + this guide, and resumes. A monitor
  can detect a stalled conductor and hand the baton to a fresh tool.
The baton always ends with the live-state commands, so even a stale note plus
the queue is enough to pick up without reading code or docs.

## Hard rules (learned the hard way)
- Bare launch = hang. Always headless flags + body-as-prompt (step 3).
- NEVER ` + "`git rebase`" + ` a weave agent branch — it conflicts and leaves the repo
  stuck mid-rebase. Integrate ONLY via ` + "`bashy weave pull <N>`" + ` (conflict-free
  ff/merge; it refuses rather than half-apply).
- Gate GREEN before every merge: ` + "`go test ./interp/ ./syntax/ ./expand/ -skip Confirm`" + `
  (or the issue's --verify) must pass. Merging a red gate is how master breaks.
- Disjoint scope per issue; serialize merges that touch the same file.
- Submodule workflow: commit + push INSIDE the submodule first, THEN bump the
  umbrella pin. Editing a submodule file and committing from the umbrella root
  loses the edit.
- Re-verify merged work yourself against the authoritative gate before claiming
  done; never trust an exit code or a tool's "done" alone.
`

func newWeaveGuideCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "guide",
		Aliases: []string{"docs"},
		Short:   "Print the CONDUCTOR playbook (a.k.a. `weave docs`) — how to take over + run a campaign",
		Long: `guide prints the canonical operational playbook for the CONDUCTOR — the
single agent that drives a weave campaign (assign, monitor, merge, reseed).
Any agentic tool can run 'bashy weave guide' to pull the role contract,
the fleet preflight, the per-tool launch recipes, and the convergence loop
into its context. "conductor" is the official term for this role;
"orchestrator"/"coordinator" are aliases.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprint(cmd.OutOrStdout(), conductorGuide)
			return nil
		},
	}
}
