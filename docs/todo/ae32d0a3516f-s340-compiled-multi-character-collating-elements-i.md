---
id: ae32d0a3516f
kind: bug
title: S340 compiled multi-character collating elements in bracket consumers
seq: 164
status: done
priority: p1
created: 2026-09-30T21:38:10.021917Z
weave: 28
assignee: s340-bracket-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
closed: 2026-09-30T22:29:28.375816Z
closed_by: codex-gpt6-astra
---

Part3 supports multi-character element string comparison but pkg/collate compiled byte bracket tables reject every locale containing a multibyte or multi-character element. This leaves the story157 bracket-consumer interface incomplete for those elements. Bounded8pt30m: after accepted part3, implement or isolate a proper compiled collation-aware bracket path for grep/sed and relevant matching consumers; [.[element].] (actual POSIX [[.ch.]] form), equivalence and ranges must consume the full element, no byte-table approximation. Acceptance red/green with declared ch element, anchored matching/quantification, neighboring single-character elements and ordinary custom-order regression; preserve host fallback and cross-platform pureGo builds. Clean-room POSIX XBD9 and locale7, no upstream source copying/translation/paraphrase. If beyond box, preserve exact open status and evidence; explicit unsupported error is not completed base behavior.
