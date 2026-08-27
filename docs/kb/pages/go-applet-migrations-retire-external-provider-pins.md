---
type: lesson
title: Go applet migrations retire external provider pins
description: When ed, patch, mail/mailx, or talk are shipped as pure-Go applets, remove their provider manifest rows, recipes, cache paths, tests, and fallback so the manifest contains only active runtime owners.
tags:
    - posix
    - providers
    - go-applets
status: validated
evidence: Manifest contains exactly 10 active providers (make and bc later joined ed/patch/mail/mailx/talk as pure-Go applets); absence and OwnerGoApplet/SelGoApplet regressions pass; broad non-external go test and scripts/crossvet.sh pass.
source:
    tool: codex-profile-d-sprint
    host: dragon
created: "2026-08-26T23:34:38Z"
updated: "2026-08-26T23:34:43Z"
supersedes: retained-provider-pins-need-a-separate-active-owner-set
---

The earlier retained-control policy is no longer repository policy. A migrated command has one shipped owner: its Go applet. Remove the old provider row, build recipe, cache/provenance expectations, and provider-specific tests and documentation. Keep provider and gate counts equal to the active manifest. The applet-matrix generator must reject a provider row that collides with a shipped Go package. Historical differential comparisons can use explicitly external fixtures outside the shipped provider registry; they must not create a second product owner or fallback.
