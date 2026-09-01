---
id: 36b59afd72ab
kind: task
title: 'webconsole: refresh a spent pairing code, and land a paired device on the launcher'
seq: 27
status: todo
priority: p2
created: 2026-09-01T19:21:46.314873Z
assignee: ci-repair
sprint: 101
---

A pairing pass is single-use, so the code on screen is spent the moment a phone redeems it and a retry reports 'that code has already been used'. Added a Refresh control beside Show pairing code, hidden until there is a code to replace.

landingFor sent a freshly paired device to the first panel in its scope (board, then mb/relay/files/terminal), dropping a phone straight into the board. It now lands on the launcher: the console's one nav, listing exactly the panels that device is scoped to, and a consoleWidePath so the landing can never be a page the device is refused.
