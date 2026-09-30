---
id: 818e47df92a2
kind: bug
title: S340 localedef CLI validation and store path corrections
seq: 161
status: assigned
priority: p1
created: 2026-09-30T20:58:24.427841Z
weave: 24
assignee: s340-localedef-astra
sprint: 340
sprint_id: 07abaf0d-4c54-57e7-b324-8c926b4c300f
sprint_title: Pure-Go m4, localedef and lp; listing view for the optional external POSIX tools
---

Bounded5pt20m correction prerequisite to part2 story9f8ce58c4dd7 acceptance. In YOUR isolated coreutils workspace fetch/cherry-pick preserved run20 branch latest80b4b2d557a39b71ba3f4bb1ad275e365ce6a891 from /Users/qiangli/.bashy/weave/coreutils-909dd8b2/workspaces/issue-20 read-only. Exact defects cmds/localedef/localedef.go: code4 validation errors can write with -c; warning behavior must follow POSIX; -u silently relabels compiled.Charmap without conversion; relative LOCPATH Save uses process cwd not rc.Dir and consumers must agree. Consult POSIX2017 official text, red/green tests for error+force/no-force warning/encoding mismatch/relative paths; preserve existing output on failure. Keep locale -k distinctive numeric/messages and sort tests passing. No part3/collation/manifest switch. Clean-room POSIX docs+blackbox only; no copying/translating/paraphrasing upstream GNU/glibc/CUPS code. Focused pkg/locale pkg/localedef cmds/localedef cmds/locale cmds/sort normal/race and platform build. Use assigned cache, no bypass; exact blocker if sandbox prevents testing. Actual assigned identity plus Sprint/Story/Story-ID trailers, no shared edits/merge/push/install. Commit partial at20m cap.


Worker correction evidence (s340-localedef-astra, 2026-09-30)

Preserved full part2 at 80b4b2d5. Copied this canonical story into the isolated
workspace for commit provenance; no canonical status mutation.

Regression tests reproduced output creation/overwrite after forced validation
errors, unforced warnings, unsupported codeset relabeling, and relative store
resolution against process cwd. Corrections prevent those writes, require -c
for warning output, refuse -u explicitly with status 2, and resolve store paths
through rc.Path for compiler, copy, locale listing/keywords and numeric sort.
The inherited UTF-8 sort test incorrectly expected 1.9 before 1.20; replaced its
input with distinctive thousands grouping (1000.5 versus 900).

Terminal worker evidence: normal and race tests passed for pkg/locale,
pkg/localedef, cmds/localedef, cmds/locale and cmds/sort. CGO_ENABLED=0 go vet
passed for the same five packages with windows/amd64, linux/amd64 and
darwin/arm64. These are scoped worker checks, not authoritative merge gates.

POSIX.1-2017 localedef OPTIONS and EXTENDED DESCRIPTION require -u target
codeset mapping. This remains unsupported, so full localedef acceptance is
NOT claimed. Source: https://man7.org/linux/man-pages/man1/localedef.1p.html
(IEEE/Open Group text reproduction; official pubs.opengroup URLs returned 403).
CONSEQUENCES OF ERRORS requires no permanent output for errors, and output
for warnings with -c; this implementation refuses unforced warnings (status 4).
No upstream implementation source consulted. No push, merge or install.
