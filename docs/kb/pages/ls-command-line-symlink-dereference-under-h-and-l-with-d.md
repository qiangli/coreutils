---
type: lesson
title: ls command line symlink dereference under -H and -L with -d
description: When implementing command-line symbolic link dereferencing under -H and -L (e.g. in ls), ensure operands are dereferenced to their target metadata even when -d is specified, while keeping dangling symlinks to fall back to the link itself.
status: candidate
source:
    tool: agy-gemini3.5-flash-r
    host: dragon
    episode: weave-issue-772
created: "2026-08-26T03:01:20Z"
---
