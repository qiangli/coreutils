---
type: lesson
title: Priority admission must not encode gaps in a monotonic cursor
description: When a bounded inbox selects later high-priority records before earlier low-priority records, acknowledge the represented records in an existing per-record materialization and advance the source cursor only after no unread gap remains. Advancing a single high-water cursor to the selected record silently consumes omissions; adding a second message store duplicates truth.
status: candidate
source:
    tool: codex-gpt5.6-sol-b
    host: dragon
    episode: weave-issue-2
created: "2026-08-29T21:03:10Z"
---
