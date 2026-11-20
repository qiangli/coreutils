---
id: 8d8c1091d947
kind: refactor
title: S575 preserve Coreutils source and rewrite only Bash module references
seq: 191
status: done
priority: p0
created: 2026-11-18T04:09:41.155601Z
assignee: s575-codex-manager
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
closed: 2026-11-20T12:26:26.859455Z
closed_by: s575-codex-manager
---

Mechanical module reference cutover only: all old sh imports point unchanged full qiangli/bash engine. Preserve implementation and all tests including foreign worker assertions. No shared-contract or DTO redesign, no sh stripping. Run official-image go build ./... with GOWORK=off.

Regression result: all Go packages passed except TestLayerImports. Root cause: its shrink-only allowlist and banned package prefixes still named mvdan.cc/sh/v3 after the import namespace moved to github.com/qiangli/bash. Classification: relocation reference. Rename those package references only; preserve the allowlist cardinality, story marker, and assertions.

## Manager closeout — 2026-11-20
Reference-only imports published at ababb9f1, with the exact namespace correction at 8de3088. Retained focused TestLayerImports passed; other package evidence is reused only for unchanged inputs. Foreign worker assertions stayed in place; the rejected neutral-contract redesign and test deletion were not replayed.
Existing receipts and source ancestry were reviewed; no new test/build ran during reconciliation. Evidence belongs to its exact historical source/artifact identities, not arbitrary later heads. S575 full-run evidence remains under posix-baseline/s575-2026-11-19 with its raw 4 FAIL, 54 UNRESOLVED and 9 INSPECT unchanged. RC38 is on operator hold.
