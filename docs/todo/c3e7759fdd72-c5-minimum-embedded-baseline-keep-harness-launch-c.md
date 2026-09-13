---
id: c3e7759fdd72
kind: task
title: 'C5 minimum embedded baseline: keep harness launch contracts, drop embedded models+agents (after U4)'
seq: 114
status: assigned
priority: p1
created: 2026-09-12T23:03:30.161218Z
weave: 11
assignee: qiangli
sprint: 161
---

Minimum embedded baseline — fish to rod. Shrink coreutils/pkg/fleet/baseline/ (embed.go:16) to what the mechanism cannot work without: the TOOL LAUNCH CONTRACTS (tools/*.yaml — measured wire contracts with the harness CLIs; keep) and ZERO models, ZERO agents.
Order: (a) umbrella story U4 exports the current 27 models / 39 agents to the operator-owned overlay (fleet/{models,agents}/ via BASHY_MODELS_PATH/BASHY_AGENTS_PATH) and/or they arrive via model|agent sync; verify the dev-host `bashy agent list` count with the overlay mounted; (b) only then delete the embedded yaml; (c) bashy model|agent list on an empty ring prints a one-line rod hint (add, sync, BASHY_*_PATH) and exits 0 — never an error; (d) agentlaunch.SeededProfiles remains the headless floor for an uncatalogued tool.
Rewrite tests that assumed baseline agents/models (band_test.go "every baseline agent resolves", fleet_test.go, agentlaunch/launch_test.go, pkg/capability, pkg/meet roster tests) to load a testdata/ ring — testdata is not shipped content.
Decision points to record in the spec: whether hermes/kimi-code/openclaw/cline/gemini/goose contracts (declared-not-measured / detection-only) stay embedded; whether the installed-host smoke (make install then bashy agent list) is expected empty until the overlay is mounted.
Gate: go test ./pkg/fleet/... ./pkg/agentlaunch/... ./pkg/chat/... ./pkg/meet/... ./pkg/capability/...; bashy chat --agent <overlay agent> works with the overlay mounted; dev host keeps its full fleet after make install (count before/after).
