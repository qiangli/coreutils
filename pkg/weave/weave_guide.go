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
     codex     -- codex exec --skip-git-repo-check --workspace workspace-write "<body>"
     agy       -- agy --dangerously-skip-permissions --print-timeout 40m -p "<body>"
     opencode  -- opencode run "<body>"
     aider     -- aider --yes-always --no-check-update --message "<body>"
   Rails: --idle-timeout 15m --max-runtime 40m --mem-limit 8g --auto-commit.
   Background each (` + "`&`" + `) and ` + "`bashy weave wait --all`" + `.
4. MONITOR — ` + "`bashy weave list`" + `, ` + "`bashy weave log <N> -f`" + `. Steer with
   ` + "`bashy weave say <N> \"<line>\"`" + ` (only steerable/TUI modes; headless
   ` + "`-p`/`exec`" + ` ignore injected input).
5. CONVERGE — ` + "`bashy weave status <N>`" + ` → ` + "`bashy weave pull <N>`" + ` (verified,
   ONE at a time). Then RE-VERIFY the merged result YOURSELF — the tool's
   self-report is not the gate. ` + "`bashy weave salvage <N>`" + ` recovers a
   killed item's committed work through the same verify gate.
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
