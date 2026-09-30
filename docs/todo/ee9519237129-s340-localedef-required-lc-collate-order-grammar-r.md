---
id: ee9519237129
kind: bug
title: S340 localedef required LC_COLLATE order grammar residue
seq: 163
status: done
priority: p1
created: 2026-09-30T21:38:09.933169Z
assignee: s340-collate-grammar-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
closed: 2026-09-30T22:29:28.209489Z
closed_by: codex-gpt6-astra
---

Independent review of part3 recovery found defined LC_COLLATE forms rejected: order ellipsis, forward/backward with position, and empty weight operands; omitted coded characters also error at Compare rather than warning plus appended order. Source: POSIX.1-2017 XBD7.3.2, official https://pubs.opengroup.org/onlinepubs/9699919799.2018edition/basedefs/V1_chap07.html (ordinary URL403). Bounded8pt30m correction after part3 and CLI162 because shared store/compiler files. Acceptance independently run valid-form parser/compiler and ordering regressions, omitted-character warning and forced compilation rules, position+IGNORE order, ellipsis endpoint/range and empty weights; no upstream code copying/translation/paraphrase. Preserve custom consumer tests and exact unsupported residuals; no fullbase support claim before required forms pass. Multi-character bracket backend is separately tracked, do not silently approximate it.

Candidate7aa286e9 supports explicit UNDEFINED IGNORE and selected weights; retain and extend that support rather than claiming all UNDEFINED forms fail. Remaining missing-character behavior without an explicit UNDEFINED is still a required correction.

Final independent review after combined854b4678: literal character identifiers/weights and literal/mixed quoted weight strings still rejected. POSIX XBD7.3.2 explicitly permits characters in any Locale Definition form and concatenated characters or symbols for one-to-many weights. Keep story OPEN; run27 recovery has remaining original cap until22:28:45UTC for exact scalar/byte/literal equivalence regressions, no broad local suite. All earlier passing evidence remains scoped, not full grammar acceptance.
