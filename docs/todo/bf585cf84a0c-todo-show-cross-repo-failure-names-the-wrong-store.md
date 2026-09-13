---
id: bf585cf84a0c
kind: task
title: todo show cross-repo failure names the wrong store and never mentions --base-dir
seq: 80
status: todo
priority: p3
created: 2026-09-06T11:30:33.949917Z
sprint: 130
---

From the umbrella, bashy todo show babd0878 fails with:

  bashy todo: no issue "babd0878" in the register (bashy issue list)

Two problems in one line. It names the ISSUE register, a different store from
the todo store the command just searched, sending the reader to a command that
will not find it either. And it never mentions --base-dir, which is a global
flag on todo and does exactly what is wanted:

  bashy todo show babd0878 --base-dir <repo>   # works

COST 2026-09-06: small but repeated. Every cross-repo story lookup while
closing five sprints across dhnt, coreutils and bashy hit this and was resolved
by cd-ing into the right repo, which the hint engine then correctly complains
about.

DO: when an id does not resolve in the current store, say so in terms of the
store that was searched and name --base-dir. If it is cheap, resolve the id
against the tracked story roots of any sprint the caller is looking at and say
which repo holds it - the sprint card already knows.
