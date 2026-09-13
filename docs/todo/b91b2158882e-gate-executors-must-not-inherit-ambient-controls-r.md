---
id: b91b2158882e
kind: task
title: 'Gate executors must not inherit ambient controls: route them through the gate allowlist'
seq: 85
status: todo
priority: p1
created: 2026-09-07T08:28:00.242307Z
sprint: 131
---

A gate's exit code is a VERDICT. A gate that inherits an ambient control decides differently depending on the shell that launched it, which makes the verdict a fact about the environment rather than about the tree. This is the highest-severity item in the section 14 design contract, because it can corrupt EVIDENCE rather than merely waste effort.

AUDIT FINDING (docs/agentic-tool-duality-design.md 14.12.1, umbrella). Four Go-side executors were classified. gate.RunLocal builds its environment with attestEnv, a strict ALLOWLIST of PATH HOME LANG TMPDIR GOCACHE GOMODCACHE GOPATH plus synthesized LC_ALL and HERALD_GATE_DIR, and is IMMUNE - fail-closed even against names that do not exist yet. weaveRunVerify via weaveVerifyEnv passed through os.Environ minus PWD and OLDPWD. supervise.runGate set no cmd.Env at all. gate_broker was listed as exposed and THAT CLASSIFICATION WAS WRONG - see below.

WHAT CHANGED. pkg/gate exports Env(dir), which is attestEnv, so every gate executor builds its environment in ONE place with the allowlist model rather than each inventing its own policy. pkg/supervise runGate now uses gate.Env(cwd); it is a true gate (the orchestrator's own check that a worker's done is real) and the strict allowlist already carries what a build or test command needs. pkg/weave weaveVerifyEnv now applies gate.ScrubControls.

WHY WEAVE GETS THE NARROWER GUARANTEE, stated because the difference matters and must not be quietly forgotten. gate.Env is fail-closed against EVERY unknown name. ScrubControls is fail-closed only within the namespaces bashy owns, by prefix. Weave verify runs an arbitrary operator-supplied build or test command, which legitimately needs the wider toolchain environment - GOFLAGS, CGO_ENABLED, SSH_AUTH_SOCK, compiler variables, proxy settings - that no fixed allowlist can enumerate without breaking real commands. So a strict allowlist there would trade a correctness bug for a broken verify. The prefix denial is sound for the threat actually identified, an ambient BASHY control reaching a gate, because those controls live in a namespace we own and a new one is caught without being added by hand. It is NOT sound against a third-party variable that changes behavior, and that limit is documented on the function and asserted by a test that fails if the doc and the behavior drift apart.

CORRECTION TO THE AUDIT: gate_broker is NOT a gate executor. The os.Environ call at gate_broker.go is inside gateBrokerBrowserLogin, which runs the BROWSER tool to perform a LOGIN - a remediation the broker routes a gate VERDICT to, not the execution of a gate. It needs the full environment and scrubbing it would break authentication. The audit table over-collected on a filename. Left unchanged deliberately; the umbrella section is corrected to match.

TESTS ARE THE INVARIANT, NOT COVERAGE. pkg/gate: Env drops ambient controls while keeping PATH; Env drops an unknown FUTURE variable, which is the whole reason it is an allowlist and not a denylist; ScrubControls keeps the toolchain and drops the controls; ScrubControls catches a future control by prefix and DELIBERATELY does not catch a third-party one, so the test states the documented limit as well as the promise. pkg/supervise: runGate actually RUNS a gate that reports what it can see, so it fails if the environment is ever inherited again, plus one asserting the gate still sees PATH so the fix cannot trade correctness for a broken gate.

VERIFIED BY MUTATION, not just by green. Reverting the supervise line turns TestRunGateDoesNotInheritAmbientControls red with the exact message gate saw BASHY_AGENTIC="1"; restoring it turns it green. Full hermetic go test ./... passes.

THIS UNBLOCKS the ambient-ceiling design: section 14.12.3 permits BASHY_AGENTIC to carry an ambient rung ceiling only when it means bashy ORCHESTRATED this run, and named this fix as the gating prerequisite, because a world-changing control inherited by a gate is exactly the failure that permission would otherwise create.
