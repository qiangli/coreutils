---
id: 8302f07a2eb7
kind: bug
title: rm -f fails when the file vanishes between the first check and the unlink (GNU rm -f ignores it)
seq: 193
status: todo
priority: p2
labels:
    - rm
created: 2026-11-26T16:30:00.197231Z
---

Found 2026-11-26 in umbrella Sprint 731 (story b390ac728272). cmds/rm/rm.go ignores a missing operand under -f at the first lstat (the fs.ErrNotExist branch), but when another process removes the file after that check, the unlink's ENOENT is reported as 'cannot remove ...: No such file or directory' and rm exits 1. GNU rm -f treats ENOENT at the unlink as success as well. Seen in bashy examples/desk/form 'stop': the script signals its server and then runs rm -f on the server's state file, which the dying server also removes; about 2 of 8 stops lost the race and exited 1. The form script was made independent of this. Not fixed here: coreutils is on the frozen baseline; this needs its own small, baseline-proven repair with a regression test (a file removed between the check and the unlink).
