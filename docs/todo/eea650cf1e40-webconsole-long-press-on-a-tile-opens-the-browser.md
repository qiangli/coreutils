---
id: eea650cf1e40
kind: task
title: 'webconsole: long press on a tile opens the browser context menu over the star'
seq: 29
status: todo
priority: p2
created: 2026-09-01T20:10:57.167855Z
assignee: ci-repair
sprint: 101
---

Verified on a real Samsung by the operator: long press already favourites fine -- the press puts the tile in its hover state, the star appears, and it can be tapped. The defect is that the SAME gesture also opens the browser's long context menu on top of it.

Fix is only that menu, and only on tiles: page-wide suppression would cost copy-link, open-in-new-tab and share everywhere else. A mouse right-click keeps its menu, since a desktop user has no long press and would lose those for nothing.

Corrects an earlier assumption of mine that a favorite could never be ADDED on touch (because .star is opacity:0 until hover). Device testing disproved it; the always-visible-on-touch CSS I had added was removed as unnecessary.
