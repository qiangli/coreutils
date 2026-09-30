---
id: 5ef807881811
kind: feature
title: 'pure-Go localedef (3/3): LC_COLLATE from localedef sources drives pkg/collate'
seq: 157
status: assigned
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:42.775348Z
weave: 25
assignee: qiangli
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Scope (8pt, 30m cap): collating elements, symbols and order from compiled locale feed pkg/collate so sort, ls, comm, join and bracket expressions honor locale built by localedef. Part2 accepted at integration77f8ce78. Worker must begin from that full integrated source; preserve other accepted lp code. Required red/green on custom collation order, element/symbol weights, ordering through named consumers; applet-matrix localedef go_applet, multicall registration and provider ownership metadata agree. Unsupported semantic features must fail explicitly, never silently approximate. POSIX spec and blackbox only: no GNU/glibc source copy/translation/paraphrase. Shared store/compiler files serialized with CLI residue02d2bb56298e (not active). No push/pins/install; report exact residuals and terminal evidence; conductor independently grades/merges. Actual BASHY_AGENT=s340-localedef-astra and Sprint340 Story157 Story-ID5ef807881811 trailers.
