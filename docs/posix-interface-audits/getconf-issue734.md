# `getconf` POSIX.1-2016 interface audit (issue 734)

## Normative scope

This audit is pinned to POSIX.1 Issue 7, 2016 Edition:

- [`getconf`](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html)
- [`sysconf()` variables](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/functions/sysconf.html)
- [`fpathconf()`/`pathconf()` variables](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/functions/fpathconf.html)
- [`confstr()` variables](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/functions/confstr.html)
- [`<limits.h>` Maximum and Minimum Values](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/basedefs/limits.h.html)

Later Issue 7 Technical Corrigenda and Issue 8 are not used to expand this
command's certification surface. `-a` is retained as a documented extension.

## Closed interfaces

- Both normative operand forms and `-v specification` retain strict arity and
  diagnostics. Unknown names fail; a known name whose value is indeterminate
  prints `undefined` and succeeds.
- The complete normative `sysconf` table (with the exclusions stated by the
  utility), all 21 pathname spellings, and all 31 `confstr` spellings are
  registered. The two robust-mutex system spellings omitted by the previous
  table are included. The audited system registry is a deliberate superset
  because it retains established numerical-limit extensions.
- All 50 fixed Maximum/Minimum values are exact specification constants,
  including `_POSIX_CLOCKRES_MIN=20000000`.
- All 17 compatibility spellings required by the utility description resolve
  to the same result as their underscored names.
- Path lookup errors produce no standard output, diagnose the failing query,
  and return non-zero. Standard-output write failures also return non-zero.
- On Linux, values are reported only when derivable without libc: resource
  limits, page size, CPU count, `AT_CLKTCK`, `/proc/sys/kernel/ngroups_max`,
  `statfs`, Linux UAPI pathname invariants, and known mounted-filesystem time
  resolution. `_POSIX_TIMESTAMP_RESOLUTION` is `1` on the carried nanosecond
  filesystems (btrfs, ext4, overlayfs, tmpfs, and XFS), otherwise `undefined`.
  Focused Linux tests compare every other claimed runtime/path value with an
  independent host `getconf` oracle.
- On Linux LP64 Go targets, `_POSIX_V6_LP64_OFF64` and
  `_POSIX_V7_LP64_OFF64` report `1`, matching the Ubuntu 24.04 certification
  image, and `-v` accepts exactly those V6/V7 environments. Issue 7 makes the
  corresponding non-`undefined` `_POSIX_V7_*` query the condition for the V7
  `-v` form. ILP32 and LPBIG environments remain unclaimed and are rejected;
  their query values remain `undefined`. V6 acceptance preserves the
  obsolescent environment advertised by Ubuntu without changing the V7 claim.
- Darwin continues to use the native `sysconf`, `pathconf`, and `confstr`
  adapters and differential tests. Windows reports fixed standard minima but
  otherwise fails closed as `undefined`.
- Implementation remains pure Go. Production code neither invokes another
  command nor uses cgo.

## Honest residuals and boundaries

Linux has no `sysconf` or `pathconf` system call, and Go deliberately has no
portable libc ABI. Therefore libc policy values and optional-feature results
that cannot be derived from a stable kernel interface remain `undefined`.
Notable examples are `PATH`, `RE_DUP_MAX`, `SYMLOOP_MAX`, programming-environment
flags, `LINK_MAX`, `FILESIZEBITS`, `SYMLINK_MAX`, asynchronous/prioritized/
synchronized I/O flags, and transfer-size maxima/increments. Unknown Linux
filesystem types likewise leave `_POSIX_TIMESTAMP_RESOLUTION` undefined rather
than guessing. Darwin's pinned adapter has no timestamp-resolution selector, so
that pathname variable is also undefined there.

The inventory additionally retains established implementation-extension names
such as numerical C limits, `PAGE_SIZE`, processor counts, and Darwin's
`_POSIX_FILE_LOCKING`; these do not enlarge the claimed POSIX surface. The
repository's deterministic diagnostic contract uses C-language diagnostics;
localized diagnostic message catalogs are outside this command's current
claim.
