---
id: 00ac1a95f15d
kind: refactor
title: S17 schedule foreground unit and pidfile lifecycle retirement
seq: 186
status: done
priority: p1
labels:
    - coreutils
created: 2026-10-11T07:48:13.688397Z
weave: 2
assignee: codex-gpt6-sol
sprint: 418
sprint_id: 9891800f-8da5-5338-92bc-1d75755ca548
sprint_title: 'Agent ecosystem synergy: agentic tools compose like a|b|c through bashy extension points'
closed: 2026-10-11T08:10:25.662473Z
resolution: fixed
closed_by: codex-gpt6-sol
---

Scoped child of Sprint 418 S17 Story-ID eae40495aac7. Own coreutils/pkg/schedule only. The bashy service monitor now owns daemon process lifecycle through the seeded schedule unit (`bashy schedule daemon` foreground exec); retire schedule's duplicated detached service start/status/stop and pidfile identity lifecycle directly, with no migration alias or old config path. Preserve all supported schedule job/cron semantics, admission, status visibility, signal cancellation and platform behavior. Do not touch yoke, Bashy monitor or outpost source. Verify GOWORK=off go test ./pkg/schedule/... on your branch, plus native Linux foreground schedule smoke and cross-OS compile under manager-claimed venues. Dragon focused short tests only. Commit with Sprint #418, this Story-ID and Agent trailer; manager handles remote gates and final S17 coverage acceptance.
