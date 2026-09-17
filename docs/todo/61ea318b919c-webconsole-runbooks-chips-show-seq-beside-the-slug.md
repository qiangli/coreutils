---
id: 61ea318b919c
kind: task
title: 'webconsole: Runbooks chips show seq beside the slug; slug clipped to a fixed width with an ellipsis, full kb:<slug> in the tooltip'
seq: 149
status: todo
created: 2026-09-17T03:19:55.074441Z
---

Follow-up to Sprint 201 (kb:sprint-runbooks). A runbook chip on the Sprint app named the page by slug only, and a long kebab-case slug stretched the chip row. Now: #seq + slug (kb.Page.Seq threaded through kb.LinkNode, todo show --links linkRef, board.LinkRef and the runbook rows), slug clipped at 20ch with text-overflow ellipsis, button title = complete kb:<slug> (kb:<seq>) — hover to read it. Gate: go test -tags verifydom ./pkg/webconsole -run 'TestDOM.*Runbook'.
