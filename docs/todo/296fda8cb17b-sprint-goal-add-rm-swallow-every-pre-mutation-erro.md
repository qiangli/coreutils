---
id: 296fda8cb17b
kind: task
title: 'sprint goal add/rm swallow every pre-mutation error: exit 1 with no message'
seq: 82
status: todo
priority: p2
created: 2026-09-06T12:14:41.021909Z
sprint: 130
---

sprint goal add and sprint goal rm exit 1 with EMPTY stdout AND stderr on every
validation error raised before the mutation runs. Measured 2026-09-06.

REPRO (each prints nothing at all, including under --json):
  bashy sprint goal add 130                          # --id and --text are required
  bashy sprint goal add 130 --id x --text y --story <id-not-on-that-sprint>
  bashy sprint goal add notanumber --id x --text y   # ParseInt
  bashy sprint goal rm 130 <goal>                    # --reason is required

CONTRAST, same command: an error raised INSIDE the mutation callback reports
correctly, because runWeaveStoryMutate prints it itself --
  bashy sprint goal add 130 --id <existing> --text y
  -> sprint goal add: goal item "..." already exists

ROOT CAUSE. weave_story_goal.go's RunE prologues return a bare error
(return err / return fmt.Errorf) for: the --id/--text check, strconv.ParseInt,
normalizeStoryRoot, todopkg.ResolveRef, the it.Sprint != id guard, and rm's
--reason check. Those return straight to cobra, and weave.go:46-47 sets
SilenceErrors/SilenceUsage, so nothing prints. The ONLY thing that reports is
weavecli.EmitError inside runWeaveStoryMutate (weave_story.go:1503), which
these paths never reach.

SCOPE IS NARROW, not endemic: 2 of 9 sprint/todo paths probed are silent.
goal link, goal evidence, sprint link, sprint show, sprint comment,
sprint track and todo show all report correctly.

FIX, already established in this package: flagerr.go solved exactly this class
for flag-PARSE errors and its comment records the same failure mode with a real
consequence (a conductor read a silent non-zero exit as "checkpoint written"
and the successor got a stale baton). Apply that pattern to the RunE prologues:
  return ec(weavecli.EmitError(cmd.ErrOrStderr(), mode, op,
                               weavecli.ExitInvalidArg, err))

WHY IT MATTERS. The swallowed message is the one that tells you what to do:
"story X belongs to sprint #0, not #130" names both the problem and the fix
(todo edit X --sprint 130 first). Without it the operator sees only exit 1 and
must read the source to proceed, which is what happened here.

GATE: a test asserting each repro path writes a non-empty stderr and a non-zero
exit; plus the existing pkg/weave suite stays green.
