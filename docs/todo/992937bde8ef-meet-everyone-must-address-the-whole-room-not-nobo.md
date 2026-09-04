---
id: 992937bde8ef
kind: task
title: 'Meet: ''Everyone'' must address the whole room, not nobody (and not the sprint''s conductor)'
seq: 40
status: done
priority: p0
created: 2026-09-04T09:40:25.283819Z
sprint: 122
closed: 2026-09-04T09:41:36.174354Z
---

REPORTED: a message with no recipient shows in the transcript but is not dispatched.

MEASURED, and worse in one case than reported:
- ORDINARY ROOM, no addressee (To=""): every seated reader does see it in the unified inbox, but as 'other' (room history), never as directed work — and meet.Dispatch wakes only on directed mail, so nobody is woken. Reaching the transcript and no accountable inbox is indistinguishable, from outside, from every agent ignoring you.
- SPRINT ROOM: PostAs falls back to State.DefaultTo when 'to' is empty, so a blank addressee became conductor:N. Directed mail is filtered OUT of every other reader's history bucket (addressedEvent in board.go), so the message reached the conductor ALONE and the other participants saw NOTHING AT ALL — while the composer labelled the choice 'Everyone'. That label was mine, added the previous session; the redirect predates it.
- The web /post route dropped 'to' entirely, so the browser could not address anyone even if it wanted to. Only the CLI could (meet tell --to).

FIX: an EXPLICIT broadcast addressee, meet.AllSeats == "all". PostAs records it verbatim and skips the DefaultTo fallback; directedEvent treats it as directed for every reader; handlePost accepts 'to'; the composer's Everyone sends it. The dropdown no longer offers a blank.

WHY NOT 'blank means everyone': dispatch.go's termination rule. A turn is recorded with NO addressee, so if 'no addressee' also meant 'everybody', each of N replies would wake the other N-1 forever. An explicit broadcast wakes each participant ONCE and terminates for exactly that reason — the replies it provokes are unaddressed. Rounds remain the chaired 'everyone speaks'; a broadcast is mail.

GATE: 5 pkg/meet broadcast tests (including the sprint-seat redirect and a two-pass dispatch proving termination), 17/17 Playwright (1 new, asserting the posted body carries to=all), crossvet PASS, ci-test-gate OK. Verified live on sprint room 10: both seated agents see the post with to='all' in their unified inbox.
