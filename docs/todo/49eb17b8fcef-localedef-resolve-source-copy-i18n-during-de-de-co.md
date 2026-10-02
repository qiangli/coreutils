---
id: 49eb17b8fcef
kind: bug
title: 'localedef: resolve source copy i18n during de_DE compilation'
seq: 174
status: todo
priority: p0
created: 2026-10-01T14:13:26.498251Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Profile C old-candidate POSIX08 localedef set36 has seven FAILs (TP10,20,32,33,36,39,40) with one shared root: de_DE.src LC_CTYPE copy "i18n" returns exit4 saying locale i18n has not been compiled. Fix the Go localedef source-copy resolution against the VSC fixture and official POSIX semantics, without shelling out or relying on host locale tools. Add a focused regression and rerun localedef with the final candidate before D. TP52 UNRESOLVED is the known Linux empty-symlink setup issue and requires separate disposition.
