---
id: 5dd0d7e03e9e
kind: bug
title: Support Unicode aliases in localedef default ASCII charmap
seq: 176
status: todo
priority: p0
created: 2026-10-01T16:24:04.105235Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Open Group VSC DOC/07.config.mm requires configured locale definition to compile with -f without warnings and also be usable with the default character mapping. Controlled C host probe: Go localedef with portable glibc POSIX source and explicit ASCII -f succeeds, but same -i source without -f fails rc4 with undefined <U0041> etc because DefaultCharmap only includes POSIX symbolic names. Add U0000..U007F aliases, retain existing names and undefined-name errors; verify both compile paths.
