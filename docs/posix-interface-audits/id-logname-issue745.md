# `id` and `logname` POSIX Issue 7 audit

Normative sources: [POSIX.1-2016 `id`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html) and [POSIX.1-2016 `logname`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logname.html).

## `id`

The implementation accepts each required synopsis: `id [user]`, `id -G [-n]
[user]`, `id -g [-nr] [user]`, and `id -u [-nr] [user]`. It rejects multiple
selection options, `-n` or `-r` without a selector, and more than one user
operand. Without an operand, real and effective process IDs and the live
supplementary-group vector are used. With a user operand, passwd/group database
records are used. Name lookup failures fall back to the numeric ID where POSIX
permits numeric output.

| Requirement | Executable evidence |
| --- | --- |
| Default real/effective fields and ordering | `TestIDDefaultReportsRealAndEffectiveWhenDifferent`, `TestIDDefaultOmitsEffectiveWhenEqual` |
| Live supplementary groups, distinct ordered `-G` result | `TestIDCurrentGroupsUseLiveProcessVector`, `TestIDOnlyFlags` |
| `-u`, `-g`, `-G`, `-n`, and `-r` forms | `TestIDOnlyFlags`, `TestIDRealFlagWithOptions`, `TestIDRealAndEffectiveSelectors` |
| Named user default and selector forms | `TestIDNamedUserOperand`, `TestIDNamedUserOperandCombinations` |
| Invalid combinations, unknown users, and operand arity | `TestIDErrors`, `TestIDRejectsExtraUserOperand` |
| Output errors and short writes return non-zero | `TestIDOutputErrors` |

The real/effective divergence tests use credential seams because an ordinary
test process is not set-ID. Native Unix runtime behavior is provided by
`id_unix.go`; platforms without POSIX numeric credentials retain an explicit
best-effort disposition and are not claimed as runtime-conformant. Diagnostic
catalog translation remains outside the carried locale provider.

## `logname`

`logname` accepts no options or operands, writes exactly the session login name
and a newline, and returns non-zero with a diagnostic when no login name is
available. It never consults `LOGNAME`, `USER`, `LNAME`, or `USERNAME` and never
falls back to the effective account.

Linux resolves the kernel audit login UID through the user database. Darwin,
DragonFly BSD, and FreeBSD call the native `getlogin` system call without cgo.
Other targets fail explicitly rather than inventing a session identity.

| Requirement | Executable evidence |
| --- | --- |
| Successful session-name output | `TestLogname` (runtime-conditional) |
| Required no-login failure | `TestLognameNoLoginName`, `TestLoginNameHasNoEffectiveUserFallback` |
| Environment/effective-user isolation | `TestLognameIgnoresEnvironmentAccountNames`, `TestLoginNameHasNoEffectiveUserFallback` |
| Linux login-UID parsing and unset handling | `TestResolveLoginUID` |
| Operand and option rejection | `TestLognameRejectsOperandsAndUnknownOptions` |
| Output errors, short writes, and no stdin use | `TestLognameOutputErrorsAndRunContextIsolation` |

The BSD provider is runtime-covered on Darwin and cross-built for the other
declared targets. A real login session is host state, so absence of one is a
valid no-login product rather than silently selecting the effective user.
OpenBSD, NetBSD, Windows, and other unsupported targets currently take the
explicit failure path; they are residual platform work, not verified POSIX
providers.
