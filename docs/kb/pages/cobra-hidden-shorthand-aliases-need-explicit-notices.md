---
type: gotcha
title: Cobra hidden shorthand aliases need explicit notices
description: When retiring a shorthand flag in Cobra/pflag, ShorthandDeprecated may hide it from help but does not reliably print a user-visible replacement notice in this repo version. Bind a hidden compatibility flag with the shorthand and check cmd.Flags().Changed(<alias>) in RunE when the contract requires a one-line stderr notice.
status: candidate
source:
    tool: codex-gpt-5.5-y
    host: dragon
    episode: weave-issue-25
created: "2026-08-26T09:34:15Z"
---
