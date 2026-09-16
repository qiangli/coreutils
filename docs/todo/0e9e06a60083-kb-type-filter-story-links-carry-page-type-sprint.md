---
id: 0e9e06a60083
kind: task
title: kb --type filter, story links carry page type, Sprint app Runbooks section
seq: 146
status: done
priority: p1
created: 2026-09-16T15:59:06.72538Z
assignee: transom
sprint: 201
closed: 2026-09-16T16:14:17.675594Z
closed_by: transom
---

Sprint #201 story 3 (code, coreutils only, ~400 LOC incl. tests). KISS: no /kb tile, no new ref kind, no kb edit/rm, no context injection, no sprint advance change.

C.1 pkg/kb/kb.go: filterType beside filterForm (:311-322); --type flag on newSearchCmd (:171) and newListCmd (:796), validated by ValidType, applied after filterForm. Test TestTypeFilterOnListAndSearch in kb_test.go (model: TestLegacyPageReadsAsForm :748-777).

C.2 pkg/kb/links.go LinkNode gains exported Type (KBNode sets p.Type). pkg/todo/cli.go linkRef gains Type json:"type,omitempty" (resolveLinks :470/:481); one assertion in TestResolveLinksBothDirections. pkg/board/model.go: LinkRef{Ref,Title,Type,Status}, Story.Outbound []LinkRef; sources.go StoryDetail (:645) appends "--links". board.js: runbookRefsEl (beside runRefsEl :239) keeps kb: refs with type==runbook or status==dangling (.ref.needs); openStory pushes it; click → openRunbook.

C.3 new pkg/webconsole/panel_runbooks.go: runbookRow/runbookDetail/runbookRing; seam var runbookRingsFn (like storyDetailFn) opening kb.Open(root/docs/kb) per distinct Sprint.StoryRoots then kb.Open("") host ring; listRunbooks (Type==runbook, dedupe by slug repo-first, sort); loadRunbook uses Store.Load ONLY (never RecordOpen — the 15s poll must not inflate activation); handleRunbooks (503 while board uncollected) + handleRunbook ({slug}; 400/404). Routes inside if on["sprint"] in handler.go (:360-366): GET /api/sprint/runbooks, GET /api/sprint/runbook/{slug}. board.html: <h2 class="bd-h">Runbooks</h2><section id="bd-runbooks"> after #bd-sprints. board.js: state.runbook + #runbook=<slug> in hash (writeHash), runbookDetail cached like storyDetail, openRunbook renders head/meta + existing storyBodyEl into #bd-runbooks .story-detail, renderRunbooks builds article.bd-sprint with button.ref.link chips + empty state; called from load(); story chip shares the same pane.

Tests: panel_runbooks_test.go (TestRunbooksListsRepoAndHostRingsDedupedBySlug, TestRunbookDetailAndUnknownSlug404, TestRunbookRoutesServeNoMutatingMethod); console_dom_test.go -tags verifydom (TestDOMRunbooksSectionOpensAPageBody, TestDOMStoryRunbookChipOpensTheSharedPane, TestDOMRunbookPaneSurvivesARefresh). Placeholder slugs only — coreutils is public. Commit with Sprint/Story trailers; push; umbrella pin bump.
