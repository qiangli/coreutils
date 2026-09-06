---
id: f5a2f2ca2b19
kind: task
title: 'GATED: seam 2 — bound the agent-facing shell output writer without changing data paths'
seq: 66
status: todo
priority: p1
created: 2026-09-06T05:34:16.856877Z
sprint: 123
---

AUTHORIZED by the operator direction to run Sprint 123. SPEC: umbrella docs/command-output-reduction-design.md and story 6a9e03eed927 corrections. Own the arbitrary shell execution seam in bashy/internal/agentos/WireExec plus reducer integration tests. Install one composed stdout/stderr writer above the POSIX early return for the full bashy binary while cmd/bash remains structurally free of AgentOS and pkg/reduce. Do not alter pipes, redirects, command substitution, or the secrets render data path. Reconcile the existing dry-run interp.StdIO override rather than appending a second StdIO option. Cover external commands and in-process coreutils, stdout and stderr, BASHY_AGENTIC activation, BASHY_OUTPUT_REDUCE, --no-elide/env/prefix escape hatches, --posix agent mode permitted, certification mode off, and deterministic digest recovery. GATE: focused bashy agentos tests, cmd/bash import-graph test, shell pipe/redirection/substitution byte-fidelity tests, coreutils reducer tests, and script/output-reduction-eval at 100% retention with determinism PASS and real BPE reporting. No sh API change unless separately authorized; no content-graph/cache/index/background parsing scope.
