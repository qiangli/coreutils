---
id: e3a421a9de08
kind: task
title: 'Meet and Chat: take a message back — hold, cancel, retract'
seq: 56
status: done
priority: p1
created: 2026-09-04T20:42:49.033446Z
sprint: 122
closed: 2026-09-04T20:43:27.039191Z
---

One control (the spinning send button). A 5s hold before dispatch makes 'not sent' truthful; after dispatch the SERVER decides the verdict — canceled if nothing was appended, retracted if the record exists. Retraction is an appended event referencing the original, never a delete.
