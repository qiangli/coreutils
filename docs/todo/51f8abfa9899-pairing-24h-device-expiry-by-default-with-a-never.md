---
id: 51f8abfa9899
kind: task
title: 'Pairing: 24h device expiry by default, with a never-expire choice'
seq: 58
status: done
priority: p2
created: 2026-09-04T20:42:49.078751Z
sprint: 122
closed: 2026-09-04T20:43:27.082323Z
---

Default device TTL 4h -> 24h, a Settings control (4h/24h/7d/30d/never), zero hours = never (stored as a century-out date, not a sentinel), and an explicit TTL now extends the operator grant instead of being silently clamped to 12h.
