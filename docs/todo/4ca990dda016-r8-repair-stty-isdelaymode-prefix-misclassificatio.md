---
id: 4ca990dda016
kind: bug
title: R8 Repair stty isDelayMode prefix misclassification of cread and crt
seq: 192
status: todo
priority: p1
created: 2026-11-24T11:35:52.505304Z
sprint: 675
sprint_id: 9b07aedb-77af-52d5-8af3-8035a213774c
sprint_title: Investigate S575 POSIX residuals and prepare certification
---

Repair the confirmed `cmds/stty` prevalidation defect where `isDelayMode` (`cmds/stty/stty_termios.go:437-444`) matches mode prefixes (`"cr"`, `"nl"`, `"tab"`, `"bs"`, `"ff"`, `"vt"`), misclassifying accepted non-delay modes `cread`, `-cread`, and `crt` as output-delay modes so `validateMode` rejects them with `"<mode> is not supported on this platform"` before `setTermios` is called.

## Worker rules (MANDATORY)

- **STEP 0 (FIRST COMMAND BEFORE ANY GIT COMMIT)**: Run `export BASHY_AGENT=codex-gpt6-sol` in your shell and verify with `git log -1` BEFORE `bashy sprint submit` that your commit carries `Agent: codex-gpt6-sol` (alongside `Sprint: #675`, `Story: #192`, `Story-ID: 4ca990dda016`).
- **Strict scope (Operator Directives D16 & D17, Supervisor Note 10A)**:
  1. Replace prefix matching in `isDelayMode` (`cmds/stty/stty_termios.go`) with exact delay-name matching across the 17 accepted delay names ONLY (`cr0`, `cr1`, `cr2`, `cr3`, `nl0`, `nl1`, `tab0`, `tab1`, `tab2`, `tab3`, `tabs`, `bs0`, `bs1`, `ff0`, `ff1`, `vt0`, `vt1`). Do NOT alter any other command, flag table, negation table, or behavior.
  2. Write **failing-first** unit tests FIRST on the unpatched tree and confirm they fail before editing `stty_termios.go`:
     - In `cmds/stty/stty_termios_test.go`: assert `isDelayMode` returns `false` for `"cread"`, `"crt"`, and `"nl"`, and `true` for all 17 exact delay names; assert `validateMode("cread") == nil`, `validateMode("-cread") == nil`, and `validateMode("crt") == nil`.
     - In `cmds/stty/stty_posix_test.go`: assert on a fresh PTY (`openTTY(t)`) that `applyMode` and `runStty` return `nil` / `code == 0` with empty stderr for `"cread"`, `"-cread"`, and `"crt"` (reaching `setTermios`; note a PTY driver may keep `CREAD` set in kernel `termios`, so assert `code == 0` and `errb == ""` for `-cread`, and assert `crt` sets `unix.ECHOE` after `-echoe`); also assert negative controls through `runStty` (`"-crt"`, `"-cr0"`, `"crbogus"`) still fail with usage exit code `2` and `"invalid argument"`.
  3. Apply the one-predicate `isDelayMode` fix in `cmds/stty/stty_termios.go` and verify `go test -count=1 ./cmds/stty/...`, `./scripts/fmtcheck.sh`, and `./scripts/applet-test-coverage.sh` pass locally (wider cross-OS and full-repo/gate-host suites run on the gate host under manager claim).
  4. **Public repository hygiene**: NO certification authority ticket numbers, NO licensed test-suite script names or `/tset/` paths, NO restricted journal excerpts, NO hostnames/IPs/logins/home paths anywhere in code, comments, commit messages, or story summary.
  5. Rebase onto `origin/main` if needed so your branch fast-forwards cleanly, add a concise `## S675 R8 Summary` to this story file, commit in one clean commit (or story summary + fix commit) with `Agent: codex-gpt6-sol`, verify `git log -1`, and run `bashy sprint submit`.

## S675 R8 Summary

- Added failing-first tests for exact delay names, valid `cread`/`-cread`/`crt` PTY application, and invalid-mode controls. The valid-mode tests failed on the original predicate as expected.
- Changed only `isDelayMode` to recognize the 17 exact delay names. `go test -count=1 ./cmds/stty/...` and `./scripts/applet-test-coverage.sh` pass; the changed Go files are gofmt-clean.
- `./scripts/fmtcheck.sh` reports seven existing formatting differences outside `cmds/stty`; none were changed for this story.
