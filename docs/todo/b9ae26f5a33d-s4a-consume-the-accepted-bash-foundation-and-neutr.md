---
id: b9ae26f5a33d
kind: refactor
title: S4A Consume the accepted Bash foundation and neutral error contract
seq: 190
status: assigned
priority: p0
created: 2026-11-18T01:12:56.240539Z
weave: 3
assignee: agy-gemini3.8-flash
sprint: 575
sprint_id: 65ce85f5-87c0-5863-bf55-62de86f5df36
sprint_title: 'Layer separation: Classic <- Bash# <- Yoke import-graph delineation'
---

Sprint575 source-preserving package/reference migration and reviewed neutral error seam. Prerequisite: product story94d888e23283 preserves frozen real worker-envelope tests under bashy/integration/weavecli, verified before removing their lower-module copy. Current coreutils6dd7a3ee main is clean/synced; accepted foundation pin is github.com/qiangli/bash v0.0.0-20261118005152-72c4bd210f74. This slice establishes reference ownership, not final interpreter stripping/import-graph closure.

Replace non-vendored Go imports of mvdan.cc/sh/v3 neutral shell/interpreter packages with github.com/qiangli/bash corresponding packages. In pkg/weavecli/envelope.go and foreign_error_test.go use the accepted stdlib-only errdetail leaf instead of polyglot.ErrorDetail/ForeignErrorDetail. Keep all mapper logic, exact JSON fields/rendered message, nil/ordinary error behavior and eight-cause limit unchanged. Remove foreign_worker_test.go ONLY after the manager confirms its exact assertions/helper are preserved in the product story above. Coreutils owns neutral decoder/mapper tests; do not import Bashsharp or copy its worker implementation or add new exported hook frameworks. No policy/default/provider/inventory changes, no unrelated fixes or new dependencies upward.

Update go.mod/go.sum to the exact accepted foundation version and remove the old sh require/replace; no local replaces, siblings or go.work. Only metadata changes needed by the new module identity; no unrelated upgrades. Go metadata download/tidy is allowed locally; no Go build/run/test. Preserve all remaining source and test assertions under the declared import transforms, with no skips/ratchet widening/baseline changes. Do not mass-edit historical docs/comments, vendored third_party sources, fixture semantics or KB.md.

All builds, full suite, go vet and cross-OS compile gates run through bashy DAG in manager's locked official image as ordinary user; no native development-box test/foreign suites. The manager will additionally verify source equivalence and downstream builds against published fixed pins at cutover. Do not run sibling consumer overlays. Work only in isolated workspace, no pushes/branch switches/root or sibling edits. Commit with Sprint575/Story/Story-ID and actual agent, report exact reference transforms, module metadata changes and SHA. If any gate fails, manager identifies named root cause before further edits.
