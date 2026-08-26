# POSIX Go evidence closure: Profile C cp, chown, id

This audit covers `cmds/cp`, `cmds/chown`, and `cmds/id` against POSIX.1-2016 (Issue 7).

The VSC-PCTS diagnostic blockers retained from earlier runs (`cp` 16, `chown` 12, `id` 13) are stale-artifact residuals. Current main is much newer, and the implementations have been heavily refactored since those metrics were captured. Any remaining limitations are precisely attributed below.

| Command | Issue 7 source | Current verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `cp` | [Issue 7 cp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html) | partial | 16 VSC-PCTS failures are stale-artifact residuals. GNU extensions (e.g. `-u`, `-a`) and interactive overrides are supported. Symlink and device node duplication require elevated privileges on some platforms. Translated diagnostics are absent. |
| `chown` | [Issue 7 chown](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html) | partial | 12 VSC-PCTS failures are stale-artifact residuals or privilege constraints. Windows lacks `chown` entirely. On Unix, root guard and symlink changes have filesystem/privilege integration boundaries. |
| `id` | [Issue 7 id](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html) | partial | 13 VSC-PCTS failures are stale-artifact residuals. GNU extensions (`-z`, `-a`, `-p`) are present. Translated diagnostics are absent. |

