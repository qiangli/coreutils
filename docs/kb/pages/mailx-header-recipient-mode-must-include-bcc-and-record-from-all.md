---
type: lesson
title: mailx header-recipient mode must include Bcc and record from all recipients
description: When mailx parses recipients from message headers, include Bcc along with To and Cc, and choose the record target from the first actual recipient across the combined list. Otherwise a Bcc-only -t message can fail with no recipients, and -F can panic or skip recording when To is empty. Applies to the local-file-only mailx applet.
status: candidate
source:
    tool: codex-gpt5.4-mini-w
    host: dragon
    episode: weave-issue-23
created: "2026-08-27T18:03:22Z"
---
