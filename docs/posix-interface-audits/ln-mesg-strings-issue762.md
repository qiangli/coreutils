# `ln`, `mesg`, and `strings` Issue 762 POSIX.1 Issue 7 Audit

Scope: Open Group POSIX.1-2008 Issue 7, 2016 Edition interfaces for exactly
`ln`, `mesg`, and `strings`, audited against repository baseline `6ffca6b`.
GNU behavior is relevant only where Bashy deliberately retains an extension;
it is not conformance evidence.

## Result

The audit confirmed and fixed two observable implementation gaps and one
cross-platform backend defect:

- `ln -f` used `os.Remove` for the required destination-removal step. On Unix,
  `os.Remove` retries a directory with `rmdir`, so an empty directory at
  `target_dir/basename(source_file)` was removed and replaced by a hard link.
  Issue 7 requires actions equivalent to `unlink()`, which must fail for that
  directory. Unix builds now call `unlink(2)` directly; non-Unix builds reject
  directories before their file-removal fallback.
- `strings -t d|o|x` always used GNU's seven-column offset field. Issue 7
  specifies the unpadded `%d %s`, `%o %s`, and `%x %s` forms. When
  `POSIXLY_CORRECT` is present (including an empty value), Bashy now emits the
  exact Issue 7 form while retaining its documented padded default outside
  POSIX mode.
- The physical hard-link backend selected `x/sys/unix.Linkat` on AIX and
  Solaris even though that API is unavailable on those targets. Those systems
  now use their native POSIX `link(2)` syscall; the remaining named Unix
  targets continue to use `linkat(2)` without `AT_SYMLINK_FOLLOW`.

All three canonical ledger rows remain `partial`. The focused evidence closes
the audited Profile-C/Linux behavior below, but it is not a complete POSIX
certification run, locale message catalogs are absent, and the command-specific
residuals remain material.

## Normative coverage

### `ln`

Source: [Issue 7 `ln`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ln.html).

| Area | Audited disposition |
| --- | --- |
| Synopsis and operands | Both required forms are covered: `source_file target_file` and ordered `source_file... target_dir`; a non-directory final operand with more than two operands fails. The accepted one-operand form is an extension. |
| `-s` | Creates a symlink containing `source_file` verbatim; dangling and self-referential source text is accepted. `-L` and `-P` are silently ignored with `-s`. |
| `-L` / `-P` | Hard-link-to-symlink behavior is covered for each option, last-option-wins including a combined short form, and the documented `-P` default on supported Unix targets. Darwin, DragonFly BSD, FreeBSD, Linux, NetBSD, and OpenBSD use `linkat(2)`; AIX and Solaris use their POSIX `link(2)` syscall. |
| Existing destination | Without `-f`, diagnose, preserve the destination, and continue. Same-directory-entry identity is checked before removal, including hard-link aliases and directory-form destinations. |
| `-f` ordering | A non-identical destination is unlinked before link creation. Unlink failure preserves it; a missing source after successful unlink leaves it absent; either error sets non-zero status and later sources continue. An empty destination directory is never removed. |
| Streams and status | Required forms do not read stdin or write stdout. Diagnostics use stderr. Status is zero only if every requested link succeeds. |

### `mesg`

Source: [Issue 7 `mesg`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html).

| Area | Audited disposition |
| --- | --- |
| Operands | No operand queries without changing state; POSIX-locale `y` and `n` set the resulting permission state; invalid or extra operands are usage errors. No options are defined, and `--` terminates parsing. |
| Terminal selection | Real PTYs prove the first terminal is selected in stdin, stdout, stderr order. `/dev/null` is rejected despite being a character device. No stream bytes are consumed. |
| Permission effect | A real PTY test proves `y` sets only group-write and `n` clears only group-write. Hermetic mode tests prove all other permission bits survive. |
| Output and errors | Query output is `is y` or `is n` followed by newline; setters write nothing. Terminal discovery, stat, chmod, and stdout failures diagnose and return greater than 1. |
| Status | Query/`y` returns 0 when receiving is allowed; query/`n` returns 1 when denied; every error returns 2. |

### `strings`

Source: [Issue 7 `strings`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strings.html).

