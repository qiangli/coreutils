---
id: c69f18eb8e44
kind: task
title: 'relay: header now matches board/messages/terminal'
seq: 31
status: todo
priority: p2
created: 2026-09-01T22:52:46.934149Z
assignee: ci-repair
sprint: 101
---

relay's header had no brand block and used lucide's LayoutGrid for the return control, so it did not read as one of the console's apps.

board, messages and terminal each state a brand in markup -- the panel's mark in a rounded tile, then bashy + the panel name -- and receive the console's four-square all-apps control from injectChrome. relay is a MOUNTED SPA served outside that path, so it receives neither and has to draw both itself, exactly as the Files app does.

The wordmark is hidden below sm: on a phone the same header carries the rooms button and the conversation title, and the title is what matters there; the mark alone still identifies the app.
