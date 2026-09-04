---
id: bdbc92811123
kind: task
title: 'Sprint web view: restore a sprint''s stories, state its open/closed split, and keep disclosures open across a refresh'
seq: 38
status: done
priority: p1
created: 2026-09-04T08:41:36.485814Z
sprint: 122
closed: 2026-09-04T08:43:14.369984Z
---

The Sprint app served ZERO stories. board's todo source asked the personal-list store for `--user --owner <name>`; --owner is an item's ASSIGNEE, not a store selector, so todo answered 'unknown flag: --owner' and todoSource.Load returned on that FIRST failure before reading a single repo store. A host with 183 stories reported 0 — every card rendered with no story chips and nothing to click, and the only evidence was one warning line about a flag.

FIX (pkg/board/sources.go): ask only for what the CLI can answer (--user reaches exactly the default owner's list) and NAME any other owner directory in the warning; make a per-store failure cost that store's rows and nothing else, still reporting which stores were skipped.

FIX (webconsole artifact): a head chip stating open/closed without expanding; closed story chips struck through in the done green so they no longer render identically to open ones; every disclosure routed through one disclose() helper that remembers its state, so the 15s poll's replaceChildren no longer collapses the story list and continuity brief under a reader; and the two toggles no longer render as one run-on word.

GATE: pkg/board regression tests (no store is ever addressed with --owner; a failing store no longer erases the readable ones) + three browser tests in console_dom_test.go (chips render with the right split and distinct colours, a story number opens a non-empty detail pane, an expanded disclosure survives a re-render — verified red against the old JS). scripts/crossvet.sh PASS, scripts/ci-test-gate.sh OK.
