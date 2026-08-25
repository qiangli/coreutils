# POSIX Go evidence closure: wave 7A

Wave 7A audits the Go-owned `od`, `pax`, `ps`, `renice`, `sed`, and `sort`
interfaces against POSIX.1-2008 / Issue 7, 2016 edition. The normative source
is the local interface inventory in `docs/posix-required-command-interfaces.tsv`
and its pinned Open Group XCU pages. GNU behavior is not used to turn a POSIX
requirement into an extension or vice versa.

Classification follows `docs/reference-policy.md`: implementation plus a
focused behavioral test is verified; apparently correct code without a focused
test remains an evidence gap; missing or observably wrong required behavior is
an implementation gap. Platform facilities and locale data that the repository
does not provide are stated as residuals rather than silently claimed.

## Results

| Command | Issue 7 source | Verdict | Verified interface evidence | Residual attribution |
| --- | --- | --- | --- | --- |
| `od` | [XCU od](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html) | partial | All required `-A`, `-j`, `-N`, `-t`, and `-v` options; ordered and concatenated type strings; C type-size letters; XSI `-bcdosx`; gated XSI offset operands; stdin/named-file concatenation; skip/count across files; partial-item zero fill; duplicate suppression; native endian/ABI formatting; per-file continuation; output and close errors. Evidence: `cmds/od/od_test.go` (`TestODConcatenatedFormatStrings` through `TestODMissingFileContinues`). | Non-C `LC_CTYPE` rendering of `-c` and non-C `LC_NUMERIC` floating output are not implemented. Translated diagnostics and `NLSPATH` catalogs are absent. |
| `pax` | [XCU pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html) | partial | Four mode synopses, archive/stdin lanes, pax/ustar/cpio interchange, block-size grammar and physical blocking, `-acdfHkLnrsuvwxX` behaviors represented by the existing mode/traversal/selection/archive tests, substitutions, pattern selection, hierarchy traversal, append/update, hard-link archive identity, safe atomic extraction, unmatched-pattern status, and read/write error propagation. Evidence: `archive_lane_test.go`, `follow_test.go`, `wave_test.go`, and `wave_b_test.go`. | Confirmed Bashy gaps: `-i` is parsed but interactive renaming is not applied; copy-mode `-l`, write/copy `-t`, and every `-o` invocation fail loudly; `-p` implements only the `m` time behavior and does not fully implement the POSIX `a/e/m/o/p` preservation grammar. Access-time restoration after reads, complete special-file archive/extraction coverage, locale-sensitive pattern matching, and translated diagnostics remain absent. These are not external-provider failures. |
| `ps` | [XCU ps](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ps.html) | partial | All XSI option spellings are accepted. Wave 7A fixes selection-option union, `-a` session-leader exclusion, default same-effective-user-and-terminal selection, Linux real/effective UID/GID separation, proc-backed `-n` acceptance, and stdout error status. `TestPSPOSIXSelectionUnionAndDefaults` and `TestPSPOSIXNameListAcceptedAndOutputErrorsFail` pin the corrections; existing tests pin `-p`, `-o`, empty headings, and invalid formats. | Non-Linux process enrichment cannot currently distinguish real/effective IDs, controlling terminal, groups, session/process group, CPU, memory, state, or niceness because the portable enumerator lacks those values. On Linux, `F`, `S`, `PRI`, `ADDR`, `WCHAN`, and `%CPU` are placeholders rather than complete POSIX field values. `COLUMNS` width behavior, non-C `LC_TIME`, translated user/group/diagnostic data, and a hermetic process-enumerator seam remain residual. |
| `renice` | [XCU renice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html) | partial | `-n increment`, obsolescent increment operand, mutually exclusive `-g/-p/-u`, default PID selection, numeric and user-name IDs, multiple-ID continuation/status, reading the current priority, incremental adjustment, and saturation are covered by `renice_test.go` plus Unix/Windows platform tests. | Real priority changes are kernel-, privilege-, and race-dependent; Windows refuses the facility loudly. User/group database locale and translated diagnostics are absent. No hermetic seam yet proves a mixed-success multiple-ID call and exact `which` dispatch without touching live scheduler state. |
| `sed` | [XCU sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html) | partial | Required `-e/-f/-n`, script/file operand grammar, mixed script-source order, concatenated input, missing-final-newline behavior, the Issue 7 command language, BRE addresses/substitutions/back-references/intervals, write-file lifecycle, runtime error propagation, and C/POSIX plus provisioned single-byte locale seams have focused coverage in `sed_test.go`, `ctype_test.go`, `sed_fifo_unix_test.go`, and `internal/gosed/*_test.go`. | Full implementation-defined locale catalog breadth, multibyte `LC_CTYPE`/`LC_COLLATE`, translated diagnostics, and `NLSPATH` are absent. GNU `-E/-r/-i/-s` behavior is extension evidence and does not substitute for Issue 7 evidence. |
| `sort` | [XCU sort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sort.html) | partial | Required `-bcdifmnorutk`, POSIX `-C`, key grammar/modifier inheritance, stdin and ordered file input, `-o`, merge-only behavior, check status, uniqueness, stable comparison behavior, numeric parsing, C/POSIX byte collation, provisioned collation/numeric locale seams, pre-read operand validation, and output errors are covered in `sort_test.go`, `collator_test.go`, and `fastsort_test.go`. | The locale provider is intentionally narrow rather than a complete host locale database; multibyte `LC_CTYPE`, arbitrary `LC_COLLATE`/`LC_NUMERIC`, translated diagnostics, and `NLSPATH` remain absent. POSIX permits implementation-defined diagnostics and does not require the GNU-only flags documented by the package. |

## Wave 7A correction detail: `ps`

The previous selector treated the presence of any list selector as a request to
ignore `-A`, `-a`, and `-d`. POSIX selection options are additive. It also
implemented `-a` as “has a terminal”, omitting the required session-leader
exclusion, and defaulted to every process of the invoking user rather than the
same effective user *and* terminal. Linux used one identity value for both the
real and effective user/group selectors.

The corrected pure-Go path stores the invoker identity in the option set,
unions every selection predicate, reads Linux real/effective IDs from
`/proc/PID/status`, and derives the invoker terminal through the same proc
enrichment path. Tests exercise the selection function with synthetic process
records, avoiding scheduler/process races. The alternate `-n namelist` is now
accepted as an observable no-op because this implementation obtains process
data through proc APIs and does not consult a kernel image. Table flushing now
propagates output failures to status 1.

No shared generated manifest, applet matrix, aggregate count, or unrelated
command row is regenerated by this batch.
