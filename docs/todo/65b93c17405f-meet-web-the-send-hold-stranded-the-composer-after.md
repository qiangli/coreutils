---
id: 65b93c17405f
kind: task
title: 'meet web: the send hold stranded the composer after one message'
seq: 59
status: done
created: 2026-09-04T22:48:35.475367Z
sprint: 122
closed: 2026-09-04T22:49:46.740205Z
---

The pending-send record kept for the recall offer was never cleared: the send button stayed a Recall button and the hook refused every later send, silently, after the composer had already emptied the box. Bound the recall offer, let typing reclaim the control, clear pending on a failed dispatch, and shorten the hold from 5s to 3s.
