---
id: 818e47df92a2
kind: bug
title: S340 localedef CLI validation and store path corrections
seq: 161
status: done
priority: p1
created: 2026-09-30T20:58:24.427841Z
weave: 24
assignee: s340-localedef-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
closed: 2026-09-30T21:19:14.33185Z
closed_by: codex-gpt6-astra
---

Bounded5pt20m correction prerequisite to part2 story9f8ce58c4dd7 acceptance. In YOUR isolated coreutils workspace fetch/cherry-pick preserved run20 branch latest80b4b2d557a39b71ba3f4bb1ad275e365ce6a891 from /Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-20 read-only. Exact defects cmds/localedef/localedef.go: code4 validation errors can write with -c; warning behavior must follow POSIX; -u silently relabels compiled.Charmap without conversion; relative LOCPATH Save uses process cwd not rc.Dir and consumers must agree. Consult POSIX2017 official text, red/green tests for error+force/no-force warning/encoding mismatch/relative paths; preserve existing output on failure. Keep locale -k distinctive numeric/messages and sort tests passing. No part3/collation/manifest switch. Clean-room POSIX docs+blackbox only; no copying/translating/paraphrasing upstream GNU/glibc/CUPS code. Focused pkg/locale pkg/localedef cmds/localedef cmds/locale cmds/sort normal/race and platform build. Use assigned cache, no bypass; exact blocker if sandbox prevents testing. Actual assigned identity plus Sprint/Story/Story-ID trailers, no shared edits/merge/push/install. Commit partial at20m cap.
