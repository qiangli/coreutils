---
type: lesson
title: pax cpio append and update require trailer-preserving rewrite
description: POSIX pax -a and -u cannot append bytes after a cpio TRAILER!!! record. For the ODC cpio format, decode existing members, preserve their original header fields and data, emit new/update members, then write one final trailer; update filtering uses archived member mtimes. Before opening with O_TRUNC, reject a requested/default output-format mismatch and readable newc/crc inputs that the ODC-only rewrite encoder cannot preserve.
status: candidate
source:
    tool: codex-gpt5.6-luna-u
    host: dragon
    episode: weave-issue-21
created: "2026-08-27T17:36:32Z"
---
