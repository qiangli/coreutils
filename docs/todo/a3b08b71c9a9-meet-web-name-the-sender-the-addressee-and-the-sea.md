---
id: a3b08b71c9a9
kind: task
title: 'Meet web: name the sender, the addressee, and the seat'
seq: 48
status: done
priority: p2
created: 2026-09-04T17:12:15.05208Z
sprint: 122
closed: 2026-09-04T17:12:55.013934Z
---

The room showed answers to questions nobody could see: Address recorded only the reply, so a human's addressed message never entered the transcript. Every agent turn also carried the same role badge ('participant'), which distinguished nobody, and the 1:1 chat offered a mention control whose syntax it does not parse.

Deliver: (1) record the question addressed to the agent before the turn runs, and acknowledge it so dispatch cannot ask twice; (2) every room message names from and to (agent, everyone, or the room); (3) one seat resolver behind the transcript, the composer and the roster, so a named seat is shown in the room's own words; (4) the chat states its recipient as @agent in the room's recipient slot and offers Start work as its only other action.

Gate: pkg/meet go test, crossvet, and the Playwright browser suite with new tests for each of the three.
