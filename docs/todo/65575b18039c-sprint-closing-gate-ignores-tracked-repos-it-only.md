---
id: 65575b18039c
kind: task
title: 'Sprint closing gate ignores tracked repos: it only inspects linked weave runs'
seq: 71
status: todo
priority: p1
created: 2026-09-06T11:30:33.745495Z
sprint: 130
---

CONFIRMED IN CODE. pkg/weave/weave_story_closing.go checkClosingConditions
iterates s.Runs and never reads s.StoryRoots. So the wrap-up gate on stop/end
inspects only repos reachable through a LINKED WEAVE RUN. A sprint with no
linked runs is checked against ZERO repos and always reports wrapped up.

OBSERVED 2026-09-06, both verdicts minutes apart on the same host and the same
working trees:

  sprint 101  4 tracked roots, 0 linked runs
              -> "ended without a recorded time-box; gate green"
              closed while coreutils held uncommitted work and bashy sibling
              pins were stale
  sprint 123  20 linked runs
              -> "NOT stopped - the repos are not wrapped up: dhnt 11
              uncommitted file(s); bashy stale pin(s): sh, coreutils"

WHY IT MATTERS MORE THAN IT LOOKS. sprint track is the command a manager uses
to declare which repos a sprint owns, and the gate that decides whether the
work was PUT anywhere does not read it. The blind spot is exactly the
Tool-managed mode sprint 130 story 6e38f62e9c1d is about: an agentic CLI told
to manage a sprint works the tree directly, links no weave runs, and therefore
gets no wrap-up verification at all. The manager reads a clean close and
reports done over a dirty tree.

The comment above the check states the intent plainly - "A green gate says the
code works; it says nothing about whether the work was PUT anywhere the next
sprint will find it" - and for a run-less sprint it verifies neither.

DO: fold the tracked story roots into the same repoState sweep as the run
repos, deduplicated by path, keeping the existing shared-with-another-active-
sprint exemption (sharedRepos) so a concurrently-worked tree stays attributed
rather than blocking.

DO NOT close this by making a run-less sprint refuse to close. The refusal must
name the same evidence a run-linked sprint gets - which file, which pin - or it
just moves the blindness from the gate to the message.
