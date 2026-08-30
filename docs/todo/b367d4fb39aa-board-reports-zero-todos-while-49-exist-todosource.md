---
id: b367d4fb39aa
kind: task
title: 'board reports zero todos while 49 exist: todoSource reads pre-loom-v2 JSON and its version guard only checks the field is non-empty'
seq: 10
status: done
priority: p1
created: 2026-08-30T22:01:22.172867Z
closed: 2026-08-30T22:07:28.687364Z
---

SPEC: coreutils/pkg/board (schema bashy-board-v1), coreutils/pkg/todo. DESIGN SIBLING: umbrella fe811f11 (sprint/todo/weave consistency contract) — that item is the convention; THIS is the shipped defect and is not gate-blocked.

SYMPTOM: `bashy board` reports ZERO todos on a host with 49 of them.

  $ bashy board --json | jq '.summary.todos, .todos'
  0
  null
  $ bashy board --json --expand todo    # identical, 84 rows: 82 sprint, 2 run, 0 todo
  $ bashy todo list --json | jq '.result.count'
  49
  $ bashy board --json | jq '.warnings'
  null                                   # nothing reported; the failure is SILENT

`board --help` says "Show the machine-global steward board across todo, sprint, and weave". Two of
the three are present. The steward's primary view silently under-reports its own work queue.

ROOT CAUSE — an envelope-shape drift the version guard was supposed to catch and does not.
pkg/board/sources.go todoSource.Load decodes the output of `todo list --json` into:

    var env struct {
        SchemaVersion string `json:"schema_version"`
        Items []struct{ ID, Title, Status, Priority string; Seq int; ... } `json:"items"`
    }

i.e. it expects `items` at the TOP LEVEL. What `todo list --json` actually emits today is the
loom-v2 envelope:

    { "schema_version": "loom-v2", "command": "todo list", "status": "ok",
      "result": { "scope": ..., "folder": ..., "count": 49, "items": [...] } }

so `items` is nested under `result`. `env.Items` therefore unmarshals to EMPTY, the loop appends
nothing, and Load returns nil. No error, no warning, no log — `b.Todos` stays empty and both
`summary.todos` and `todos` render as 0/null.

WHY THE GUARD MISSED IT (the part worth fixing properly): the next line is

    if env.SchemaVersion == "" { return fmt.Errorf("%s returned unversioned todo JSON", sc.scope) }

It tests only that the field is NON-EMPTY. "loom-v2" is non-empty, so a total change of payload
shape sails through the check that exists to detect exactly that. A version guard that does not
compare the version to anything is decoration.

A SECOND, INDEPENDENT MISMATCH, still latent: board's item struct reads `Status`, while the items
emitted carry `state` (item keys today: age, created, id, priority, scope, seq, state, title). Fixing
only the `result` nesting would populate rows with an EMPTY status column. Fix both together or the
first fix looks broken.

FIX
1. Decode the loom-v2 envelope: read `result.items`, not top-level `items`.
2. Map `state` -> Status (and confirm the rest of the field names against the live payload rather
   than against the old struct).
3. Make the version guard compare: accept a known schema_version, and FAIL LOUDLY on an unknown one
   instead of proceeding with a zero-length decode.
4. Do not let a source that loaded nothing look identical to a source that legitimately has nothing:
   surface a warning in `board.warnings` when a configured todo scope yields zero items but the
   store is non-empty on disk, or at minimum when the decode produced no items from a non-empty
   payload.

TESTS THAT WOULD HAVE CAUGHT IT, and none of which exist today
- A golden test that feeds `todo list --json`'s REAL current output through todoSource and asserts
  a non-zero count. The existing sources_test.go passes because it constructs the shape board wants
  rather than the shape todo emits — the two packages are tested against each other's assumptions,
  never against each other.
- An assertion that `board --json`'s summary.todos equals `todo list --json`'s result.count for the
  same scope. One line, and it pins the contract between the two packages permanently.
- A guard test: an unknown schema_version must error, not silently yield zero.

WHY p1: this is silent data loss in the steward's own dashboard. A count of 0 is indistinguishable
from "nothing to do", which is the exact wrong answer to give a steward, and nothing in the output
hints that a source failed. It also mis-sets `bashy board --html`, which renders the same summary.

NON-SCOPE: the sprint<->todo linkage design (umbrella fe811f11); adding a sprint field to todo
frontmatter; changing the loom-v2 envelope, which is fine and is not the thing at fault here.
