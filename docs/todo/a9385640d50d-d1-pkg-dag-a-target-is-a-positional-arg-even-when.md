---
id: a9385640d50d
kind: task
title: 'D1 pkg/dag: a target is a positional arg even when subcommands are mounted — explicit root Args + regression test with AddCapacityCommands'
seq: 126
status: done
priority: p0
created: 2026-09-13T05:23:18.834074Z
sprint: 164
closed: 2026-09-13T05:27:33.505674Z
---

ROOT CAUSE (measured 2026-09-13 with a debug build): 'bashy dag hello' fails with cobra's 'unknown command "hello" for "dag"'. bashy mounts dag.AddCapacityCommands(cmd, …) which adds a 'capacity' subcommand; once a cobra root has ANY subcommand and no explicit Args validator, cobra's legacyArgs treats every positional argument as a subcommand lookup and rejects it. The direct pkg/dag command (no subcommands) runs the target fine — so every pkg/dag test passed while the shipped verb was dead for every target in every repo (-n, --json, --plain, targets, all of it).

FIX (pkg/dag/command.go): the root NewDagCmd declares its positional contract explicitly — Args: cobra.ArbitraryArgs (targets are operands; the existing RunE already validates unknown target names against DAG.md and emits the dag error envelope). Keep SilenceErrors/SilenceUsage. Do NOT special-case 'capacity'.

TEST (pkg/dag): TestTargetRunsWhenSubcommandsAreMounted — build a cobra cmd via NewDagCmd(), call AddCapacityCommands(cmd, CapacityServices{}), SetArgs(["hello"]) on a temp dir with a one-target DAG.md, assert Execute() == nil and the target body ran; a second case with ["-n", "hello"] prints the plan; a third asserts an unknown target still returns a *dag.Error (not cobra's unknown-command string). Add the same mounted shape to any existing target-execution test helper if one is shared.

Gate: go test ./pkg/dag/... green; from the ycode and coreutils repos, a bashy rebuilt on this commit runs 'bashy dag build -n' with exit 0 (conductor verifies with the installed binary). Files: pkg/dag/command.go, pkg/dag/command_test.go (or a new capacity_mount_test.go). Depends on: nothing.
