---
id: 299cafe0f6ac
kind: task
title: Build GNU 9.11/uutils differential test campaign for Bashy utilities
seq: 1
status: assigned
priority: p1
created: 2026-08-07T04:46:05.852041Z
weave: 251
assignee: qiangli
---

Create a durable coreutils + vsc-pcts-harness-kit testing plan and executable first slice. Inventory registered Bashy applets and map each to POSIX, GNU Coreutils 9.11, and pinned uutils reference coverage. Design a safe container/VM-only foreign-suite runner with hard memory/PID/time limits and no host-root/home mounts. Run GNU GPL tests externally without copying GPL test source; use uutils only as semantic reference per CLAUDE.md and preserve attribution for any permitted test adaptation. Define machine-readable three-way results covering argv/env/stdin/stdout/stderr/status/filesystem effects. Keep GNU extensions distinct from POSIX. Include AgentOS verbs as a separate Bashy contract/schema test track. Implement a minimal differential slice for test, printf, env, expr, basename, and dirname; add documentation and CI-safe local tests. Coordinate the formal 116-set VSC utility arm in the harness without licensed-source disclosure. Required gates: focused tests, go test -short ./..., scripts/crossvet.sh. Commit locally for conductor review; do not push.
