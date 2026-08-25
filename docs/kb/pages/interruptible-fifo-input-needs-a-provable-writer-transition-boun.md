---
type: gotcha
title: Interruptible FIFO input needs a provable writer-transition boundary
description: When emulating blocking named-FIFO input with O_NONBLOCK for signal cancellation, prove that the target kernel retains a writer open-close transition before first read. Linux latches POLLHUP and supports an exact poll state machine; XNU can lose the transition, so pathname FIFO input must fail explicitly unless a safe native cancellation boundary is available. Never cancel blocking open in an abandoned goroutine because unlink or rename can make it unreleasable.
status: validated
evidence: Validated by cmds/dd Linux 500-iteration no-sleep immediate writer-open-close stress in ubuntu:24.04, 20x unlink/rename plus SIGINT descriptor/goroutine leak checks, Darwin deterministic-refusal tests, focused race, and windows/linux/darwin crossvet.
source:
    tool: codex-gpt5.6-sol-g
    host: dragon
    episode: weave-issue-683
created: "2026-08-25T08:18:55Z"
updated: "2026-08-25T08:19:01Z"
---
