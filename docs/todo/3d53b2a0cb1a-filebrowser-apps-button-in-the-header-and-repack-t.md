---
id: 3d53b2a0cb1a
kind: task
title: 'filebrowser: Apps button in the header, and repack the embedded SPA'
seq: 25
status: todo
priority: p1
created: 2026-09-01T18:54:21.307479Z
assignee: ci-repair
sprint: 101
---

Header gains an Apps control returning to the bashy apps launcher, wired through every view that renders a header via a showApps prop, with its en.json string and a contract test.

fbembed/dist.zip must be repacked in the same change: importers (outpost, the bashy console) embed the BUILT SPA, not these sources, so a frontend edit that does not repack ships nothing and the two silently disagree about what the UI is.
