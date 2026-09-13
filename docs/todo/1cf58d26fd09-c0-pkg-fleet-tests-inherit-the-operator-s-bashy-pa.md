---
id: 1cf58d26fd09
kind: task
title: C0 pkg/fleet tests inherit the operator's BASHY_*_PATH overlay — strip it in TestMain
seq: 125
status: done
priority: p0
created: 2026-09-13T02:55:15.828876Z
sprint: 163
closed: 2026-09-13T02:55:23.644832Z
---

Found while merging C1: TestFamilyAliasPicksHighestVersion and TestResolveLaunchModelLongestCanonicalMatch fail on any host that exports the umbrella fleet/ overlay (sprint 161 U4) because the catalog under go test inherits BASHY_MODELS_PATH. The project gate (.bashy/gate = whole-tree go test) is therefore red on main on the dev host and every weave pull is refused. Fix: pkg/fleet/main_test.go TestMain unsets BASHY_TOOLS_PATH / BASHY_MODELS_PATH / BASHY_AGENTS_PATH; tests that need a ring set it with t.Setenv. Gate: go test ./pkg/fleet/... green with the overlay exported.
