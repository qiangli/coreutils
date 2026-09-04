---
id: 66dcc9d271ad
kind: task
title: 'Meet composer: drop the redundant mention button, leave one addressing control'
seq: 53
status: done
priority: p2
created: 2026-09-04T19:57:33.607917Z
sprint: 122
closed: 2026-09-04T19:58:57.445238Z
---

The composer had two ways to address one agent: the recipient dropdown (which names who an unaddressed message goes to) and an @ button whose only effect was to type '@' into the textarea. Remove the button; typed '@name' still overrides the selection for one message and the mention list still completes it.
