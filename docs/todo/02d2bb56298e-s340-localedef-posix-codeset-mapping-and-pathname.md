---
id: 02d2bb56298e
kind: bug
title: S340 localedef POSIX codeset mapping and pathname output
seq: 162
status: done
priority: p1
created: 2026-09-30T21:06:53.540307Z
assignee: s340-codeset-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
closed: 2026-09-30T22:29:28.065716Z
closed_by: codex-gpt6-astra
---

Required localedef base-interface residue discovered in independent review; <=8pt30m after part2 accepted. -u code_set_name must map UCS position constants in charmap to an implementation-supported target codeset (start UTF-8 and ASCII, document supported aliases, reject unsupported code sets with2/no output). No blind relabel or blanket unsupported for supported codesets. Name operand with slash must write at that pathname, readers must support locale path per specification; successful compilation reports processed categories stdout. Source POSIX2017 localedef OPTIONS/OPERANDS/STDOUT/EXTENDED DESCRIPTION (official pubs.opengroup 403; licensed text https://man7.org/linux/man-pages/man1/localedef.1p.html). Acceptance red/green target mapping ASCII/nonASCII UTF8/out-of-range and pathname+rc.Dir readback with locale/consuming utility; preserve -c error/warning rules and host fallback. Clean-room specs and blackbox only, no upstream code copying/translation/paraphrase. Gate focused localedef/locale/store/consumer normal/race and cross-platform, no unsupported full-completion claim. Serialize with part3 if shared files; no implementation before assigned.
