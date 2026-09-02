---
id: f365f03d0142
kind: task
title: 'sprint lease: record the process holding a seat open, and stand it down on detach'
seq: 35
status: done
created: 2026-09-02T19:49:16.929403Z
sprint: 105
closed: 2026-09-02T19:50:33.376884Z
---

A conductor lease read healthy with nothing running. The heartbeat is the only evidence a seat has and it is a claim about a MOMENT: an attached watch beats every TTL/3, so a killed watch leaves a beat that is still inside the window, and detaching wrote nothing at all. Record the holding process when there is one (cleared by every ephemeral refresher, whose pid dies with the command), withdraw the heartbeat in seat() when that process is gone, and add a holder-checked release so detaching is written down.
