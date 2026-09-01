---
id: 05817b6ac20b
kind: task
title: 'webconsole: sign-in panics on a loopback console, and the login eye toggle never swaps'
seq: 24
status: todo
priority: p0
created: 2026-09-01T18:53:43.945476Z
assignee: ci-repair
sprint: 101
---

Three defects found while verifying the uncommitted apps-surface work.

1. PANIC on sign-in. handleLogin calls s.sessions.Mint, but cmd.go builds the
   session store only when binding off-loopback, so s.sessions is nil on a
   loopback console and Mint dereferences a nil receiver. The response aborts
   mid-flight and the page after signing in is BLANK. handleLogout, directly
   below it, already guarded with s.sessions != nil. Survived every test
   because a WRONG password returns 401 long before that line and the e2e
   suite always constructs a store -- only a correct password on loopback
   reaches it.

2. The login password eye toggle showed BOTH icons and never swapped. Two
   stacked causes: the UA [hidden]{display:none} does not apply to an inline
   <svg> (measured: removing the author rule gives eye=block eye-off=block),
   and 'hidden' is an IDL attribute of HTMLElement which SVGElement does not
   inherit, so assigning it set a meaningless expando.

3. The e2e-tagged suite did not COMPILE: pair_http.go and e2e_test.go both
   declared truncate, both committed. So the sign-in round-trip guarantee that
   039f24ef says it proves has not been runnable.
