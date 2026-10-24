---
id: 8bc0d70e1edd
kind: bug
title: 'coreutils: an unavailable locale (e.g. LANG=en_US.UTF-8) makes ~41 utilities exit 1 (date, ls, cut, grep, ps, ...); must behave as if unset (C, or C.UTF-8 for UTF-8 codesets), silently, per POSIX XCU 8.2'
seq: 187
status: done
priority: p0
labels:
    - release
created: 2026-10-23T17:34:58.538718Z
weave: 14
assignee: codex-gpt6-sol
sprint: 458
sprint_id: 3b16e60f-dce3-5dd1-8ed1-4384110d0741
sprint_title: 'bashy is all you need, MVP: invariant core + every other command as editable Bash# skill/script'
closed: 2026-10-23T19:49:28.000391Z
resolution: fixed
closed_by: codex-gpt6-sol-n
---

OPERATOR: top priority, fix now (2026-10-23). Moved into Sprint 458 by operator direction.

Repro (production Linux host, login shell bashy, LANG=en_US.UTF-8; also macOS dev box):
- `ls` -> "ls: LC_COLLATE=en_US.UTF-8: collate: unsupported locale; only the ISO-8859-1 aliases ... are accepted"
- `cut` -> "LC_CTYPE \"en_US.UTF-8\" is unavailable; supported locales are C/POSIX, their UTF-8 aliases, and de_DE.ISO-8859-1"
- `grep` -> "LC_CTYPE=en_US.UTF-8: unsupported locale; expected C, POSIX, de_DE.ISO-8859-1, or de_DE.iso88591"
- `ps`, `date` -> "LC_TIME locale \"en_US.UTF-8\" is unavailable: embedded time data is limited to C/POSIX and de_DE"
About 41 sources carry such refusals (grep 'unsupported locale|is unavailable|supported locales are|ISO-8859-1 aliases' in coreutils, including pkg/ctype, pkg/locale, cmds/{date,grep,cut,ls,awk,expand,join,od,expr,more,time,fold,paste,...}).

POSIX XCU 8.2: if a locale environment variable has an invalid or unavailable value, the utility behaves as if none were set (the POSIX locale). GNU silently falls back. Required:
1. One central resolver (pkg/locale or pkg/ctype) maps an unavailable locale to C, except that a UTF-8 codeset (en_US.UTF-8, *.utf8) maps to the C.UTF-8 character model that the UTF-8 aliases already use. Exit status and output are then identical to the C/C.UTF-8 run. No per-command refusal and no stderr noise.
2. Keep the supported locales exactly as today: C, POSIX, the UTF-8 aliases, de_DE.ISO-8859-1 (the certification test locale), and the localedef/libc-bound providers (commits b24d3a76, 3dedce4a: the certification locale path must not change). An explicitly requested locale that the cert harness provisions must still bind as before.
3. Do not weaken `unsupported flags fail loudly`; this is locale availability only.

Acceptance, red/green: a table test over every affected command with LANG and LC_ALL set to en_US.UTF-8 (and a bogus xx_YY.FOO) that exits 0 with output equal to the LC_ALL=C (or C.UTF-8) run; existing locale tests unchanged; `go test ./...` in coreutils; `go build ./...`. Then the manager runs the certification-relevant gates on a remote host (no foreign suites natively on the dev box; see umbrella docs/conformance-test-landmines.md). coreutils is the certified package: bug fix only, minimal diff, no unrelated refactors. Public repo: no hostnames or user ids.
