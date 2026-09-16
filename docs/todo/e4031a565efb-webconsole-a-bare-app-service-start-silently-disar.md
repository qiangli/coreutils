---
id: e4031a565efb
kind: task
title: 'webconsole: a bare ''app service start'' silently disarms phone pairing'
seq: 147
status: todo
priority: p1
created: 2026-09-16T17:01:52.827335Z
sprint: 202
---

Observed on the operator's dev host 2026-09-16: after `make install` an agent
session ran `bashy app service stop` then a bare `bashy app service start`.
The console came back as `apps serve --port 22749 --bind 127.0.0.1` — no
`--pair`, no LAN bind — so the LAN listener never opened and the two paired
phones (grant = never expires) could not reach it. Outpost's supervisor only
restarts on "stopped"; status read "running", so it never re-armed. Fixed by
`app service stop` and letting outpost relaunch (`--pair --bind <lan-ip>`).

Gap: `service start` takes its arming from flags only, while the pairing
store (`~/.bashy/console/pairing.json`) already knows there are live paired
devices. A restart by any hand (human, agent, installer) loses the arming
and nothing reports it.

Candidates (pick one, keep fail-closed):
- `service start` without `--pair` while the store holds live devices → warn
  loudly with the exact re-arm command (cheapest; no behaviour change).
- persist the last launch options (pair, bind) beside the pidfile and reuse
  them when `start` is given neither flag; explicit flags still override.
- `service status` reports `pairing: armed|off` so the supervisor (and a
  human) can see the drift.