| Area | Audited disposition |
| --- | --- |
| Input selection | No operands reads stdin. Regular-file operands are processed in order, stdin is ignored when files are present, offsets reset per file, and open errors do not suppress successful earlier or later files. A first `-` argument is unspecified by Issue 7 and remains a literal pathname. |
| `-a` / `-n` | `-a` scans the entire input; the implementation documents that as its default too. `-n` accepts a positive minimum and counts printable characters, not UTF-8 bytes; the default is four. |
| Locale printability | `LC_ALL` overrides `LC_CTYPE`, which overrides `LANG`. C/POSIX ASCII and UTF-8 character-granular classification are covered, including invalid UTF-8 boundaries, canonical U+FFFD, exact-byte preservation, and byte offsets. |
| `-t` | `d`, `o`, and `x` offsets are byte offsets. POSIX mode emits the exact unpadded Issue 7 forms; default mode retains seven-column GNU-shaped padding. Invalid format arguments fail. |
| I/O and status | Stdin read, file open/read, and stdout write failures diagnose and return non-zero. Successful operands remain in order around an open error; success returns zero. |

## Residuals

- All three commands emit fixed English diagnostics and do not implement
  `LC_MESSAGES`/`NLSPATH` catalogs.
- `ln` retains GNU extensions (`-b`, `-S`, `-i`, `-n`, `-r`, `-t`, `-T`,
  `-v`, long options, and the single-operand form). Its implementation-defined
  default hard-link treatment of a symlink source is evidenced as `-P` on the
  Unix targets using `linkat` or the AIX/Solaris POSIX `link` syscall; other
  targets retain `os.Link` behavior.
  Hard-linking a directory remains kernel/platform-defined as Issue 7 permits.
- `mesg` uses the traditional group-write-bit mechanism, which Issue 7 leaves
  unspecified. Windows fails closed because it has no equivalent terminal
  permission. Unix device-path resolution is evidenced with real Darwin and
  Linux PTYs but is not an exhaustive test of every `/dev` layout.
- `strings` has exact C/POSIX, UTF-8, and carried single-byte locale-provider
  evidence, but no complete installed locale database. Its whole-file default
  and additional strings terminated by non-printable bytes are permitted
  implementation-defined choices. The first-argument `-` behavior is
  explicitly unspecified by Issue 7.

## Verification

The following gates passed on the Darwin host:

- default and `POSIXLY_CORRECT=1` `go test -count=20` for all three packages;
- default and `POSIXLY_CORRECT=1` `go test -race -count=5` for all three;
- focused and repository-wide native `go vet`;
- focused Linux, Darwin, Windows, FreeBSD, DragonFly BSD, NetBSD, OpenBSD,
  AIX/ppc64, Solaris/amd64, js/wasm, and wasip1/wasm vet/build checks;
- the remaining full `scripts/crossvet.sh` commands: repository-wide Windows,
  Linux, and Darwin vet, the js/wasip1 ownership-mode canaries, the AIX lock/dd
  build canary, and both WebAssembly `mv` build canaries;
- `scripts/fmtcheck.sh`, `scripts/applet-test-coverage.sh`, and
  `scripts/applet-matrix.py --check`;
- an exact comparison of the canonical TSV renderer output with the generated
  Markdown plus validation of every row after suppressing only the inherited
  unavailable `sh` semantic/routing evidence references in memory.

The focused suite also passed in the cached Go 1.26 Debian container. A
POSIX-mode count-20 run explicitly exercised the new `ln` unlink-order tests,
all stdin/stdout/stderr `mesg` selection cases and live PTY permission changes,
and exact `strings -t` formatting against real Linux `unlink(2)` and
`/dev/pts` behavior; no Linux test skipped.

The official manifest unit/check and therefore the `scripts/crossvet.sh`
wrapper remain red before cross-compilation because of a pre-existing external
evidence mismatch: the canonical `sh` row names
`bashy:internal/cli/profile_b_sh_entrypoint_unix_test.go#TestProfileBShUtilityEntrypointContract`,
but that test is absent from the pinned sibling Bashy checkout. This audit did
not alter or launder the unrelated `sh` row. The target rows validate, their
generated Markdown exactly matches the canonical TSV, and the cross-vet
compile commands that the wrapper could not reach all passed independently.
