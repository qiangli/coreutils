---
id: b66bb36def3f
kind: bug
title: Restore Coreutils Linux test workflow before Sprint 341 certification freeze
seq: 166
status: todo
priority: p0
created: 2026-10-01T05:32:09.624479Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Current 5379915 test run 36812030027 fails on Linux in getconf POSIX2_VERSION, m4 TestDifferentialCorpus, and localedef TestSystemSource. Fix regressions without weakening base POSIX behavior. Acceptance: focused Linux tests and full test workflow pass on pushed exact candidate, with macOS and Windows jobs green.
