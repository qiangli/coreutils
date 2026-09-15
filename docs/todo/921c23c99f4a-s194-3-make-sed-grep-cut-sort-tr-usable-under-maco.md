---
id: 921c23c99f4a
kind: task
title: S194.3 make sed grep cut sort tr usable under macOS LANG=en_US.UTF-8
seq: 145
status: assigned
priority: p0
created: 2026-09-15T12:05:21.468931Z
weave: 28
assignee: qiangli
sprint: 194
---

Close umbrella todo #384. Obtain and record cert-arm acknowledgement first. Outside VSC_PROFILE=cert, accept the default macOS LANG=en_US.UTF-8 with honest UTF-8/byte semantics for sed, grep, cut, sort and tr; preserve the reviewed certification locale/provider behavior and negative tests. Do not globally accept arbitrary locale names. Gate: affected package tests, scripts/ci-test-gate.sh, scripts/crossvet.sh, applicable cert-profile gate, and installed bashy five-command matrix.

Certification review NACKed any shared relaxation and required this boundary:
the macOS default is carried only outside `VSC_PROFILE=cert`; certification
remains fail-closed with its reviewed locale/provider matrix and negative tests.
