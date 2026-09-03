---
id: d23761ebec51
kind: task
title: Tolerate slow Chrome DevTools startup in the webconsole CI gate
seq: 37
status: done
priority: p0
created: 2026-09-03T19:15:09.598021Z
sprint: 101
closed: 2026-09-03T19:15:20.466537Z
---

GitHub Actions run 33793026930 failed only in Linux TestDOMLauncherRendersCleanly: chromedp hit its 20s WebSocket URL read timeout while later browser cases ran. Raise only the bounded DevTools endpoint startup wait; keep the 90s test context. Accept when verifydom, ordinary webconsole tests, and fmtcheck pass.
