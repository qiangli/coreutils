# POSIX Go evidence closure: batch 6C

Batch 6C covers the Go-owned `more`, `newgrp`, and `nice` interfaces against
POSIX.1-2016 Issue 7. The three interface rows move from `unverified` to
`partial`; none is claimed fully verified.

| Command | Evidence closed | Remaining boundary |
| --- | --- | --- |
| `more` | Stdin/default-file handling, ordered file concatenation, positive `-n` validation, `-s`, the required nonterminal rule that every other option has no effect, stdout content separation from terminal UI, prompt-before-read ordering, no silent copy fallback when a controlling terminal is unavailable, and Windows explicit unsupported-terminal behavior. | The Issue 7 `more` utility is optional, but once supplied its `-i` and `-p` interfaces are required and remain incomplete on terminal output; tags and `-t` are conditional on the ctags option and remain unsupported. The non-POSIX `-d`, `-l`, and `-f` extensions are also unsupported interactively. Locale-sensitive command processing, editor handoff, tags, and the full command grammar remain residual. |
| `newgrp` | `newgrp [-l] [group]`, primary-group and supplementary-group reset, name-before-gid resolution, membership and group-password authorization, refused-change-but-start-shell status behavior, kernel credential-refusal retry, login-shell argv/environment construction, usage errors starting no shell, Unix credential planning, and Windows loud refusal. | The implementation starts a child shell instead of replacing the embedding process image, by repository design. Real setuid-root group switching, system NSS/PAM behavior, real terminal password reads, and implementation-specific login environment breadth remain outside hermetic coverage. |
| `nice` | POSIX `-n increment utility [argument...]` parsing, utility-operand passthrough, PATH lookup, 126/127 distinction where constructable, child status propagation, raw signal boundary propagation through `RunContext.ExitSignal`, child-only scheduler adjustment, and missing-utility rejection whenever `POSIXLY_CORRECT` is present (including an empty value). | Priority is applied immediately after process start, leaving a pre-exec race in which a very short-lived utility can run before adjustment; closing it requires a child-start barrier. Scheduler priority effects are also platform and privilege dependent. GNU-compatible no-operand printing, `--adjustment`, and obsolete `-NUM` forms remain available regardless of `POSIXLY_CORRECT`; POSIX does not require rejecting extension invocations. |

Focused package tests and the required count-10, race, vet, and cross-platform
checks were run for the touched packages and scripts before committing. The
generated manifest Markdown, applet matrix, aggregate counts, and unrelated
command rows were not edited.
