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
| `od` | [XCU od](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html) | partial | All required option, offset, stream, binary-format, continuation, and I/O-error behavior is covered. Profile C Issue 774 additionally applies invocation-owned `LC_CTYPE` to `-t c`, including UTF-8 continuation fields, exact Latin-1 bytes, and malformed/nonprintable octals, while `LC_NUMERIC` controls every required floating format on carried ABIs. | The carried `LC_CTYPE`/`LC_NUMERIC` corpus and Unicode printability tables are bounded; translated catalogs remain a localization product gap rather than a reproduced command defect. |
| `pax` | [XCU pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html) | partial | Four mode synopses, archive/stdin lanes, pax/ustar/cpio interchange, physical block sizing from the selected archive sink, interactive/substitution preflight, copy links, source-atime reset, ordered preservation, the complete `-o` families, verbose hard/symbolic-link fields and occurrence-bound targets, custom-list effective PAX names, deterministic timestamp selection, and archive/list I/O failures now have focused evidence. Current provenance is Issues 715-717, 775, 776, and 778; see their issue audits rather than the original wave snapshot alone. | Remaining: complete special-file archive/extraction coverage, privileged ownership/device products, unsupported no-follow metadata platforms, real terminal breadth, locale-sensitive pattern matching, and uncarried legacy locale encodings. These are not external-provider failures. |
| `ps` | [XCU ps](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ps.html) | verified on Linux; platform residuals stated | All XSI option spellings are accepted. Selection options union, `-a` uses the permitted session-leader omission, `-g` selects by session ID, blank- or comma-separated lists are accepted, and default selection uses the invoking effective user and terminal. Required `-o` fields, headers, null-header widths, elapsed/CPU duration shapes, plus the XSI default/full/long layouts are exact. Linux procfs supplies truthful real/effective IDs, flags, state, priority, nice value, address, wait channel, CPU, virtual size, terminal, group/session IDs, and start data; `%CPU` and `C` are lifetime-average utilization, an algorithm POSIX leaves unspecified. `COLUMNS`, `LC_TIME`, `LC_ALL` precedence, and `TZ` are invocation-local. Enumeration and procfs reads have hermetic seams, output errors propagate, and no host command is executed. Evidence: all `cmds/ps/*_test.go`, especially `TestPSHermeticEnumeratorSelectionOrderingAndFields`, `TestPSEnvironmentIsInvocationLocal`, `TestPSEnrichLinuxProcFixture`, and `TestPSLiveLinuxEnumeratorOwnProcess`. | The portable non-Linux provider does not expose all kernel fields, so fields unavailable there use POSIX's permitted hyphen representation. Linux may likewise hide `ADDR` or `WCHAN` through procfs security settings, in which case a hyphen is emitted. The embedded locale provider is intentionally bounded to C/POSIX and de_DE and fails closed for unavailable locales. Translated user/group/diagnostic data and `NLSPATH` catalogs remain absent. |
| `renice` | [XCU renice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html) | partial | `-n increment`, obsolescent increment operand, mutually exclusive `-g/-p/-u`, default PID selection, numeric and user-name IDs, multiple-ID continuation/status, reading the current priority, incremental adjustment, and saturation are covered by `renice_test.go` plus Unix/Windows platform tests. | Real priority changes are kernel-, privilege-, and race-dependent; Windows refuses the facility loudly. User/group database locale and translated diagnostics are absent. No hermetic seam yet proves a mixed-success multiple-ID call and exact `which` dispatch without touching live scheduler state. |
| `sed` | [XCU sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html) | partial | Required command, stream, and error behavior plus invocation-owned carried-locale `LC_CTYPE`/`LC_COLLATE` classes, equivalence, case, high-byte boundaries, and nonidentity `a < ä < b` ranges in both BRE and ERE have focused coverage. Locale wave B (`5cda0c2`, corrected by `525acc1`) is the current source. | The carried provider corpus is bounded; unsupported locale/platform products fail closed. GNU `-E/-r/-i/-s` behavior remains extension evidence and does not substitute for Issue 7 evidence. |
| `sort` | [XCU sort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sort.html) | partial | Required options, key and stream behavior, output failures, carried `LC_COLLATE`/`LC_NUMERIC`, and locale-wave-B `LC_CTYPE` folding, dictionary, and nonprinting-key semantics have focused command-surface coverage. | The locale provider is intentionally narrow rather than a complete host locale database; unsupported locale/platform products fail closed. POSIX does not require the GNU-only flags documented by the package. |

## Wave 7A correction detail: `ps`

The previous selector treated the presence of any list selector as a request to
ignore `-A`, `-a`, and `-d`. POSIX selection options are additive. It also
implemented `-a` as “has a terminal” without documenting that this
implementation uses POSIX's permitted session-leader omission, and defaulted to every process of the invoking user rather than the
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

The Issue 714 closure adds an injectable process source and injectable Linux
procfs reader. This makes ordering, selection, every required field, argv[0]
versus full arguments, and unavailable kernel data testable without depending
on a racing process table. A separate Linux live test confirms that the same
production enumerator returns truthful hierarchy, identity, state, flags,
priority, memory, command, arguments, and creation time for the test process
itself. The implementation never invokes a host utility.

The renderer captures one time for the whole table. Elapsed time is
`[[dd-]hh:]mm:ss`, CPU time is `[dd-]hh:mm:ss`, and `C` and `%CPU` use the
permitted implementation-defined lifetime-average CPU algorithm. XSI default
layouts now use `C`, `TTY`, and `CMD`, while explicit `-o tty` uses its required
`TT` default heading. `LC_TIME` and `TZ` control
the start-time field through embedded providers. A positive invocation-local
`COLUMNS` value bounds each line using the active `LC_CTYPE`: C/POSIX counts
bytes, while supported UTF-8 locales use display widths (including wide and
combining characters) without splitting an encoded character. Invalid values
select the implementation default. XSI default, `-f`, `-l`, and additive
`-fl` layouts have separate exact table evidence; `-f` uses the textual owner
under its `UID` heading. Standard layouts visibly mark a detected zombie as
`<defunct>`; full layouts use bracketed command names when argv is unavailable,
without changing explicit `-o comm` or `-o args` values.

`TestPSRetainsDefaultSIGTTINDisposition` separately pins the product half of
the native harness's `SigWait -w` plus `SIGTTIN` preflight. It places the Go
`ps` helper in a non-orphan process group, stops it with `SIGTTIN` exactly at
its first output write, observes the stopped wait status, continues it, and
requires the same invocation to finish with intact output. The test runs on
both Darwin and Linux and does not use the Profiles A/B procps interposer.

No shared generated manifest, applet matrix, aggregate count, or unrelated
command row is regenerated by this batch.
