---
id: f2c6d2526498
kind: task
title: 'BUG: mb send posts and reports success when a selector cannot resolve'
seq: 103
status: todo
priority: p1
created: 2026-09-08T03:58:01.976715Z
sprint: 139
---

FOUND while adding the --role selector (story 312252aeb358): pkg/bus/send.go resolved the audience AFTER durably appending the post, and DISCARDED the resolver error:

  seq, err := PostMessageSeq(...)          // durable append happens first
  if names, ferr := FleetSelect(aud); ferr == nil { ... }   // error dropped

So `mb send --role reviewer` (or any selector the host cannot resolve) posted to the board and reported success while reaching nobody.

WHY IT IS A DEFECT AND NOT A CHOICE: Send's own doc comment states the contract in as many words -- "an unresolvable target writes NOTHING and fails with choices -- a post to a name nobody answers was a receipt indistinguishable from a real delivery." The req.To path honours that and resolves before validating the body. The AUDIENCE path did the opposite. The comment was already right; only the selector branch disagreed with it.

FIX (landed): resolve the audience BEFORE PostMessageSeq and return the resolver error, so a failed send writes nothing. Also added Role to Audiences() so the receipt label reads "posted to role conductor" instead of an empty "posted to ".

GATE: pkg/bus/send_audience_test.go asserts the board does not grow on an unresolvable selector, that a resolving selector still posts once and names its recipients, and that a role-only Audience is not Empty (or it would silently broadcast). Mutation-checked: restoring the swallowed error fails the test.

STILL OPEN, deliberately not changed here: a selector that resolves to ZERO names still reports "posted to <label>" with no recipients. That is honest for --band 9 (nobody is at band 9) but reads as success for --role conductor when no manager is seated. Deciding what a zero-match receipt should say is a separate question from swallowing an error, and belongs with whoever owns the delivery-state ladder.
