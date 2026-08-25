# POSIX Go evidence closure: batch 6C

Batch 6C covers the Go-owned `more`, `newgrp`, and `nice` interfaces against
POSIX.1-2016 Issue 7. The three interface rows move from `unverified` to
`partial`; none is claimed fully verified.

| Command | Evidence closed | Remaining boundary |
| --- | --- | --- |
| `more` | Stdin/default-file handling, ordered file concatenation, `-s`, terminal vs nonterminal option effects, stdout content separation from terminal UI, prompt-before-read ordering, no silent copy fallback when a controlling terminal is unavailable, and Windows explicit unsupported-terminal behavior. | The Issue 7 `more` utility is optional, and this implementation still refuses several optional interactive features instead of approximating them: `-d`, `-i`, `-l`, `-f`, `-t`, and most `-p` commands. Locale-sensitive command processing, editor handoff, tags, and full command grammar remain residual. |
| `newgrp` | `newgrp [-l] [group]`, primary-group fallback, name-before-gid resolution, membership and group-password authorization, refused-change-but-start-shell status behavior, kernel credential-refusal retry, login-shell argv/environment construction, usage errors starting no shell, Unix credential planning, and Windows loud refusal. | The implementation starts a child shell instead of replacing the embedding process image, by repository design. Real setuid-root group switching, system NSS/PAM behavior, real terminal password reads, and implementation-specific login environment breadth remain outside hermetic coverage. |
| `nice` | POSIX `-n increment utility [argument...]` parsing, utility-operand passthrough, PATH lookup, 126/127 distinction where constructable, child status propagation, signaled-child status mapping, child-only scheduler adjustment, and `POSIXLY_CORRECT=1` rejection of missing utility and GNU-only adjustment spellings. | Scheduler priority effects are inherently platform and privilege dependent; failures to lower niceness are diagnostic-only per Issue 7. Outside `POSIXLY_CORRECT`, existing GNU-compatible extensions such as no-operand priority printing, `--adjustment`, and obsolete `-NUM` forms remain intentionally unchanged. |

Focused package tests and the required count-10, race, vet, and cross-platform
checks were run for the touched packages and scripts before committing. The
generated manifest Markdown, applet matrix, aggregate counts, and unrelated
command rows were not edited.
