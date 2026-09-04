---
id: d00549753ddf
kind: task
title: 'Console chrome: put the Inbox all-apps control in the bar like every other app'
seq: 54
status: done
priority: p2
created: 2026-09-04T19:57:37.773053Z
sprint: 122
closed: 2026-09-04T19:58:57.468981Z
---

injectChrome appended the all-apps button before the LAST </header>. Every other page has one header (the bar); inbox.html has a second one over its message list, so the control landed inside the content area and read as an inbox control rather than the console's. Target the close of <header id="bar"> instead, and cover /inbox/ in the chrome test that missed it.
