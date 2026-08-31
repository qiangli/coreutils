# Retired regression audit — issue 4

Audit scope: existing public, deterministic regressions for the retired
identities below. No product behavior is added by this audit.

| Retired identity | Causal correction | Focused public regression |
| --- | --- | --- |
| `kill_NE:19` | `kill -l` reports a stdout write failure. | `cmds/kill.TestKillListWriteError` exercises both the listing and one-operand forms with a failing writer. |
| `m4:73` | POSIX `maketemp` neither creates a file nor emits the GNU-only warning; GNU mode remains unchanged. | `tools/posix-providers/m4-semantic-test.sh` checks both branches against the locally built provider. |
| `pwd:9` | A repointed logical `PWD` is rejected for the process-CWD route. | `cmds/pwd.TestPwdLogicalRejectsRepointedPWD`. |
| `pwd:11` | Physical process-CWD resolution ignores a logical `$PWD` symlink spelling. | `cmds/pwd.TestPwdPhysicalOnProcessCwdIgnoresLogicalPWD` covers default, `-P`, and last-option-wins forms. |

Review result: 4/4 retired identities map to an existing focused regression.
