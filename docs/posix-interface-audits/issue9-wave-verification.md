# Issue 9 verification wave: getconf…rm mandatory-interface audit

Date: 2026-08-27
Scope: every mandatory POSIX.1-2017 (Issue 7, 2018 edition) option,
option-argument, operand grammar, stdin/stdout/stderr shape, and
exit-status requirement for: getconf, grep, iconv, id, join, kill,
locale, logger, logname, ls, mesg, mkdir, mkfifo, more, mv, newgrp,
nice, nohup, od, paste, pathchk, pr, printf, ps, pwd, renice, rm.
Excluded per lane split: awk, ed, ln, mailx, patch, pax, rmdir, sleep.

Method: live clause probes of the built multicall against a clean
`LC_ALL=C` environment, compared clause-by-clause with the prior audit
ledgers in this directory, plus primary-source fetches of the Open
Group pages for every clause where the implementation deviates from
the "obvious" reading. Result: **zero violations**. Three clauses that
are commonly misread were re-confirmed with verbatim citations below.

## Re-confirmed clauses (primary source)

### renice: `-n increment` is mandatory

Issue 7 SYNOPSIS (post SD5-XCU-ERN-97, which removed the obsolescent
operand forms):

    renice [-g|-p|-u] -n increment ID...

`-n increment` is not bracketed, so `renice -p 123` without `-n` is a
usage error, not a default-increment form. Implementation:
`cmds/renice/renice.go:217` refuses the obsolescent spelling loudly;
pinned by `cmds/renice/renice_test.go` (TestReniceObsolescentForm).

### paste: `-d ''` may error; `-d '\0'` is the no-separator form

XCU paste OPTIONS defines `\0` as "Empty string (not a null
character)", and APPLICATION USAGE states of `paste -d "" ...`:

    the latter is not specified by this volume of POSIX.1-2017 and may
    result in an error.

Implementation errors loudly (`cmds/paste/paste.go:107`) and supports
the specified `\0` form ("no separator"), pinned by
`cmds/paste/paste_test.go` (empty `-d` case and the `\0` case).
Verified against the circular-reuse and EOF-as-empty-line clauses
(`paste -d: f1 f2` over a shorter second file yields the trailing
`c:` line).

### iconv: `-c` and `-s` "shall not affect the exit status"

XCU iconv OPTIONS states for both `-c` and `-s`:

    The presence or absence of -c shall not affect the exit status of
    iconv.  (likewise -s)

So `iconv -c -f UTF-8 -t ISO-8859-1` over invalid input omitting the
bad byte and still exiting 1 is conformant; the without-`-c` result is
system-documentation-defined (ours: omit, diagnose, exit 1).
Pinned by `cmds/iconv/iconv_discard_state_test.go` and the package
header comment in `cmds/iconv/iconv.go`.

## Probe matrix (all conformant)

| Applet | Clauses exercised | Result |
|---|---|---|
| getconf | `-a` full table; path var with/without pathname operand (PATH_MAX needs exactly one); unknown var diagnostics | ok |
| grep | exit 0/1/2; `-q` exits 0 with a match even after an open error (error-before-match ordering); `-c`, `-l`, `-n`, `-x`, `-s`, `-E`/`-F` conflict (exit 2), `-e`/`-f` accumulation, `--` | ok |
| iconv | `-l` list; unknown codeset error; UTF-8↔ISO-8859-1 conversion; invalid input with and without `-c` (status invariance) | ok |
| id | default `uid=…(%s) gid=…(%s) groups=…` shape; `-u`, `-nu`, `-G`; `-n` without `-Ggu` usage error; unknown user exit 1 | ok |
| join | `-t char`; `-a 1/2` unpaired lines; `-e` substitution with `-o` lists incl. `0`; `-v 1 -v 2` both-sides unpaired; `-1`/`-2`; extra operand usage error | ok |
| kill | `-l` (no operand), `-l <n>` name, invalid signal spec exit >0, `-s` missing argument exit 2 | ok |
| locale | `-a` public names; `-c LC_CTYPE` category header + keyword values; `-k keyword="value"`; unknown name exit 1 | ok |
| logger | option-free synopsis; operands concatenated with single spaces | ok (pinned by prior wave) |
| logname | name+newline on stdout; extra operand usage error exit 2 | ok |
| ls | `-F` suffixes, `-i`, `-d`, `-R` on missing operand exit 2; prior Profile C/D ledgers cover ordering/layout | ok |
| mesg | non-tty stderr diagnostic, exit 2 (tty paths pinned by ln-mesg-strings-issue762) | ok |
| mkdir | `-m` bypasses umask; existing dir without `-p` exit 1; `-p` parents | ok |
| mkfifo | `-m` bypasses umask; EEXIST exit 1 | ok |
| more | non-tty stdout writes file contents; `-<number>` accepted | ok |
| mv | same-file diagnostic exit 1; missing source exit 1; directory-into-itself exit 1; `-f`/`-i` pins from prior wave | ok |
| newgrp | exec-of-shell path; semantics pinned by newgrp-issue731 | ok |
| nice | `-n` missing arg / invalid increment exit 125-class; utility-not-found 127 | ok |
| nohup | 127 not-found; 126-class spawn failures; tty redirect clauses pinned by prior wave | ok |
| od | `-An -tu1 -N`, `-Ad -j -t x1` offsets, `-v` no-dedup, `-c`, and the default old-style octal (`od file`) | ok |
| paste | see re-confirmed clause above; escapes `\n`, `\t`, `\\`; circular reset per line and per file under `-s` | ok |
| pathchk | `-p` nonportable char diagnostics; `-P` empty-name; `-p -P` combined; exit 1 on failure | ok |
| pr | `-t`; `-h`, `-l`, `-2`, `-a`; `+2` past last page; column output byte-identical to host pr | ok |
| printf | format reuse across arguments; `%b`; `%d` invalid-arg zero-fill + diagnostic + exit 1; invalid directive exit 1; missing arg = empty string | ok |
| ps | live inspection fails loudly off-Linux; unknown flag usage exit 2 | ok |
| pwd | `-L` logical vs `-P` physical resolution; extra operand warning | ok |
| renice | see re-confirmed clause above; `-g/-p/-u` operand interpretation | ok |
| rm | `-f` missing-file exit 0 silent; without `-f` exit 1 + diagnostic; `-R`; write-protected prompt only required when stdin is a tty (XCU rm) | ok |

Note on the rm row: XCU conditions the write-protected prompt on
standard input being a terminal; silent removal of a 555 file through
a non-tty stdin is conformant (GNU prompts regardless; that is GNU's
own choice, not a POSIX clause).

## Conclusion

No interface changes required by this wave. The three
commonly-misread clauses (`renice` mandatory `-n`, `paste -d ''`
permitted error, `iconv -c` status invariance) are implemented
correctly, test-pinned, and now documented with verbatim Issue 7
citations.
