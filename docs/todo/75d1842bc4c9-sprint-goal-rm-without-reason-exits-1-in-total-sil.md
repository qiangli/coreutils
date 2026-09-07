---
id: 75d1842bc4c9
kind: task
title: sprint goal rm without --reason exits 1 in total silence
seq: 98
status: done
priority: p0
created: 2026-09-07T20:25:16.995932Z
sprint: 136
closed: 2026-09-07T21:01:05.289529Z
---

MEASURED 2026-09-07 while splitting sprint #129:

  bashy sprint goal rm 129 ci-sweep   ->  exit=1  stdout=0B  stderr=0B

The goal is not retired and nothing says why. `--reason` is required — correctly, since retiring a goal must record where the outcome went — but the requirement is enforced by an invisible failure.

WHAT IT COST, and this is why it is filed rather than shrugged at. I ran two removals with `2>&1 | tail -1`, saw the NEXT command's output, believed both goals were retired, and went on to print a "corrected" sprint #129 that still carried both. A silent exit 1 does not just fail — it produces a confident wrong belief in whoever is driving, which is worse than an error, because an error stops you.

THIS IS A KNOWN DEFECT CLASS IN THIS EXACT COMMAND TREE. sprint_flagerr_test.go pins the fix for todo b0acdf2c: "NewSprintCmd inherited weave's silence (flags.attach sets SilenceErrors/SilenceUsage) but installed NEITHER reporter, so every subverb exited 1 having written ZERO bytes to stdout AND stderr." That fix covered UNKNOWN FLAGS. This is a MISSING REQUIRED FLAG, which takes a different path and was never covered — so the reporters are installed and this still slips past them.

IT IS NOT ONE VERB. Measured the same session, same shape:

  bashy sprint goal rm 129 ci-sweep   ->  exit=1  stdout=0B  stderr=0B   (--reason required)
  bashy sprint handoff 136            ->  exit=1  stdout=0B  stderr=0B   (--message required)

Two verbs, two different required flags, identical silence. That is what makes
this a CLASS rather than a typo — and `handoff` is the worse of the two, because
a conductor releasing a seat and getting a silent exit 1 has no way to tell
whether the lease was released. It looks exactly like a lease that stayed held.

WANTED. A sprint verb with no required flag fails the way every other sprint verb now fails: a named, non-empty message on stderr saying which flag is missing and why it is required. Then AUDIT THE SIBLINGS rather than fixing this one call — any sprint subverb with a required flag is a candidate, and the b0acdf2c test only proves the unknown-flag path.

GATE. Extend sprint_flagerr_test.go, which already has the right shape: it asserts exit code AND that stderr is non-empty, deliberately capturing the two streams separately. Add the missing-required-flag cases beside the unknown-flag ones, measured failing first — every case in that test was recorded FAILING before its fix, and that is what makes it a regression test rather than a description.

NOT LINKED TO A SPRINT yet: found while splitting #129/#137, belongs to neither. It is a sprint-surface defect and wants triage onto whichever sprint owns that surface next.
