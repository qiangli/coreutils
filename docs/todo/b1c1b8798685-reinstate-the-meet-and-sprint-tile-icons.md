---
id: b1c1b8798685
kind: task
title: Reinstate the Meet and Sprint tile icons
seq: 63
status: done
created: 2026-09-05T05:18:02.991489Z
sprint: 122
closed: 2026-09-05T05:18:48.901358Z
---

The launcher's SVG mark table and pinned tile colours are keyed by PANEL NAME. The relay->meet and board->sprint rename rekeyed the routes but not the table, so both tiles fell back to appIcon's initial-letter placeholder (M, S). Rekey meet/sprint and pin the join with two gates: a byte-level table-coverage test over Discover(), and a browser test that reads what the launcher actually paints.
