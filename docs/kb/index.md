# kb index

2 page(s). Search: `bashy kb search <query>` — check before starting a task; `bashy kb retro` after. Pages live under pages/.

- [pathchk-empty-operands](pages/pathchk-empty-operands.md) `validated/lesson` pathchk treats an empty string as an invalid operand — Quote empty-operand probes explicitly; GNU Coreutils 9.11 and this implementation reject the empty string in default, -p, and -P modes.
- [uuencode-stdin-and-zero-sextet-framing](pages/uuencode-stdin-and-zero-sextet-framing.md) `validated/lesson` uuencode stdin and zero sextet framing — For POSIX uuencode, the header records the input file's access bits and '-' is a pathname; classic zero sextets and the terminating zero-length line use SPACE, while uudecode may accept backtick. Preserve decoded access bits with a post-write chmod instead of changing process umask.
