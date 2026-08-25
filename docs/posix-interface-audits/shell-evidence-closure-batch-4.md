# POSIX shell evidence closure: wave 4

Wave 4 covers the shell-selected `kill`, `time`, and `wait` interfaces.
POSIX.1-2016 Issue 7 is normative. GNU Bash 5.3 behavior is retained only
where POSIX leaves behavior unspecified or Bash exposes an extension. Bashy's
Profile B routing tests separately prove that direct invocations select the
shell builtin or keyword rather than a same-name Go applet.

All three ledger rows move from `unverified` to `partial`. The tests execute
the same effective shell mode as Bashy's POSIX path: Bash grammar with the
parser POSIX switch, runner POSIX mode, strict `sh` semantics, a fixed C
locale, and `POSIXLY_CORRECT`. They do not claim translated diagnostics,
arbitrary non-C locales, every host signal, or every process/job-control race.

| Command | Stable evidence | Remaining boundary |
| --- | --- | --- |
| `kill` | `TestKillIssue7Interface` covers the default signal; `-s` names and zero; numeric and XSI name selectors; `-l` listing/mapping; PID, negative process-group, and job operands; diagnostics, statuses, streams, and the separate Bash `-n` extension. | Exhaustive host signal sets, permission failures, multi-operand partial failure, translated diagnostics, and real process-group topology remain. POSIX requires a signaled wait status greater than 128; exact `128+signal` is classified separately as Bash compatibility. |
| `time` | `TestTimeIssue7CommandInterface` covers portable `-p` output on standard error, utility-status propagation, lookup failure 127, and standard-input preservation through the real POSIX execution path. Existing timing tests cover shell, pipeline, and child CPU accounting. | Locale-specific numeric formatting/messages, signal interruption, status 126, and all implementation-defined report precision remain. |
| `wait` | `TestWaitIssue7Interface` covers zero, one, and multiple operands, final-operand status, known and unknown jobs, and unused standard input/output. Direct channel-controlled tests prove operandless blocking and retained completed-status consumption. | Exhaustive interactive job-control states, signal interruption/traps, implementation capacity for retained statuses, and translated diagnostics remain. Bash `-n`, `-p`, and `-f` are extensions and are not counted as POSIX requirements. |

## Accepted implementation correction

Review found that reaping the most recent job changed `$!` to an older job,
so the original `wait -n` test accidentally depended on behavior opposite to
GNU Bash 5.3. The accepted correction retains the most recently started
asynchronous job independently from the active wait table. Ordinary
background jobs, process substitutions (including those propagated out of
pipeline sub-runners), and coprocs all update that value; a failed launch does
not. Regression tests save and clean up explicit PIDs and verify that `wait
-n` itself leaves `$!` unchanged.

The accepted shell commits are `f576d839` and `ca11b10f` for the initial
evidence, followed by reviewed correction `d6515a19`. Focused tests passed ten
repetitions and the race detector; all Issue 7 interface tests, the exact
`TestRunnerRun` gate, and diff checks passed. An independent review approved
the final source and evidence.

The corresponding routing evidence remains
`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteKill`,
`TestProfileBRouteTime`, and `TestProfileBRouteWait`. Semantic evidence and
routing evidence are independent fail-closed ledger lanes.
