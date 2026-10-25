---
id: b2654a7dc07f
kind: bug
title: ps rejects the BSD form 'ps -axo pid,etime,command' (unknown shorthand flag x)
seq: 188
status: todo
priority: p3
created: 2026-10-23T17:35:09.64562Z
---

Found 2026-10-23 on macOS: 'ps -axo pid,etime,command' -> 'ps: unknown shorthand flag: x in -xo'. Agents on macOS commonly use the BSD form. Triage: support -a -x and combined -axo in the bashy ps, or route BSD-style invocations to a compatible parse; ps is not in the POSIX-required set, so check where this ps lives (coreutils vs yoke) before fixing.
