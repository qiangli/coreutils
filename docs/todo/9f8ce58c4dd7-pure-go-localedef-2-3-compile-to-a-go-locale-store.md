---
id: 9f8ce58c4dd7
kind: feature
title: 'pure-Go localedef (2/3): compile to a Go locale store read by pkg/locale and the locale applet'
seq: 156
status: assigned
priority: p1
labels:
    - posix-cert
created: 2026-09-30T15:34:41.569637Z
weave: 20
assignee: s340-localedef-sol
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

S340 localedef part2 correction8pt/25m. Preserve run20 c69da490 and all earlier unique work. Manager source review exact defects: cmds/localedef/run allows code4 validation errors to compile/write when -c; warnings without -c also write despite warning handling contract; -u blindly assigns compiled.Charmap without required codeset conversion or honest unsupported error. Check POSIX.1-2017 localedef specification and add meaningful red/green cases for warning/error/-c and incompatible -u, preserving valid existing output on failure. Store path must honor rc.Dir for relative LOCPATH (writer currently Save(dir) process-relative); verify reader/consumer path consistency. Maintain locale -k distinctive decimal/yes-no and sort consuming utility; normal/race focused pkg/locale pkg/localedef cmds/localedef cmds/locale cmds/sort and cross-platform checks. No part3/collation or multicall/manifest switch until part2 independently accepted. Scope only localedef/store/locale and necessary consumer/tests. Clean-room POSIX docs plus black-box only: never copy/translate/paraphrase GNU/glibc/CUPS code. Use assigned cache; no hook/sandbox bypass; report exact failures. Actual s340-localedef-sol identity; Sprint:340 Story:156 Story-ID:9f8ce58c4dd7. No canonical edits/merge/push/install; commit partial and report unmet at cap.
