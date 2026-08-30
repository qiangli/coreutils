---
id: b0acdf2cea3c
kind: task
title: 'sprint: every subcommand fails silently on an unknown flag — exit 1, zero bytes on both streams'
seq: 11
status: done
priority: p1
created: 2026-08-30T23:30:16.611526Z
closed: 2026-08-30T23:42:27.150117Z
---

SPRINT: #87 (epic bashy-yoke; conductor casement). SHIPPED-CODE DEFECT, NOT
gate-blocked — same standing as the two coreutils browser items: this is a bug
in shipped code, not Yoke kernel design. SPEC: coreutils/pkg/weave.

THE DEFECT: every `bashy sprint` subcommand exits 1 on an unknown flag having
written ZERO BYTES to stdout AND zero to stderr. A silent exit 1 is
indistinguishable from a command that ran and found nothing.

MEASURED 2026-08-30 on this host, one line each:
  sprint comment 87 --body x     exit=1  out=0B  err=0B
  sprint checkpoint 87 --as x    exit=1  out=0B  err=0B
  sprint show 87 --nope          exit=1  out=0B  err=0B
  sprint take 87 --bogus         exit=1  out=0B  err=0B
  sprint board --nope            exit=1  out=0B  err=0B
  sprint status --nope           exit=1  out=0B  err=0B
CONTROL — the same binary gets this right everywhere else:
  todo list --nope    err="bashy todo: unknown flag: --nope"
  mb post --nope x    err="bashy mb: unknown flag: --nope"
  weave list --nope   err="weave list: unknown flag: --nope (run `weave list
                           --help` for ...)"

ROOT CAUSE, located: NewWeaveCmd installs a self-reporting flag handler —
pkg/weave/weave.go:79, `cmd.SetFlagErrorFunc(weaveFlagErrorFunc)`, whose own
comment explains cobra's FlagErrorFunc() climbs to the parent so EVERY subverb
inherits it, and that the alternative is "indistinguishable from success".
NewSprintCmd (pkg/weave/weave_story.go:308) never calls it. Both roots set
SilenceErrors, only one installs the handler. The mechanism is already written,
already documented, and already proven by weave — sprint simply does not use it.

FIX: install the same handler on the sprint root, and give positional/arg errors
the argerr.go treatment for the same reason. One line plus a test. Check the
other cobra roots in the tree for the same omission while there — this is a
class, not an instance.

COST, twice on record and both on this very card:
  - a prior session: `sprint ping 85` and `sprint comment <id> --body` (thread,
    08-30 10:38 / 10:40)
  - this session: `sprint checkpoint 87 --as casement` appeared to succeed, so a
    continuity record I believed was written was not, and I only noticed because
    I re-read the card
That makes this the THIRD recorded instance and the first with a located cause.

WHY IT BELONGS ON THIS CARD: it is defect class C — "a refusal that names no
remedy is indistinguishable from a reachability failure" — in its purest form,
because here the refusal names NOTHING AT ALL. It is also a direct violation of
bashy's own stated house rule ("anything else fails with a clear error (exit 2),
never a silent guess") and of rule 4 of the admission test in ed48f3f4 (a foreign
agent must be able to use a verb cold from --help alone; it cannot, if a typo is
silent). Exit code should be 2, not 1, per that house rule.

ACCEPTANCE
- Every `sprint` subverb, given an unknown flag, writes a diagnostic naming the
  flag to stderr and exits 2.
- A regression test asserts stderr is non-empty for at least: sprint show,
  comment, checkpoint, take, board, status.
- `sprint checkpoint --as X` either accepts --as (it is the natural spelling —
  take/handoff both have it) or names it as unknown. Silence is not an option.
- The other cobra roots in coreutils are audited for the same missing handler
  and the finding recorded either way.

NON-SCOPE: changing any sprint semantics; the Yoke design items on this card.
