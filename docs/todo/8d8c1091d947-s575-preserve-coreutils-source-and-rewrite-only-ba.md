---
id: 8d8c1091d947
kind: refactor
title: S575 preserve Coreutils source and rewrite only Bash module references
seq: 191
status: assigned
priority: p0
created: 2026-11-18T04:09:41.155601Z
assignee: s575-codex-manager
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
---

Mechanical module reference cutover only: all old sh imports point unchanged full qiangli/bash engine. Preserve implementation and all tests including foreign worker assertions. No shared-contract or DTO redesign, no sh stripping. Run official-image go build ./... with GOWORK=off.

Regression result: all Go packages passed except TestLayerImports. Root cause: its shrink-only allowlist and banned package prefixes still named mvdan.cc/sh/v3 after the import namespace moved to github.com/qiangli/bash. Classification: relocation reference. Rename those package references only; preserve the allowlist cardinality, story marker, and assertions.
