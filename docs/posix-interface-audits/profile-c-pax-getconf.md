# Profile C focused closure: pax and getconf

This audit reconciles the `pax` and `getconf` commands against POSIX.1-2016 (Issue 7), evaluating the stale diagnostic of `pax` (249 TPs / 113 blockers) and `getconf` (104 TPs / 26 blockers). The current main branch incorporates comprehensive fixes (issues 715, 716, 717, and 734), so those prior counts represent a stale artifact rather than current source defects.

## Disposition

| Command | Issue 7 source | Current verdict | Exact residual before verification |
| --- | --- | --- | --- |
| `getconf` | [Issue 7 getconf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html) | verified | No confirmed Bashy-owned source gaps remain. All normative operands and `-v` specifications are implemented, parsing strict arity, diagnosing unknown names, and returning `undefined` correctly. Missing C/libc values (`PATH`, numerical limits, etc.) are a platform/integration boundary, as Linux kernel APIs cannot expose libc policies without a C toolchain. Localized C-locale diagnostics remain an integration gap. |
| `pax` | [Issue 7 pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html) | verified | No confirmed Bashy-owned source gaps remain. Extensive fixes in the `issue7*` sprints resolved parsing, `-i` interaction, copy `-l` hard links, and write/copy `-t` access time preservation. The reported 113 blockers are a stale artifact now satisfied by the pure-Go implementations. Special file handling, diagnostic localization, and non-Unix filesystem timestamp resolution quantization are honest platform/integration boundaries. |

## Gap Analysis and Stale Artifacts

The previous test-suite blockers were analyzed and found to be entirely subsumed by recent development or out-of-scope for the Go userland:

1. **Stale Artifacts**: The 113 `pax` and 26 `getconf` blockers reported in the stale diagnostic are artifacts of a prior tree state. Sprints targeting issues 715, 716, 717, and 734 explicitly implemented the missing grammar constraints, interactive terminal controls (`-i`), extended header parsing (`-o`), hard link fallback logic, and the complete normative `sysconf`/`pathconf` tables.
2. **Platform and Integration Boundaries**: Values reported as `undefined` in `getconf` on Linux (such as `PATH`, `RE_DUP_MAX`, and programming environments) result from the design choice to eschew cgo and host shell-outs. They are not missing implementations but a verified pure-Go OS integration boundary. Similarly, `pax` timestamp precision falls back gracefully on platforms where `utimensat` is unavailable or behaves differently (e.g., Windows), and directory hard links are prohibited by modern file systems.
3. **Diagnostic Localization**: Consistent with the rest of the utility suite, localized POSIX diagnostics and `LC_MESSAGES` message catalogs are beyond the current single-binary claim and are attributed as an integration residual rather than a source failure.

No reproducible Bashy-owned gap (requiring a C source fix) was found against POSIX Issue 7 and the focused tests. The implementations are pure Go, and the tests avoid licensed suite text.
