# POSIX-required command interface evidence ledger

Generated from `docs/posix-required-command-interfaces.tsv` by
`scripts/posix_manifest.py`. This ledger is an audit aid, not a normative
specification or a claim of complete POSIX conformance. States are `missing`,
`partial`, `implemented`, and `verified`:

- `missing`: no behavioral implementation evidence is available.
- `partial`: focused behavioral evidence exists, but a source-interface residual remains.
- `implemented`: normative semantics, parser coverage, and focused authored tests are complete.
- `verified`: reserved for `implemented` plus applicable byte-derived full-run/pair verification from the proprietary harness.

Integration verification is deferred and unavailable in this OSS ledger today.
`implemented` is therefore the highest currently attainable state. This is a
fail-closed deferral, not a waiver: every attempted `verified` promotion and every
non-empty `integration_evidence` value is rejected.

GNU compatibility is explicitly out of scope and deferred.

| Axis | Value | Count |
| --- | --- | ---: |
| Availability | Go | 86 |
| Availability | Shell-only | 14 |
| Availability | Provider | 16 |
| Effective owner | Go | 78 |
| Effective owner | Shell | 22 |
| Effective owner | Provider | 16 |
| Evidence | Verified | 0 |
| Evidence | Implemented | 3 |
| Evidence | Partial | 97 |
| Evidence | Missing | 16 |

The pre-integration `--require-owned-source-complete` gate accepts only
`implemented` or `verified` for the exact 78 Go plus 22 shell owners.
Final completion is deliberately fail-closed: `scripts/posix_manifest.py
--require-complete` covers all 116 rows, while `--require-owned-complete`
covers Sprint 79's 100 owned rows (78 Go plus 22 shell) without treating the
16 external-provider rows as owned implementation evidence. Both final gates accept
only `verified`. They intentionally remain red until the proprietary harness adds
a byte-derived integration gate over the authoritative complete run/pair bundle.
The parser scan below is only a conservative
source-token audit; finding a token is never proof of runtime behavior.

Evidence is lane-specific. Go references stay in `cmds/<command>`; provider
references name a command-specific test in `cmds/posixproviders`; shell semantic
references normally use `sh:<path>#<TestID>` against the sibling sh repository.
The sole approved exception is the process-level Bashy sh-entrypoint contract,
recorded as `bashy:<path>#<TestID>` on the `sh` row because it proves behavior
that exists only at the selected executable boundary. Shell
routing references separately use `bashy:<approved-path>#<TestID>` against the
sibling bashy repository and are legal only for shell-selected rows. Future verified
shell rows will require both lanes: routing evidence can never substitute for semantic
evidence, and a missing cross-repository reference fails closed.

The future integration mapping is already fixed and non-negotiable: Go and external
provider rows require Profiles C+D; shell rows require Profiles B+D. A future gate
must derive membership, denominators, results, pins, binaries, provider provenance,
and no-skip/no-cap/no-drift status from authoritative harness bytes; caller-authored
hashes or attestations cannot establish `verified`.

For implemented rows, `NONE` explicitly records an empty option-argument or
operand set; `-` in those normative slots means missing data. Likewise, paired
`-` synopsis or option fields are incomplete, and normative prose cannot be `-`.

## `alias`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
alias [alias-name[=string]...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `alias-name; alias-name=string`. With no operands, write every alias definition in a form suitable for re-entry; alias-name=string defines or replaces an alias and alias-name alone queries it; process operands in order and retain successful definitions or output when another query fails.

**Special tokens:** A trailing blank in an alias replacement causes the next command word to be checked for alias substitution; -- ends option parsing in this implementation.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write requested or all alias definitions in reusable quoted form, one per line; no output is required when only definitions are made.

**Standard error:** Used only for diagnostic messages, including an undefined queried name or standard-output failure.

**Effects:** `Defines or replaces aliases in the current shell execution environment; definitions are inherited by a subshell but changes made in a subshell do not alter the parent, and definitions do not cross separate shell invocations.`.

**Exit status:** 0 when every requested operation succeeds; greater than 0 when a queried name is undefined or output fails.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:alias`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestAliasIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteAlias`; provider=`-`; clauses=`XCU:alias:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [alias](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/alias.html).

## `ar`

**Evidence state:** `missing`.

**Applicability:** `base; xsi; development`.

**Issue 7 synopsis candidate:**

```text
ar -m -a [-v] posname archive file...
ar -m -b [-v] posname archive file...
ar -m -i [-v] posname archive file...
ar -r [-cuv] archive file...
ar -r -b [-cuv] posname archive file...
ar -r -i [-cuv] posname archive file...
[development] ar -d [-v] archive file...
[xsi] ar -m [-v] archive file...
[xsi] ar -p [-v] [-s] archive [file...]
[xsi] ar -q [-cv] archive file...
[xsi] ar -r -a [-cuv] posname archive file...
[xsi] ar -t [-v] [-s] archive [file...]
[xsi] ar -x [-v] [-sCT] archive [file...]
```

**Issue 7 required-option candidate:** `-a; -b; -c; -i; -m; -r; -u; -v`.

**Issue 7 conditional-option candidate:** `development:-d; xsi:-C,-p,-q,-s,-t,-T,-x`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `archive; file; posname`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TMPDIR; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ar`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ar:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ar](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ar.html).

## `at`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
at [-m] [-f file] [-q queuename] -t time_arg
at [-m] [-f file] [-q queuename] timespec...
at -r at_job_id...
at -l -q queuename
at -l [at_job_id...]
```

**Issue 7 required-option candidate:** `-f; -l; -m; -q; -r; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<file>; -q=<queuename>; -t=<time_arg>`.

**Operands:** `at_job_id; timespec; time; midnight; noon; now; date; today; tomorrow; increment`. -t excludes timespec; -l -q has no IDs; -r requires one or more owned job IDs; timespec operands join with spaces; queue defaults a and b is batch-reserved.

**Special tokens:** - is a pathname for -f; empty stdin is a valid empty shell program.

**Standard input:** Read text commands from stdin unless -f names the source.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; LC_TIME; SHELL; TZ`.

**Standard output:** -l writes id<TAB>date lines; submission writes nothing to stdout.

**Standard error:** Successful submission writes job id/date confirmation and diagnostics to stderr.

**Effects:** `Persist an owned shell job with invocation environment, cwd, umask, queue, mail state, and scheduled time; list/remove affect only invoking-owner jobs.`.

**Exit status:** 0 on successful submit/list/remove; greater than 0 on an error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/at`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/at`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/at/at_test.go#TestAtCreateAndListAndRemove;cmds/at/at_test.go#TestAtQueueSubmissionAndListFiltering;cmds/at/at_test.go#TestAtAcceptsEmptyAndBlankStdin;cmds/at/at_test.go#TestAtMailCompletionState;cmds/at/at_test.go#TestAtLCTimeParsingAndUnsupportedLocale;cmds/at/at_test.go#TestAtOutputWriteErrorsFail;cmds/at/issue743_test.go#TestIssue743TouchTimePastDiagnosticNamesArgument;cmds/at/issue743_test.go#TestIssue743ListUnknownJobIDFails;cmds/at/issue743_access_unix_test.go#TestIssue743AtAccessMalformedPolicyFailsClosed;cmds/at/issue743_access_unix_test.go#TestIssue743AtAccessStatErrorFailsClosed;cmds/at/issue743_access_unix_test.go#TestIssue743AtAccessEmptyDenyPermits`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:at:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [at](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/at.html).

## `awk`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
awk [-F sepstring] [-v assignment]... program [argument...]
awk [-F sepstring] -f progfile [-f progfile]... [-v assignment]... [argument...]
```

**Issue 7 required-option candidate:** `-F; -f; -v`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-F=<sepstring>; -f=<progfile>; -v=<assignment>`.

**Operands:** `program; argument; file; assignment`. Without -f, the first operand is program text; with one or more -f operands, the program is the concatenation of those progfiles and all non-option operands are input file names or name=value assignments processed in ARGV order by the interpreter. -F is translated to FS before execution and -v assignments are validated and unescaped before program execution.

**Special tokens:** - as a progfile reads program text from standard input; - as an input file operand is passed to GoAWK as standard input; name=value operands are assignments rather than file names; an empty program is valid.

**Standard input:** Used as program source for -f -; otherwise used as input when no file operand is supplied or when an input file operand is -.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; xsi:NLSPATH; PATH. ENVIRON receives invocation variables;  LC_CTYPE and LC_COLLATE are wired for regex character classes/equivalence in the evidenced public locale path. LC_NUMERIC language semantics remain residual.`.

**Standard output:** Program-defined standard output with deterministic LF output.

**Standard error:** Used for parser, interpreter, invalid assignment, locale, and file diagnostics.

**Effects:** `Executes the POSIX awk program through the embedded GoAWK interpreter; reads named inputs and may create program-directed output files through interpreter semantics.`.

**Exit status:** Interpreter status is returned; parser, bad option/operand, inaccessible program/input, and locale setup errors return greater than zero.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/awk`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/awk`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/awk/awk_test.go#TestAwk;cmds/awk/awk_test.go#TestAwkPOSIXFloatFormats;cmds/awk/awk_test.go#TestAwkPOSIXOctalAlternateFormZeroPrecision;cmds/awk/awk_test.go#TestAwkPOSIXEREBackendExpressions;cmds/awk/awk_test.go#TestAwkPOSIXEREDotNewlineAndLeftmostLongest;cmds/awk/awk_test.go#TestAwkProgramFile;cmds/awk/awk_test.go#TestAwkPOSIXInterfaceProgramFileAndAssignments;cmds/awk/awk_test.go#TestAwkPOSIXProgramFromStdinAndEmptyProgram;cmds/awk/awk_test.go#TestAwkInvalidAssignmentAndPOSIXMissingInput;cmds/awk/awk_locale_test.go#TestAwkLocaleRegexEveryEndpoint;cmds/awk/awk_locale_test.go#TestAwkResolvesCTypeAndCollateIndependently;cmds/awk/awk_locale_test.go#TestAwkLocaleLifecycle;cmds/awk/awk_locale_test.go#TestAwkLocaleEquivalenceClassMatches;cmds/awk/awk_routing_test.go#TestAwkRoutingIntervalCeiling;cmds/awk/awk_routing_test.go#TestAwkRoutingLeadingZerosBothPaths;cmds/awk/awk_routing_test.go#TestAwkRoutingMalformedQuantifiers`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:awk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [awk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/awk.html).

## `basename`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
basename string [suffix]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string; suffix`. Treat string as a pathname: apply the permitted null-string and exactly // choices; reduce an all-slash string to /; remove trailing slashes and the prefix through the last remaining slash; then remove suffix only when it is a matching non-whole suffix.

**Special tokens:** -- ends option parsing so an option-like string can be supplied as the first operand.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write the resulting string followed by a newline.

**Standard error:** Used only for diagnostic messages.

**Effects:** `No files are modified.`.

**Exit status:** 0 for successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/basename`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/basename`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/basename/basename_test.go#TestBasename;cmds/basename/basename_test.go#TestBasenameErrors;cmds/basename/basename_test.go#TestBasenameWriteErrors;cmds/basename/basename_test.go#TestBasenameEndOfOptions;cmds/basename/basename_test.go#TestBasenameByteSafety;cmds/basename/basename_test.go#TestBasenameDoesNotConsumeStdin`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:basename:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [basename](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/basename.html).

## `batch`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
batch
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No options or operands are accepted.

**Special tokens:** NONE

**Standard input:** Read a text shell program from stdin; empty input is valid.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; SHELL; TZ`.

**Standard output:** No stdout on successful submission.

**Standard error:** Successful submission writes job id/date confirmation and diagnostics to stderr.

**Effects:** `Persist a queue-b, completion-mail-enabled, load-governed at job with invocation environment, cwd, and umask.`.

**Exit status:** 0 on successful submission; greater than 0 on an error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/batch`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/batch`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/batch/batch_test.go#TestBatchSubmissionStdoutEmpty;cmds/batch/batch_test.go#TestBatchPersistsJob;cmds/batch/batch_test.go#TestBatchIsAtQueueBWithCompletionMailAndLoadMarker;cmds/batch/batch_test.go#TestBatchRejectsOperands;cmds/batch/batch_test.go#TestBatchAuthenticatedRecipientAndWriteError;cmds/batch/issue743_access_unix_test.go#TestIssue743BatchAccessMalformedDenyFailsClosed;cmds/batch/issue743_access_unix_test.go#TestIssue743BatchAccessEmptyDenyPermits`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:batch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [batch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/batch.html).

## `bc`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
bc [-l] [file...]
```

**Issue 7 required-option candidate:** `-l`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#bc`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:bc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [bc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bc.html).

## `bg`

**Evidence state:** `partial`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] bg [job_id...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `job_id`. Each job_id uses the shell job-control ID syntax and names a job to resume in the background; with no operand, resume the most recently suspended job.

**Special tokens:** The job IDs %+ and %% identify the current job and %- identifies the previous job.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write one line per resumed job as [job-number] command.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Continues each selected stopped job as a background job in the current shell execution environment.`.

**Exit status:** 0 on successful completion; greater than 0 on error. If job control is disabled, fail without placing a job in the background.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:bg`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_jobcontrol_interface_test.go#TestBgIssue7SyntheticState;sh:interp/jobcontrol_issue7_unix_test.go#TestBgIssue7SendsContinueToRealProcessGroup`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteBg`; provider=`-`; clauses=`XCU:bg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [bg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bg.html).

## `cat`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cat [-u] [file...]
```

**Issue 7 required-option candidate:** `-u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Read file operands in order; no operands selects standard input; every - operand reads the same continuing standard-input stream; -u writes each byte without delay; any input file type is accepted.

**Special tokens:** -- ends option parsing; a file operand of - selects standard input and may occur more than once.

**Standard input:** Used when no file operands are given and at each - operand, without closing or reopening the stream.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write exactly the sequence of bytes read from the inputs and nothing else; the implementation may reject a regular output file that is also an input file.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Reads inputs without modifying them; output is standard output only.`.

**Exit status:** 0 only when all input files are output successfully; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cat`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cat`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/cat/cat_test.go#TestCat;cmds/cat/cat_test.go#TestCatFiles;cmds/cat/cat_test.go#TestCatUnbufferedFlushesBeforeEOF;cmds/cat/cat_test.go#TestCatErrors;cmds/cat/cat_test.go#TestCatSameFile;cmds/cat/cat_test.go#TestCatWriteError;cmds/cat/cat_test.go#TestCatBrokenPipeHonorsInheritedSIGPIPE;cmds/cat/cat_signal_unix_test.go#TestCatProcessSIGPIPEBehavior;cmds/cat/cat_fifo_test.go#TestCatFIFOSymlink;cmds/cat/cat_io_test.go#TestCatInjectedReadErrorContinues;cmds/cat/cat_io_test.go#TestCatSpecialFileOperands`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:cat:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [cat](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html).

## `cd`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cd [-L|-P] [directory]
cd -
```

**Issue 7 required-option candidate:** `-L; -P`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `directory; -`. With no directory operand, use HOME; with one directory operand, resolve it directly or through CDPATH as specified. -L processes dot-dot components logically before symbolic-link resolution; -P resolves the physical pathname, with the last -L/-P option winning. More than one operand is a usage error.

**Special tokens:** The directory operand - selects OLDPWD and writes the resulting directory. A non-empty CDPATH selection also writes the selected pathname. An empty directory is a diagnostic failure; Bash's empty-HOME behavior is a documented extension.

**Standard input:** Not used.

**Environment:** `CDPATH; HOME; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; OLDPWD; PWD`.

**Standard output:** Write the new directory followed by a newline for cd - and for a successful non-empty CDPATH search; otherwise no output. A write failure makes the command fail after the directory change has occurred.

**Standard error:** Used for usage, missing HOME/OLDPWD, inaccessible directory, and standard-output failure diagnostics.

**Effects:** `Changes the shell execution environment's current working directory; updates PWD and OLDPWD on success and leaves them and the working directory unchanged on lookup/change failure.`.

**Exit status:** 0 when the directory is changed successfully and required output is written; greater than 0 on failure (usage errors use the repository's status 2 convention).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:cd`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestCdIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteCd`; provider=`-`; clauses=`XCU:cd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [cd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cd.html).

## `chgrp`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
chgrp [-h] group file...
chgrp -R [-H|-L|-P] group file...
```

**Issue 7 required-option candidate:** `-h; -H; -L; -P; -R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `group; file`. First operand is the group: looked up as a group name in the group database first and read as a numeric group ID only when no such name exists, so a numeric operand that exists as a group name selects that group's ID; each following file operand is processed independently and a failure (unreadable operand, unreadable directory during -R, failed change) is diagnosed, sets exit >0, and continues with the rest of the hierarchy and the remaining operands.

**Special tokens:** -H/-L/-P are recognized only with -R, mutually override with the last one specified winning (including within clusters such as -RLP), and without any of them -R defaults to -P physical traversal (POSIX leaves the default unspecified; -P is the pinned choice); -H follows a symbolic-link operand (through a whole link chain) but no link met during descent, -L follows every link to a directory with cycle detection, -P follows none and changes the link's own group; -h targets the link instead of the referent and is orthogonal to traversal; -- ends options so a following operand spelled like -H or -h is a filename; - is an ordinary filename, never standard input.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used (extension: -v/-c reports).

**Standard error:** Used only for diagnostic messages.

**Effects:** `Sets the group ID of each selected file via an unconditional chown()/lchown()-equivalent call, issued even when the file already has the requested group, so the kernel's POSIX side effects (clearing set-user-ID/set-group-ID on regular files for unprivileged callers, ctime update) still occur.`.

**Exit status:** 0 when the utility executed successfully and all requested changes were made; greater than 0 if an error occurred (1 for operational failures, 2 for usage errors per the documented repo deviation, both within the POSIX >0 contract).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/chgrp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/chgrp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/chgrp/chgrp_test.go#TestChgrpSelf;cmds/chgrp/chgrp_test.go#TestChgrpErrors;cmds/chgrp/chgrp_test.go#TestChgrpNoDereference;cmds/chgrp/chgrp_test.go#TestChgrpWindows;cmds/chgrp/hierarchy_unix_test.go#TestChgrpTraversalModes;cmds/chgrp/hierarchy_unix_test.go#TestChgrpSymbolicLinkTargetOfTheChange;cmds/chgrp/hierarchy_unix_test.go#TestChgrpTraversalAndDereferenceAreOrthogonal;cmds/chgrp/hierarchy_unix_test.go#TestChgrpCommandLineLinkChainIsFollowed;cmds/chgrp/hierarchy_unix_test.go#TestChgrpNameIsPreferredOverNumber;cmds/chgrp/hierarchy_unix_test.go#TestChgrpUnchangedGroupStillCallsChown;cmds/chgrp/hierarchy_unix_test.go#TestChgrpContinuesPastFailures;cmds/chgrp/hierarchy_unix_test.go#TestChgrpDanglingSymbolicLink;cmds/chgrp/hierarchy_unix_test.go#TestChgrpSymbolicLinkCycleTerminates;cmds/chgrp/hierarchy_unix_test.go#TestChgrpDoubleDashEndsOptions;cmds/chgrp/hierarchy_unix_test.go#TestChgrpDashOperandIsAFileName;cmds/chgrp/hierarchy_unix_test.go#TestChgrpCycleDiagnostic;cmds/chgrp/hierarchy_unix_test.go#TestChgrpOutputFailureSetsStatusAndContinues;cmds/chgrp/s79_unix_test.go#TestChgrpObservedOwnerProvider;cmds/chgrp/s79_unix_test.go#TestChgrpTransitionFailuresContinue;cmds/chgrp/s79_unix_test.go#TestChgrpNumericGrammarAndLookupFailures;cmds/chgrp/s79_unix_test.go#TestChgrpFromNumericGrammarAndLookupFailures;cmds/chgrp/s79_unix_test.go#TestChgrpFromRuntimeRejectsInvalidOrUnavailableOwner;cmds/chgrp/s79_unix_test.go#TestChgrpNativeCtimeSymlinkAndHardLinkIdentity`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:chgrp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [chgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chgrp.html).

## `chmod`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
chmod [-R] mode file...
```

**Issue 7 required-option candidate:** `-R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `mode; file`. The first non-option operand (or a leading dash-prefixed argument whose body is entirely mode characters, e.g. -w or -rx, rescued before flag parsing) is the mode; every following file operand is processed independently and a failure is diagnosed and continues with the rest at exit >0; -R changes each directory operand and every file in the hierarchy below it (symlinks are never themselves changed and by default -R neither follows symlink operands nor symlinks met during descent, with cycle detection by resolved path); without -R a symlink operand is dereferenced.

**Special tokens:** mode is octal (up to 07777, set absolutely in every invocation) or the full Issue 7 symbolic grammar: comma-separated clauses, who ugoa, one or more actions per clause with op + - =, permlist rwxXst, or a single permcopy ugo; an omitted who applies the file mode creation mask to + - = and bare = clears all file mode bits; X tests the original unmodified mode for non-directories and always applies to directories; who o with perm s is accepted and leaves set-id bits unmodified; a mode operand may begin with -.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used (extension: -v/-c per-file reports).

**Standard error:** Used only for diagnostic messages (the -f extension suppresses per-file diagnostics without changing the exit status; invalid-mode and usage diagnostics are never suppressed).

**Effects:** `Sets each named file's 07777 mode bits as computed from the mode operand via chmod(), which updates the file's last file status change timestamp; set-id requests on non-regular files pass through to the OS, whose honoring is implementation-defined; on Windows every invocation fails loudly with exit 1 because no POSIX file mode bits exist.`.

**Exit status:** 0 when every requested mode change was made; greater than 0 if an error occurred (usage errors exit 2 per the documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/chmod`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/chmod`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/chmod/chmod_test.go#TestModeApply;cmds/chmod/chmod_test.go#TestParseModeInvalid;cmds/chmod/chmod_test.go#TestExtractDashMode;cmds/chmod/chmod_test.go#TestChmodFiles;cmds/chmod/chmod_test.go#TestChmodRecursive;cmds/chmod/chmod_test.go#TestChmodErrors;cmds/chmod/chmod_test.go#TestChmodWindows;cmds/chmod/chmod_test.go#TestChmodDefaultDereferencesSymlinkOperand;cmds/chmod/chmod_posix_test.go#TestModeApplyXUsesOriginalUnmodifiedMode;cmds/chmod/chmod_posix_test.go#TestModeApplyXPreservesGNUBehaviorOutsidePOSIXMode;cmds/chmod/chmod_posix_test.go#TestModeApplyBareOpAndOWithS;cmds/chmod/chmod_posix_test.go#TestModeApplyPOSIXOctalAbsolute;cmds/chmod/chmod_posix_test.go#TestChmodPOSIXModeOctalClearsDirectorySetID;cmds/chmod/chmod_posix_test.go#TestChmodReferenceCopiesExactModeToDirectory;cmds/chmod/chmod_posix_test.go#TestChmodContinuesAfterOperandError;cmds/chmod/chmod_test.go#TestChmodNoDereferenceAbbreviationSkipsSymlinkOperand;cmds/chmod/chmod_test.go#TestChmodReferenceValueThatLooksLikeMode;cmds/chmod/recursion_unix_test.go#TestChmodRecursiveRemovingPermissionsReachesWholeHierarchy;cmds/chmod/recursion_unix_test.go#TestChmodRecursiveVisitsChildrenBeforeDirectory;cmds/chmod/recursion_unix_test.go#TestChmodRecursiveUnreadableDirectoryIsDiagnosedAndStillChanged;cmds/chmod/recursion_unix_test.go#TestChmodRecursiveSymlinkLoopTerminates;cmds/chmod/s79_unix_test.go#TestChmodSymbolicModeUsesInvocationUmask;cmds/chmod/s79_unix_test.go#TestChmodSameModeFailuresContinue;cmds/chmod/s79_unix_test.go#TestChmodNativeCtimeSetIDAndHardLinkIdentity`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:chmod:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [chmod](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chmod.html).

## `chown`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
chown [-h] owner[:group] file...
chown -R [-H|-L|-P] owner[:group] file...
```

**Issue 7 required-option candidate:** `-h; -H; -L; -P; -R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `owner[:group]; file`. First operand is owner[:group]: each half is looked up as a name in the user/group database first and read as a numeric ID only when no such name exists (a numeric spelling that exists as a name selects that account's ID); with no colon the group is unchanged; extensions accept :group (owner unchanged), owner: (owner's login group), and : (no-op); an invalid half is an invalid user/invalid group diagnostic at exit 1; each following file operand is processed independently and a failure is diagnosed, sets exit >0, and continues with the rest.

**Special tokens:** -H/-L/-P are recognized only with -R, mutually override with the last one specified winning (including within clusters such as -RLP), and without any of them -R defaults to -P physical traversal (POSIX leaves the default unspecified; -P is the pinned choice); -H follows a symbolic-link operand (through a whole link chain) but no link met during descent, -L follows every link to a directory with cycle/loop detection, -P follows none and changes the link's own IDs; -h targets the link instead of the referent and is orthogonal to traversal; -- ends options so a following operand spelled like -H or -h is a filename; - is an ordinary filename, never standard input.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used (extension: -v/-c reports).

**Standard error:** Used only for diagnostic messages.

**Effects:** `Sets the user ID (and, when a group was given, the group ID) of each selected file via an unconditional chown()/lchown()-equivalent call, issued even when the file already has the requested IDs, so the kernel's POSIX side effects (clearing set-user-ID/set-group-ID on regular files for unprivileged callers, ctime update) still occur.`.

**Exit status:** 0 when the utility executed successfully and all requested changes were made; greater than 0 if an error occurred (1 for operational failures, 2 for usage errors per the documented repo deviation, both within the POSIX >0 contract).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/chown`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/chown`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/chown/chown_test.go#TestChownSelf;cmds/chown/chown_test.go#TestChownErrors;cmds/chown/chown_test.go#TestChownNoDereference;cmds/chown/chown_test.go#TestChownWindows;cmds/chown/hierarchy_unix_test.go#TestChownTraversalModes;cmds/chown/hierarchy_unix_test.go#TestChownSymbolicLinkTargetOfTheChange;cmds/chown/hierarchy_unix_test.go#TestChownTraversalAndDereferenceAreOrthogonal;cmds/chown/hierarchy_unix_test.go#TestChownCommandLineLinkChainIsFollowed;cmds/chown/hierarchy_unix_test.go#TestChownNameIsPreferredOverNumber;cmds/chown/hierarchy_unix_test.go#TestChownUnchangedOwnershipStillCallsChown;cmds/chown/hierarchy_unix_test.go#TestChownContinuesPastFailures;cmds/chown/hierarchy_unix_test.go#TestChownDanglingSymbolicLink;cmds/chown/hierarchy_unix_test.go#TestChownSymbolicLinkCycleTerminates;cmds/chown/hierarchy_unix_test.go#TestChownMutualSymbolicLinkLoopTerminates;cmds/chown/hierarchy_unix_test.go#TestChownDoubleDashEndsOptions;cmds/chown/hierarchy_unix_test.go#TestChownDashOperandIsAFileName;cmds/chown/hierarchy_unix_test.go#TestChownCycleDiagnostic;cmds/chown/hierarchy_unix_test.go#TestChownOutputFailureSetsStatusAndContinues;cmds/chown/s79_unix_test.go#TestChownTransitionFailuresContinue;cmds/chown/s79_unix_test.go#TestChownNumericGrammarAndLookupFailures;cmds/chown/s79_unix_test.go#TestChownNativeCtimeSymlinkAndHardLinkIdentity`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:chown:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [chown](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html).

## `cksum`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cksum [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. For every operand in order, calculate the Issue 7 Ethernet-polynomial CRC including the encoded file length and count its octets; no operands selects standard input; process later operands after an earlier error.

**Special tokens:** -- ends option parsing; this implementation treats a file operand of - as standard input, an Issue 7-permitted implementation choice.

**Standard input:** Used when no file operand is supplied and for each - operand; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** For each successful named file write checksum, octet count, pathname, and newline separated by spaces; omit pathname and its leading space for no-operand standard input.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Reads inputs without modifying them; output is standard output only.`.

**Exit status:** 0 only when all files are processed successfully; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cksum`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cksum`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/cksum/cksum_test.go#TestCKSumStdinAndFiles;cmds/cksum/cksum_test.go#TestCKSumErrors;cmds/cksum/cksum_test.go#TestCKSumReportsStandardOutputWriteError;cmds/cksum/cksum_test.go#TestCKSumStandardInputOperandAndReadError`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:cksum:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [cksum](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cksum.html).

## `cmp`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cmp [-l|-s] file1 file2
```

**Issue 7 required-option candidate:** `-l; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file1; file2`. Compare file1 and file2 byte by byte from the beginning, numbering bytes and lines from 1; a - operand reads standard input; identical files produce no output. POSIXLY_CORRECT requires exactly two operands; GNU-diffutils omitted-file2 and trailing-SKIP extensions remain outside that mode.

**Special tokens:** -- ends option parsing; a file operand of - selects standard input. Both operands selecting stdin, or aliases of one FIFO/block/character special file, are undefined by Issue 7 and are refused deterministically.

**Standard input:** Used only when a file operand is -.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Default mode writes "%s %s differ: char %d, line %d" for the first difference; -l lists every difference as "%d %o %o" per line, exactly in POSIX mode (POSIXLY_CORRECT) and with GNU-diffutils column alignment otherwise; -s writes nothing.

**Standard error:** Used only for diagnostic messages; a shorter identical-prefix file yields "cmp: EOF on %s" plus implementation-defined additional information in default and -l modes, and a normal-output write failure is diagnosed.

**Effects:** `Reads inputs without modifying them; no output files.`.

**Exit status:** 0 files identical; 1 files differ; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cmp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cmp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/cmp/cmp_test.go#TestCmpIdentical;cmds/cmp/cmp_test.go#TestCmpDiffer;cmds/cmp/cmp_test.go#TestCmpVerbose;cmds/cmp/cmp_test.go#TestCmpSilent;cmds/cmp/cmp_test.go#TestCmpEOF;cmds/cmp/cmp_test.go#TestCmpRejectsRepeatedStandardInput;cmds/cmp/cmp_test.go#TestCmpErrors;cmds/cmp/cmp_posix_test.go#TestCmpVerbosePOSIXModeFormat;cmds/cmp/cmp_posix_test.go#TestCmpVerboseEOFDiagnostic;cmds/cmp/cmp_posix_test.go#TestCmpPOSIXOperandGrammarAndOutputErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:cmp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [cmp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cmp.html).

## `comm`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
comm [-123] file1 file2
```

**Issue 7 required-option candidate:** `-1; -2; -3`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file1; file2`. Exactly two operands, each a text file assumed sorted in the collating sequence; one operand (not both) may be - for standard input, and both being - is a usage error. A missing, single, or third operand is a usage error. Whole lines are compared: bytewise under C/POSIX, otherwise through the invocation LC_COLLATE provider. Following GNU default order checking, an out-of-order input is diagnosed only after an unpairable line has been seen.

**Special tokens:** -- ends option parsing so a following operand spelled -1/-2/-3 names a file; a single - operand selects standard input for at most one of the two files.

**Standard input:** Read for whichever single operand is -; not read when both operands name files.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Three columns (lines only in file1, lines only in file2, lines common to both), each non-suppressed column indented by one <tab> per non-suppressed column printed before it; -1/-2/-3 suppress their columns and suppressing all three writes nothing.

**Standard error:** Used only for diagnostic messages: operand access failure, a read failure naming the operand, a detected out-of-order input, a comparison-provider failure, and a standard-output write failure.

**Effects:** `Reads the two inputs without modifying them; output is standard output only.`.

**Exit status:** 0 when the inputs were read and written as specified; greater than 0 on error (2 for usage and locale-provider errors per the documented repo deviation, 1 for input/output failure and detected disorder).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/comm`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/comm`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/comm/comm_test.go#TestComm;cmds/comm/comm_test.go#TestCommStdin;cmds/comm/comm_test.go#TestCommDuplicateAndEmptyLines;cmds/comm/comm_test.go#TestCommStreamsBeforeEOF;cmds/comm/comm_test.go#TestCommFinalRecordWithoutDelimiter;cmds/comm/comm_test.go#TestCommOrderCheck;cmds/comm/comm_test.go#TestCommErrors;cmds/comm/comm_test.go#TestCommStandardOutputWriteFailure;cmds/comm/comm_test.go#TestCommInputReadErrorIsDiagnosed;cmds/comm/comm_test.go#TestCommUsesInvocationCollatorForMergeAndOrderChecks;cmds/comm/comm_test.go#TestCommLocaleInitFailsBeforeInputOpen;cmds/comm/comm_test.go#TestCommCAndPOSIXBypassCollator`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:comm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [comm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/comm.html).

## `command`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
command [-p] command_name [argument...]
command [-p][-v|-V] command_name
```

**Issue 7 required-option candidate:** `-p; -v; -V`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `argument; command_name`. Without -v/-V, invoke command_name with argument operands while suppressing shell-function lookup; a special built-in invoked through command loses the special declaration-error and assignment-persistence properties. -p uses a guaranteed standard-utilities PATH. -v and -V describe command_name instead of invoking it.

**Special tokens:** -v writes a reusable pathname or command word; -V writes an implementation-defined description. Bash accepts multiple names as an extension and succeeds when any resolves; no operands is a silent-success extension.

**Standard input:** Not read by command itself; an invoked command inherits standard input.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** An invoked command inherits standard output. -v writes one reusable resolution per found name and -V writes a descriptive resolution; an unresolved -v name is silent.

**Standard error:** Used for usage, command-not-found, descriptive -V lookup, and invoked-command diagnostics.

**Effects:** `Invokes the selected utility or built-in directly with the supplied arguments and environment; -v/-V only query resolution. Shell functions are bypassed.`.

**Exit status:** Without -v/-V, the invoked command's status, 127 when command_name cannot be found, and greater than 0 on command errors. With -v/-V, 0 when a name is found and greater than 0 when none is found.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:command`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestCommandIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteCommand`; provider=`-`; clauses=`XCU:command:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [command](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/command.html).

## `cp`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cp [-Pfip] source_file target_file
cp [-Pfip] source_file... target
cp -R [-H|-L|-P] [-fip] source_file... target
```

**Issue 7 required-option candidate:** `-f; -H; -i; -L; -P; -p; -R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `source_file; target_file; target`. Two operands where neither names an existing directory copy source_file (or its referent when it is a symbolic link) to target_file; with more operands or an existing-directory target each source is copied to target/ plus its last component (-R: its pathname relative to the source's parent), and a non-directory target with multiple sources is an error before any copy; -R with exactly two operands and an absent target copies the source hierarchy as target; a source directory without -R gets a diagnostic and is skipped; a source that is the same file as its destination is diagnosed and skipped before any -i prompt, as are a destination directory that a non-directory cannot replace and a directory copied into itself; each source_file is processed independently and a failure continues with the rest; - is an ordinary pathname for both source and target, never stdin/stdout.

**Special tokens:** Of -H, -L, and -P the last specified wins; without -R a symbolic-link source is followed unless -P; with -R and none of the three (unspecified per Issue 7) the copy is physical (-P): every symlink is recreated with the identical target pathname; -H follows only symlinks named as operands while links met during traversal stay physical; -L follows every symlink (a dangling link or traversal cycle is diagnosed); -R duplicates FIFOs, sockets, and where the platform allows device nodes as nodes of the same type, and creates each new directory with source-mode-modified-by-umask OR S_IRWXU while populating, restoring the umask-filtered (with -p exact) source mode afterwards; an existing destination directory's mode is left alone; -f unlinks a destination whose open failed and retries once; -p duplicates atime/mtime, uid/gid, and permission bits including S_ISUID/S_ISGID, clearing those two bits when uid/gid cannot be duplicated (ownership failure itself is silent and not an error, mode/time failures are diagnosed); -i prompts on stderr before copying to any existing non-directory destination.

**Standard input:** Read only for the response line to an -i prompt (affirmative per the C-locale yesexpr anchored at byte zero, plus the provisioned de_DE catalog; other locales are refused loudly); otherwise not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used (the -v/--debug extensions emit traces only when requested).

**Standard error:** Used for the -i overwrite prompt, which contains the destination pathname, and for diagnostic messages.

**Effects:** `Duplicates the contents of each source_file at the destination path (existing regular destinations are truncated and rewritten, absent ones created with the source permission bits as the open mode); -R duplicates whole hierarchies including special files and symlinks per the selected -H/-L/-P token; -p additionally duplicates timestamps, ownership, and mode with the setuid/setgid clearing rule; -f removes an unopenable destination before retrying; nothing is written to a destination whose source was diagnosed as the same file, a directory without -R, or unreadable.`.

**Exit status:** 0 when all source files were copied successfully, including runs whose only skips are declined -i responses (pinned as not an error); greater than 0 if any diagnostic was written (same file, omitted directory, skipped hierarchy, failed open/read/write, failed mode or time preservation), with processing always continuing to the remaining operands first.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/cp/cp_test.go#TestCpFile;cmds/cp/cp_test.go#TestCpIntoDir;cmds/cp/cp_test.go#TestCpMultipleToNonDir;cmds/cp/cp_test.go#TestCpOmitsDirWithoutR;cmds/cp/cp_test.go#TestCpRecursive;cmds/cp/cp_test.go#TestCpSameFile;cmds/cp/cp_test.go#TestCpIntoItself;cmds/cp/cp_test.go#TestCpSymlinkInTree;cmds/cp/cp_test.go#TestCpRecursiveDereferenceAllSymlinks;cmds/cp/cp_test.go#TestCpDereferenceOptionOrdering;cmds/cp/cp_test.go#TestCpForceUnwritableDest;cmds/cp/cp_test.go#TestCpPreserve;cmds/cp/cp_test.go#TestCpInteractiveDecline;cmds/cp/cp_test.go#TestCpInteractiveEOFDeclines;cmds/cp/cp_test.go#TestCpInteractiveDeclineContinues;cmds/cp/cp_test.go#TestCpInteractiveDeclineThenError;cmds/cp/cp_test.go#TestCpInteractiveAffirmativeMatching;cmds/cp/cp_test.go#TestCpInteractiveAccept;cmds/cp/cp_test.go#TestCpMissingSource;cmds/cp/cp_posix_test.go#TestCpInteractiveSameFileDiagnosesWithoutPrompt;cmds/cp/cp_posix_test.go#TestCpUpdateDoesNotBypassSameFileDiagnostic;cmds/cp/cp_posix_test.go#TestCpInteractiveDirDestDiagnosesWithoutPrompt;cmds/cp/cp_posix_test.go#TestCpRecursiveDirOverExistingNonDirContinues;cmds/cp/cp_posix_test.go#TestCpDashOperandsAreOrdinaryPathnames;cmds/cp/cp_posix_unix_test.go#TestCpPreserveModeAppliedAfterOwnership;cmds/cp/cp_posix_unix_test.go#TestCpPreserveClearsSetuidWhenOwnershipFails;cmds/cp/cp_umask_unix_test.go#TestCpRecursiveNewDirUmaskFinalMode;cmds/cp/cp_umask_unix_test.go#TestCpRecursiveNewDirPreserveModeIgnoresUmask;cmds/cp/cp_umask_unix_test.go#TestCpRecursiveReadOnlySourceDirPopulates;cmds/cp/cp_umask_unix_test.go#TestCpRecursiveExistingDirKeepsMode;cmds/cp/cp_fifo_unix_test.go#TestCpFIFORecursiveRecreatesNode;cmds/cp/cp_fifo_unix_test.go#TestCpRecursiveRecreatesUnixSocket;cmds/cp/cp_posix_test.go#TestCpDoesNotCreateMissingDestinationParents;cmds/cp/cp_posix_test.go#TestCpPreserveFailsLoudlyWhenAccessTimeIsUnavailable;cmds/cp/cp_posix_test.go#TestCpNoClobberDoesNotHideSameFile;cmds/cp/cp_posix_test.go#TestCpPhysicalSymlinkSameFileIsNotReplaced;cmds/cp/cp_posix_test.go#TestCpPhysicalSymlinkDoesNotReplaceDestinationDirectory;cmds/cp/cp_posix_test.go#TestCpRecursiveRejectsSymlinkAliasedDestinationInsideSource;cmds/cp/cp_posix_test.go#TestCpRecursiveRejectsDestinationAliasedToSourceSubdirectory;cmds/cp/cp_symlink_preserve_linux_darwin_test.go#TestCpPreservePhysicalSymlinkMetadataWithoutMutatingReferent;cmds/cp/cp_umask_unix_test.go#TestCpRecursiveHonorsVirtualUmaskForAllCreatedTypes;cmds/cp/path_resolver_linux_test.go#TestCpNearPathMaxWithVirtualWorkingDirectory`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:cp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [cp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html).

## `crontab`

**Evidence state:** `partial`.

**Applicability:** `base; optional`.

**Issue 7 synopsis candidate:**

```text
crontab [file]
[optional] crontab [-e|-l|-r]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-e,-l,-r`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. At most one file; no file reads stdin; -e, -l, and -r are mutually exclusive and take no file.

**Special tokens:** - is a literal pathname, not stdin.

**Standard input:** Read replacement table from stdin only when file is omitted.

**Environment:** `EDITOR; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** -l writes the installed source; successful install/remove are silent.

**Standard error:** Diagnostics and editor failures use stderr.

**Effects:** `Replace, remove, or edit the invoking user's table atomically; installation parses jobs with cron execution environment.`.

**Exit status:** 0 on success; greater than 0 on error without replacing an invalid table.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/crontab`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/crontab`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/crontab/crontab_test.go#TestCrontabInstallListRemove;cmds/crontab/crontab_test.go#TestCrontabInvalidScheduleIsAtomic;cmds/crontab/crontab_test.go#TestCrontabPreservesWholeSourceAndEditorInputByteForByte;cmds/crontab/crontab_test.go#TestCrontabInstallSilent;cmds/crontab/crontab_test.go#TestCrontabRejectsConflictingModesAndExtraOperands;cmds/crontab/issue743_test.go#TestIssue743BackslashOnlyEscapesPercent;cmds/crontab/issue743_test.go#TestIssue743TrailingBackslashIsLiteral`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:crontab:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [crontab](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/crontab.html).

## `csplit`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
csplit [-ks] [-f prefix] [-n number] file arg...
```

**Issue 7 required-option candidate:** `-f; -k; -n; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<prefix>; -n=<number>`.

**Operands:** `file; /rexp/[offset]; %rexp%[offset]; line_no; {num}`. The first operand is the input file (- for standard input); each following POSIX operand is a line number, a /rexp/[offset] context split, a %rexp%[offset] skip split, or a {num} repetition of the immediately preceding pattern. rexp is a basic regular expression and an offset (+N, -N, or N) shifts the split line. A line number or match resolving past the last line, or a repetition exhausting the input, is an error. The GNU {*} extension repeats the preceding pattern as many times as possible.

**Special tokens:** {num} repeats the preceding pattern num additional times; a - file operand selects standard input; a delimiter escaped inside a regexp (\/ or \%) is a literal pattern character, not the closing delimiter. GNU's {*} repeat-to-end token is accepted as an extension.

**Standard input:** Read when the file operand is -.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Unless POSIX -s or the GNU -q/--quiet extension is selected, write the decimal byte count of each written piece on its own line, in order.

**Standard error:** Used only for diagnostic messages: an unopenable input, an out-of-range line number, an unmatched pattern, an invalid repeat count or suffix format, and a write failure.

**Effects:** `Writes each section between split points to a separate file named PREFIX plus a formatted numeric suffix (default prefix xx, default two-digit suffix); on any error the files already created are removed unless -k is given. GNU extensions -z/--elide-empty-files and --suppress-matched respectively elide empty pieces and omit a matched line.`.

**Exit status:** 0 when every section was written successfully; greater than 0 on error (usage errors exit 2 per the documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/csplit`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/csplit`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/csplit/csplit_test.go#TestCsplitLineNumber;cmds/csplit/csplit_test.go#TestCsplitRegexAndPrefix;cmds/csplit/csplit_test.go#TestCsplitRepeatedRegexAdvances;cmds/csplit/csplit_test.go#TestCsplitRepeatToEOF;cmds/csplit/csplit_test.go#TestCsplitRegexOffsets;cmds/csplit/csplit_test.go#TestCsplitPatternsAreBRE;cmds/csplit/csplit_test.go#TestCsplitEscapedDelimiterInRegex;cmds/csplit/csplit_test.go#TestCsplitSuffixSuppressRepeatAndElideEmpty;cmds/csplit/csplit_test.go#TestCsplitLineNumberRepeatAdvances;cmds/csplit/csplit_test.go#TestCsplitLineNumberRepeatToEOFCleansUp;cmds/csplit/csplit_test.go#TestCsplitLineNumberRepeatOutOfRange;cmds/csplit/csplit_test.go#TestCsplitKeepFilesRetainsOutputOnError;cmds/csplit/csplit_test.go#TestCsplitErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:csplit:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [csplit](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/csplit.html).

## `ctags`

**Evidence state:** `missing`.

**Applicability:** `base; development`.

**Issue 7 synopsis candidate:**

```text
ctags -x pathname...
[development] ctags [-a] [-f tagsfile] pathname...
```

**Issue 7 required-option candidate:** `-x`.

**Issue 7 conditional-option candidate:** `development:-a,-f`.

**Issue 7 option-argument candidate:** `-f=<tagsfile>`.

**Operands:** `file.c; file.h; file.f`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ctags`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ctags:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ctags](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ctags.html).

## `cut`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cut -b list [-n] [file...]
cut -c list [file...]
cut -f list [-d delim] [-s] [file...]
```

**Issue 7 required-option candidate:** `-b; -c; -d; -f; -n; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-b=<list>; -c=<list>; -d=<delim>; -f=<list>`.

**Operands:** `file`. Zero or more file operands are processed in order; no operand or - selects standard input. Exactly one of -b, -c, -f selects the mode and its list is comma- or blank-separated ranges (N, N-, N-M, -M) numbered from 1 with no 0 and no decreasing range. -d sets the single-character field delimiter (field mode only), -s suppresses lines that contain no delimiter (field mode only), and -n (byte mode) keeps multi-byte characters unsplit.

**Special tokens:** -- ends option parsing; a - file operand selects standard input and may be mixed with named files.

**Standard input:** Read when no file operand is given or for each - operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. LC_CTYPE drives -c/-n character boundaries and the -d delimiter character count over exact-byte C/POSIX plus carried UTF-8 and ISO-8859-1 decoding;  arbitrary installed locale providers remain residual and fail before input.`.

**Standard output:** For each input line, the selected bytes, characters, or fields in input order with overlapping and adjacent ranges merged; in field mode a line containing no delimiter is written unchanged unless -s.

**Standard error:** Used only for diagnostic messages: an invalid list, a conflicting mode or option combination, operand access failure, and a standard-output write failure.

**Effects:** `Reads inputs without modifying them; output is standard output only.`.

**Exit status:** 0 when all input files were processed successfully; greater than 0 on error (usage errors exit 2 per the documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cut`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cut`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/cut/cut_test.go#TestCutFields;cmds/cut/cut_test.go#TestCutBytesAndChars;cmds/cut/cut_test.go#TestCutBytesNoSplit;cmds/cut/cut_test.go#TestCutFiles;cmds/cut/cut_test.go#TestCutUsageErrors;cmds/cut/cut_test.go#TestCutUnknownFlag;cmds/cut/cut_test.go#TestCutStandardOutputWriteError;cmds/cut/issue736_posix_test.go#TestIssue736CharacterSpansFollowLCCType;cmds/cut/issue736_posix_test.go#TestIssue736MalformedBytesAreNeverReencoded;cmds/cut/issue736_posix_test.go#TestIssue736ByteNoSplitFollowsLCCType;cmds/cut/issue736_posix_test.go#TestIssue736MultibyteDelimiter;cmds/cut/issue736_posix_test.go#TestIssue736DelimiterMustBeOneCharacterInLocale;cmds/cut/issue736_posix_test.go#TestIssue736LCCTypePrecedence;cmds/cut/issue736_posix_test.go#TestIssue736UnsupportedLocaleFailsBeforeInput;cmds/cut/issue736_posix_test.go#TestIssue736UnsupportedLocaleFailsBeforeOpeningOperand;cmds/cut/issue736_posix_test.go#TestIssue736TextFileBoundaries`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:cut:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [cut](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html).

## `date`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
date [-u] [+format]
[xsi] date [-u] mmddhhmm[[cc]yy]
```

**Issue 7 required-option candidate:** `-u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `+format; mmddhhmm[[cc]yy]`. A leading + marks the format operand: conversion specifications (all Issue 7 specifiers including the %E/%O modified forms) are replaced and other characters copied, newline-terminated; the XSI mmddhhmm[[cc]yy] operand validates and sets the system clock, using the current year when omitted and the specified Issue 7 two-digit-year mapping.

**Special tokens:** %% literal percent; %n newline; %t tab; E and O modifier prefixes render as the unmodified conversion; an unknown conversion specification is copied literally.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TZ`.

**Standard output:** With no operand, the POSIX default format "+%a %b %e %H:%M:%S %Z %Y"; with +format, the expanded format; after a successful XSI set, the resulting date in the default format; always newline-terminated.

**Standard error:** Used only for diagnostic messages.

**Effects:** `The XSI operand sets the system clock through the platform clock API; -u interprets and formats as if TZ were UTC0, otherwise TZ from the invocation environment selects the timezone.`.

**Exit status:** 0 when the date was written or set and reported successfully; greater than 0 if validation, clock setting, formatting, or output failed.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/date`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/date`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/date/date_test.go#TestDateFormats;cmds/date/date_test.go#TestDateAlternativeModifiers;cmds/date/date_test.go#TestDateTZ;cmds/date/date_test.go#TestDateLCTimePrecedence;cmds/date/date_test.go#TestDateLCTimeCompleteFormatsAndUnsupported;cmds/date/date_test.go#TestDateDefaultShape;cmds/date/date_test.go#TestDateXSISetDateOperand;cmds/date/date_test.go#TestDateXSISetDateRejectsBeforeMutationAndPropagatesFailure;cmds/date/date_test.go#TestDateXSIYearDefaultUsesSelectedTimezone;cmds/date/date_test.go#TestDateErrors;cmds/date/date_test.go#TestDateInvalidUsageDiagnostics;cmds/date/date_test.go#TestDateWriteErrorDiagnostic`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:date:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [date](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/date.html).

## `dd`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
dd [operand...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `if=file; of=file; ibs=expr; obs=expr; bs=expr; cbs=expr; skip=n; seek=n; count=n; conv=value[,value ...]; ascii; ebcdic; ibm; block; unblock; lcase; ucase; swab; noerror; notrunc; sync`. KEY=VALUE operands select files, blocks, limits, offsets, and conversions; invalid numbers/conversion conflicts fail before copying; bs overrides ibs/obs.

**Special tokens:** Omitting if= selects standard input and omitting of= selects standard output; a value of - is a literal pathname, not a stream alias.

**Standard input:** Read stdin when if is absent.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write converted records to stdout when of is absent.

**Standard error:** Write record counts and diagnostics to stderr; POSIX mode omits the GNU byte-count extension.

**Effects:** `Copy/convert bytes, applying seek/truncation, noerror/sync, XSI codeset conversion, and signal status effects.`.

**Exit status:** 0 on complete copy; greater than 0 on operand, I/O, signal, or status-write error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/dd`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/dd`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/dd/dd_test.go#TestDdPOSIXStatusOmitsGNUByteCountExtension;cmds/dd/dd_test.go#TestDdOperandSizeSyntax;cmds/dd/dd_test.go#TestDdSeekOnRedirectedStandardOutput;cmds/dd/conv_test.go#TestDdConvAsciiEbcdicRoundTrip;cmds/dd/dd_signal_unix_test.go#TestDdEmbeddedSIGINTReturnsStatusAndDoesNotSignalHost`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:dd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [dd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dd.html).

## `df`

**Evidence state:** `partial`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] df [-k] [-P|-t] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-k,-P,-t`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Each file selects its containing filesystem; no operands lists accessible filesystems; -P and -t conflict.

**Special tokens:** - is a pathname.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write filesystem rows; default/-k include free slots; -P writes six portable fields; -t includes allocated space without a synthetic total.

**Standard error:** Per-operand and mount-discovery diagnostics use stderr.

**Effects:** `Read filesystem metadata only; no output files.`.

**Exit status:** 0 if all requested information is written; greater than 0 for an error, while successful operands remain reported.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/df`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/df`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/df/df_test.go#TestPortableExactOutput;cmds/df/df_test.go#TestXSITotalAllocatedSpaceOption;cmds/df/df_test.go#TestXSIDefaultIncludesFreeFileSlots;cmds/df/df_test.go#TestPOSIXPortableAndTotalAreMutuallyExclusive;cmds/df/df_test.go#TestMixedOperandsPreserveSuccessfulOutput`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:df:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [df](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/df.html).

## `diff`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
diff [-c|-e|-f|-u|-C n|-U n] [-br] file1 file2
```

**Issue 7 required-option candidate:** `-b; -c; -C; -e; -f; -r; -u; -U`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-C=<n>; -U=<n>`.

**Operands:** `file1, file2`. Exactly two operands; file/directory combinations use basename matching; -r recurses directory pairs.

**Special tokens:** One - operand selects stdin; two - operands are rejected.

**Standard input:** Read stdin only for one - operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TZ`.

**Standard output:** Write normal, ed, forward-ed, context, or unified difference output to stdout.

**Standard error:** Write access, traversal, and output diagnostics to stderr.

**Effects:** `Read operands only; directory traversal does not modify inputs or create output files.`.

**Exit status:** 0 no differences; 1 differences found; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/diff`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/diff`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/diff/diff_test.go#TestNormalFormat;cmds/diff/diff_test.go#TestUnifiedGolden;cmds/diff/diff_test.go#TestContextGolden;cmds/diff/diff_test.go#TestRunReportsBufferedFlushError;cmds/diff/diff_fifo_unix_test.go#TestDirectoryFIFOComparedByType`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:diff:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [diff](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/diff.html).

## `dirname`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
dirname string
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string`. POSIX takes a single string operand; this implementation also accepts multiple operands (a GNU extension), writing one result per operand. For each string: remove trailing / characters; if nothing remains the result is /; otherwise remove the last /-separated component and any / that remain, yielding / when the prefix is all slashes and . when the string contains no /. No path canonicalization is performed, so a/./b yields a/.

**Special tokens:** -- ends option parsing so a string beginning with - is treated as an operand; / is the only separator and, being a byte that cannot occur inside a multi-byte character in a POSIX encoding, is located bytewise.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** The resulting directory-name string followed by a newline, one per operand (a NUL terminator under the -z GNU extension).

**Standard error:** Used only for diagnostic messages: a missing operand and a standard-output write failure.

**Effects:** `Pure string manipulation with no filesystem access; no files are modified.`.

**Exit status:** 0 on success; greater than 0 on error (a missing operand exits 2 per the documented repo deviation, a write failure exits 1).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/dirname`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/dirname`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/dirname/dirname_test.go#TestDirname;cmds/dirname/dirname_test.go#TestDirnamePOSIXSingleOperandByteSafety;cmds/dirname/dirname_test.go#TestDirnameErrors;cmds/dirname/dirname_test.go#TestDirnameWriteErrors;cmds/dirname/dirname_test.go#TestDirnameDoesNotConsumeStdin`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:dirname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [dirname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html).

## `du`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
du [-a|-s] [-kx] [-H|-L] [file...]
```

**Issue 7 required-option candidate:** `-a; -H; -k; -L; -s; -x`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Each file operand is a path whose hierarchy is processed and reported; POSIX does not prescribe record order. With no operand the hierarchy rooted at . is used. File operands that cannot be accessed are diagnosed on stderr and do not abort other operands.

**Special tokens:** -- ends option parsing; - and + prefixes on numeric-like arguments are not applicable because du takes no numeric option arguments; option clustering follows the utility syntax guidelines.

**Standard input:** Not used; no operand reads standard input.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** For each visited directory and, with -a, each non-directory file: one line of the form "%d %s", <blocks> <space> <path>, where the default block size is 512 bytes, rounded up, and -k selects 1024-byte units; -s emits exactly one such line per operand.

**Standard error:** Used for diagnostics only: inaccessible operands, unreadable directories, and write failures on standard output.

**Effects:** `None; du is read-only and writes no files or other state.`.

**Exit status:** 0 when every operand was processed; greater than 0 when any operand or subdirectory could not be accessed or read, or the write to standard output failed.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/du`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/du`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/du/issue7_test.go#TestDuIssue7DefaultOperandIsWorkingDirectory;cmds/du/issue7_test.go#TestDuIssue7StdoutFormatRoundsUpToBlocks;cmds/du/issue7_test.go#TestDuIssue7SummarizeFileOperand;cmds/du/issue7_test.go#TestDuIssue7OneFileSystemKeepsSameDeviceEntries;cmds/du/issue7_test.go#TestDuIssue7SymlinkNotFollowedByDefault`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:du:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [du](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/du.html).

## `echo`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
echo [string...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string; \a; \b; \c; \f; \n; \r; \t; \v; \\; \0num`. Zero or more string operands are separated by one space in output; base behavior is implementation-defined when the first operand is -n or any operand contains a backslash, while the XSI option defines the listed escape sequences.

**Special tokens:** The operand -- is data: echo does not recognize it as the Guideline 10 end-of-options delimiter. A lone - is also a string operand.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write the string operands separated by single spaces and followed by a newline; with no operands, write only a newline. XSI escape processing can suppress that newline with \c.

**Standard error:** Used only for diagnostic messages, including a standard-output failure.

**Effects:** `Writes only to standard output and does not consume standard input or modify files.`.

**Exit status:** 0 for successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:echo`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestEchoIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteEcho`; provider=`-`; clauses=`XCU:echo:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [echo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/echo.html).

## `ed`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
ed [-p string] [-s] [file]
```

**Issue 7 required-option candidate:** `-p; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-p=<string>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ed`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ed:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ed.html).

## `env`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
env [-i] [name=value]... [utility [argument...]]
```

**Issue 7 required-option candidate:** `-i`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `name=value; utility; argument`. Start with the inherited environment or, under -i, an empty environment; apply leading name=value operands in order; the first non-assignment names the utility and all later operands are its arguments; search with the resulting PATH.

**Special tokens:** -- ends env option parsing; after the first non-option operand all remaining strings belong to the assignment or utility operand sequence; Issue 7 leaves a first argument of - unspecified.

**Standard input:** Not read by env; when utility is present, pass standard input through to that utility.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** With no utility, write each resulting name=value entry followed by a newline; with a utility, env writes nothing and passes through the utility standard output.

**Standard error:** Used only for env diagnostics; an invoked utility inherits standard error.

**Effects:** `Writes the resulting environment or invokes utility directly with that environment and the supplied arguments.`.

**Exit status:** With utility, return its exit status; otherwise 0 on success, 1-125 for an env error, 126 when utility is found but cannot be invoked, and 127 when it cannot be found.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/env`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/env`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/env/env_test.go#TestEnv;cmds/env/env_test.go#TestEnvRunsCommandWithModifiedEnvironment;cmds/env/env_test.go#TestEnvCommandStdioAndExitStatus;cmds/env/env_test.go#TestEnvCommandNotFoundAndNotExecutable;cmds/env/env_test.go#TestEnvWriteErrorDiagnostic;cmds/env/env_exec_test.go#TestEnvExecPassesArgvVerbatimWithoutShell;cmds/env/env_exec_test.go#TestEnvExecChildEnvironmentOrdering;cmds/env/env_exec_test.go#TestEnvExecStdioPassthroughAndSilence;cmds/env/env_exec_test.go#TestEnvExecEmptyPathSearchesWorkingDirectoryOnly;cmds/env/env_exec_test.go#TestEnvDoubleDashEndsOptions;cmds/env/env_posix_test.go#TestEnvNoUtilityDoesNotReadStdin;cmds/env/env_posix_test.go#TestEnvValueContainsEquals`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:env:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [env](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html).

## `ex`

**Evidence state:** `missing`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] ex [-rR] [-s|-v] [-c command] [-t tagstring] [-w size] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-c,-r,-R,-s,-t,-v,-w`.

**Issue 7 option-argument candidate:** `-c=<command>; -t=<tagstring>; -w=<size>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `COLUMNS; EXINIT; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LINES; xsi:NLSPATH; PATH; SHELL; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ex`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ex:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ex](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ex.html).

## `expand`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
expand [-t tablist] [file...]
```

**Issue 7 required-option candidate:** `-t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-t=<tablist>`.

**Operands:** `file`. Zero or more file operands are processed in order; no operand or - selects standard input. -t tablist sets tab stops: a single positive decimal repeats that interval, and a comma- or blank-separated strictly ascending list sets explicit stops with a <tab> at or beyond the last stop replaced by a single <space>. In POSIX mode, difficulty accessing or reading an operand terminates processing before later operands; GNU-compatible default mode diagnoses the error and continues.

**Special tokens:** -- ends option parsing; a - file operand selects standard input. A <backspace> decrements the column counter (never below zero) and a <newline> resets it.

**Standard input:** Read when no file operand is given or for each - operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. Exact-byte C/POSIX behavior plus carried UTF-8 and ISO-8859-1 locale decoding are evidenced;   arbitrary installed locale providers remain residual.`.

**Standard output:** Each input line with every <tab> replaced by the number of <space> characters needed to reach the next tab stop; all other bytes are copied unchanged.

**Standard error:** Used only for diagnostic messages: an invalid tablist, operand access failure, and a standard-output write failure.

**Effects:** `Reads inputs without modifying them; output is standard output only.`.

**Exit status:** 0 when all input files were processed successfully; greater than 0 on error (an invalid tablist exits 2 per the documented repo deviation, operand and output failures exit 1).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/expand`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/expand`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/expand/expand_test.go#TestExpandDefaultTabsFromStdin;cmds/expand/expand_test.go#TestExpandCustomTabsAndFile;cmds/expand/expand_test.go#TestExpandTabListIncrement;cmds/expand/expand_test.go#TestExpandTabListExtend;cmds/expand/expand_test.go#TestExpandBlankSeparatedTabList;cmds/expand/expand_test.go#TestExpandTabsBeyondLastStopBecomeSingleSpaces;cmds/expand/expand_test.go#TestExpandBackspaceDecrementsColumn;cmds/expand/expand_test.go#TestExpandRejectsBadTabs;cmds/expand/expand_test.go#TestExpandOperandAccessFailureModes;cmds/expand/expand_test.go#TestExpandStandardOutputWriteError;cmds/expand/expand_test.go#TestExpandParseTabStops;cmds/expand/issue737_posix_test.go#TestIssue737LocaleCharacterBoundariesPreserveOriginalBytes;cmds/expand/issue737_posix_test.go#TestIssue737MalformedAndCLocaleBytesAreNeverReencoded;cmds/expand/issue737_posix_test.go#TestIssue737InitialRegionAndBackspaceAcrossLocales;cmds/expand/issue737_posix_test.go#TestIssue737LCCTypePrecedence;cmds/expand/issue737_posix_test.go#TestIssue737UnsupportedLocaleFailsBeforeInput;cmds/expand/issue737_posix_test.go#TestIssue737UnsupportedLocaleFailsBeforeOpeningOperand;cmds/expand/issue737_posix_test.go#TestIssue737UsageErrorPrecedesLocaleValidation;cmds/expand/issue737_posix_test.go#TestIssue737ExitStatusMatrix;cmds/expand/issue737_posix_test.go#TestIssue737ReadAndShortWriteErrors;cmds/expand/issue737_posix_test.go#TestIssue737RunReportsReadAndShortWriteErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:expand:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [expand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html).

## `expr`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
expr operand...
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `operand`. Evaluate the expression formed by all operands after an optional -- delimiter; operands are separate tokens and no stdin data is consumed.

**Special tokens:** Operators are |, &, =, >, >=, <, <=, !=, +, -, *, /, %, :, and parentheses; leading + forces the following token to be a string operand; GNU string-function keywords remain extensions outside the POSIX base grammar.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. C/POSIX-locale expression semantics are evidenced;  locale collation and character classes beyond the implemented BRE subset remain residual.`.

**Standard output:** Write the expression result followed by a newline on successful evaluation.

**Standard error:** Used only for diagnostics, including missing operands, syntax errors, invalid regular expressions, arithmetic conversion failures, division by zero, and output errors.

**Effects:** `No filesystem effects; stdin is not consumed.`.

**Exit status:** 0 non-null/non-zero result; 1 null/zero result; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/expr`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/expr`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/expr/expr_test.go#TestExprPOSIXArithmeticAndComparison;cmds/expr/expr_test.go#TestExprPOSIXBooleanAndExitStatus;cmds/expr/expr_test.go#TestExprPOSIXMatchAndStringFunctions;cmds/expr/expr_test.go#TestExprPOSIXOperandsStdinAndDiagnostics`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:expr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [expr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html).

## `false`

**Evidence state:** `implemented`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
false
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No operands are specified.

**Special tokens:** NONE

**Standard input:** Not used.

**Environment:** `none`.

**Standard output:** Not used.

**Standard error:** Not used.

**Effects:** `Changes no state, consumes no input, and produces no output; only the exit status is observable.`.

**Exit status:** Always greater than 0.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:false`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestFalseIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteFalse`; provider=`-`; clauses=`XCU:false:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [false](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/false.html).

## `fc`

**Evidence state:** `partial`.

**Applicability:** `base; optional`.

**Issue 7 synopsis candidate:**

```text
fc -l [-nr] [first [last]]
fc -s [old=new] [first]
[optional] fc [-r] [-e editor] [first [last]]
```

**Issue 7 required-option candidate:** `-l; -n; -r; -s`.

**Issue 7 conditional-option candidate:** `optional:-e`.

**Issue 7 option-argument candidate:** `-e=<editor>`.

**Operands:** `first, last; [+]number; -number; string; old=new`. first and last select history entries by positive command number, negative relative number, or most-recent command prefix; omitted bounds select the prescribed prior command(s). Under -s, optional old=new replaces the first occurrence before re-execution.

**Special tokens:** A negative history number is an operand rather than an option when parsed in an operand position. With no -l, edited or selected commands are re-executed in the current shell environment.

**Standard input:** Not used.

**Environment:** `FCEDIT; HISTFILE; HISTSIZE; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Under -l, write each selected command as line-number, tab, command and indent continuation lines; -n suppresses line numbers.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Lists history, invokes the selected editor and executes its resulting commands, or substitutes and re-executes a selected history command in the current shell environment.`.

**Exit status:** 0 for successful listing; greater than 0 on error; otherwise the status of the command or commands executed by fc.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:fc`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/history_test.go#TestFcIssue7FormValidation;sh:interp/history_test.go#TestFcIssue7FirstSubstitutionAndMultilineListing`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteFc`; provider=`-`; clauses=`XCU:fc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [fc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fc.html).

## `fg`

**Evidence state:** `partial`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] fg [job_id]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `job_id`. job_id uses shell job-control ID syntax and names the job to run in the foreground; when omitted, select the most recently suspended, backgrounded, or asynchronously run job.

**Special tokens:** The job IDs %+ and %% identify the current job and %- identifies the previous job.

**Standard input:** Not used by fg itself; the resumed foreground job receives the shell foreground terminal/input according to job control.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write the selected job command line followed by a newline.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Moves the selected job into the foreground, continues it if stopped, and waits according to shell job-control semantics.`.

**Exit status:** 0 on successful completion; greater than 0 on error. If job control is disabled, fail without placing a job in the foreground.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:fg`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_jobcontrol_interface_test.go#TestFgIssue7ParserAndCompletedStatus;sh:interp/jobcontrol_issue7_unix_test.go#TestFgIssue7OwnsTerminalReadsWaitsAndRestores`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteFg`; provider=`-`; clauses=`XCU:fg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [fg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fg.html).

## `file`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
file [-dh] [-M file] [-m file] file...
file -i [-h] file...
```

**Issue 7 required-option candidate:** `-d; -h; -i; -M; -m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-M=<file>; -m=<file>`.

**Operands:** `file`. Each file operand is classified in argument order; at least one operand is required and its absence is a usage error. Bashy treats the operand - as standard input, an implementation-defined POSIX choice. An operand that is a symbolic link is followed by default; -h (with no -L) classifies the link itself.

**Special tokens:** -- ends option parsing; Bashy's implementation-defined - operand reads standard input; -d/-m/-M order their test sources position-sensitively.

**Standard input:** Bashy reads standard input when and only when an operand is exactly -, reporting it under that name; POSIX permits this implementation-defined interpretation.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** One line per operand of the form "%s: %s\n", <file> colon <space> <type>. A nonexistent, unreadable, or undetermined operand still produces its line containing "cannot open" and does not by itself change the exit status.

**Standard error:** Used only for diagnostics, including invalid magic-file syntax and standard-output write errors.

**Effects:** `None; file is read-only and writes no files or other state.`.

**Exit status:** 0 on success; greater than 0 when a usage error occurs or a write fails. Per the Issue 7 file page, operands that cannot be opened or classified do not by themselves affect the exit status.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/file`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/file`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/file/issue7_test.go#TestFileIssue7OperandOrderPreserved;cmds/file/issue7_test.go#TestFileIssue7StdinOperandUsedByName;cmds/file/issue7_test.go#TestFileIssue7MissingOperandIsUsageError;cmds/file/issue7_test.go#TestFileIssue7MagicOptionArgumentsAndPermutation;cmds/file/issue7_test.go#TestFileIssue7DoubleDashParsing`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:file:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [file](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/file.html).

## `find`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
find [-H|-L] path... [operand_expression...]
```

**Issue 7 required-option candidate:** `-H; -L`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `+n; n; -n; -name pattern; -path pattern; -nouser; -nogroup; -xdev; -prune; -perm [-]mode; -perm [-]onum; -type c; -links n; -user uname; -group gname; -size n[c]; -atime n; -ctime n; -mtime n; -exec utility_name [argument ...] ; ; -exec utility_name [argument ...] {} +; -ok utility_name [argument ...] ; ; -print; -newer file; -depth; ( expression ); ! expression; expression [-a] expression; expression -o expression`. One or more path operands are required by Issue 7 and name the hierarchies to search. With POSIXLY_CORRECT present, including an empty value, a path operand is required and an expression-first invocation is diagnosed; outside POSIX mode Bashy's no-path default of . remains a documented extension. Everything after the first expression-looking token is the expression, evaluated in preorder for each path, with operator precedence ! > -a (implicit between adjacent primaries) > -o. A missing start point is diagnosed on stderr; later operands continue.

**Special tokens:** -- ends the leading -H/-L/-P option scan; ( ) ! -a -o are grammar tokens; +n is more than n, -n less than n, bare n exactly n; {} in -exec/-ok is replaced by the current path; a lone ; terminates -exec/-ok arguments and {} + requests batched invocation.

**Standard input:** Not used by any primary except -ok, which reads one affirmative reply line per invocation from standard input.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** Each matching path is written by -print (the default action when the expression contains no other action), one per line, spelled as it was reached from the path operand.

**Standard error:** Used for diagnostics: unreachable start points, unreadable directories, -ok affirmations, and write failures.

**Effects:** `None by default; -exec and -ok run the named utility with the matched path substituted for {}, which is the only documented side effect.`.

**Exit status:** 0 when every operand was processed, including when an -ok reply declines an invocation; greater than 0 when a start point cannot be descended, an -exec {} + invocation exits non-zero, or a write error occurs; 2 for syntax and usage errors.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/find`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/find`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/find/issue7_test.go#TestFindIssue7OperatorPrecedence;cmds/find/issue7_test.go#TestFindIssue7NameLeadingPeriodNotSpecial;cmds/find/issue7_test.go#TestFindIssue7NumericArgumentTrichotomy;cmds/find/issue7_test.go#TestFindIssue7FollowOptionsOnlyLeading;cmds/find/issue7_test.go#TestFindIssue7DoubleDashEndsLeadingOptions;cmds/find/issue7_unix_test.go#TestFindIssue7NouserUnownedPositivePath;cmds/find/issue742_test.go#TestFindIssue7POSIXModeRequiresPathOperand;cmds/find/issue742_test.go#TestFindIssue7StatusAggregation;cmds/find/issue742_test.go#TestFindIssue7ExecSideEffectsAndBatching;cmds/find/issue742_test.go#TestFindIssue7LCAllPrecedenceForOKAffirmative;cmds/find/issue742_unix_test.go#TestFindIssue7OwnershipSeamUnassignedOwnerAndGroup;cmds/find/issue742_unix_test.go#TestFindIssue7NewerMissingReference;cmds/find/issue742_unix_test.go#TestFindIssue7UnreadableDirectoryTraversal`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:find:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [find](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/find.html).

## `fold`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
fold [-bs] [-w width] [file...]
```

**Issue 7 required-option candidate:** `-b; -s; -w`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-w=<width>`.

**Operands:** `file`. With no file operands, or a file operand of -, read standard input; otherwise process file operands in order and continue after an unreadable-file error.

**Special tokens:** Tabs advance to the next column position that is a multiple of 8; backspace decrements the column count without deleting data; carriage return resets the column count; -s breaks after the last blank that fits and preserves all bytes.

**Standard input:** Read when no file operands are given or when an operand is -.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. Exact-byte C/POSIX behavior plus carried UTF-8 and ISO-8859-1 locale decoding are evidenced;  arbitrary installed locale providers remain residual.`.

**Standard output:** Write folded input in operand order, inserting newlines as needed; no metadata is written.

**Standard error:** Used only for diagnostics, including invalid widths, unreadable file operands, and output errors.

**Effects:** `No filesystem effects other than reading named input files.`.

**Exit status:** 0 if all input was processed successfully; greater than 0 if an error occurred; usage errors return 2 in this implementation.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/fold`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/fold`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/fold/fold_test.go#TestFoldDefaultUsesDisplayColumns;cmds/fold/fold_test.go#TestFoldCharactersKeepsUTF8RunesWhole;cmds/fold/fold_test.go#TestFoldBytesPreservesUtf8UnitsAtSmallWidth;cmds/fold/fold_test.go#TestFoldPOSIXOperandsAndErrors;cmds/fold/issue735_posix_test.go#TestIssue735LocaleCharacterBoundariesPreserveOriginalBytes;cmds/fold/issue735_posix_test.go#TestIssue735MalformedAndCLocaleBytesAreNeverReencoded;cmds/fold/issue735_posix_test.go#TestIssue735SpacesAndControlCharactersAcrossLocales;cmds/fold/issue735_posix_test.go#TestIssue735LCCTypePrecedence;cmds/fold/issue735_posix_test.go#TestIssue735UnsupportedLocaleFailsBeforeInput;cmds/fold/issue735_posix_test.go#TestIssue735UnsupportedLocaleFailsBeforeOpeningOperand;cmds/fold/issue735_posix_test.go#TestIssue735ReadAndShortWriteErrors;cmds/fold/issue735_posix_test.go#TestIssue735RunReportsReadAndShortWriteErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:fold:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [fold](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html).

## `getconf`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
getconf [-v specification] system_var
getconf [-v specification] path_var pathname
```

**Issue 7 required-option candidate:** `-v`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-v=<specification>`.

**Operands:** `path_var; pathname; system_var`. Exactly one system_var operand queries a known system/confstr name; exactly path_var pathname queries a known pathconf name against pathname; unknown variables and wrong arity are usage errors. -v accepts only the programming environment this build/platform adapter can honestly report, with an empty specification rejected as unsupported.

**Special tokens:** No special operand token; -a is an extension outside the POSIX synopsis.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write the variable value followed by a newline, or undefined for a known variable with no determinate value or no limit.

**Standard error:** Used only for diagnostics, including unknown variables, unsupported -v specifications, and option/arity errors.

**Effects:** `Read-only query of platform configuration adapters and pathname configuration; no files are modified.`.

**Exit status:** 0 when the requested known variable is reported; greater than 0 for unknown names, unsupported specification, option errors, or arity errors.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/getconf`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/getconf`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/getconf/getconf_test.go#TestAgreesWithSystemGetconf;cmds/getconf/getconf_test.go#TestPathconfAgreesWithSystem;cmds/getconf/getconf_test.go#TestDarwinConfstrAdapterMatchesEveryQueryableValue;cmds/getconf/getconf_test.go#TestUnknownVariableIsAnErrorNotUndefined;cmds/getconf/getconf_test.go#TestCompileTimeMinimumsComeFromTheStandard;cmds/getconf/getconf_test.go#TestEveryInventoryNameHasAValueClass;cmds/getconf/getconf_test.go#TestDarwinRegressionValues;cmds/getconf/getconf_test.go#TestWindowsFailsClosed;cmds/getconf/getconf_test.go#TestLinuxReportsOnlyDerivedRuntimeValues;cmds/getconf/getconf_test.go#TestMandatoryTableInventoryAndCompatibilityAliases;cmds/getconf/getconf_test.go#TestPathErrorsWriteNoStdoutAndFail;cmds/getconf/getconf_test.go#TestStandardOutputFailureIsAnError;cmds/getconf/linux_posix_test.go#TestLinuxDerivedValuesMatchHostGetconf;cmds/getconf/linux_posix_test.go#TestLinuxDerivedPathValuesMatchHostGetconf;cmds/getconf/linux_posix_test.go#TestLinuxTimestampResolutionIsExactOrUndefined;cmds/getconf/linux_posix_test.go#TestLinuxProgrammingEnvironmentMatchesUbuntuOracle;cmds/getconf/linux_posix_test.go#TestLinuxDoesNotClaimOtherProgrammingEnvironments;cmds/getconf/getconf_test.go#TestAllListsAndDoesNotTakeOperands;cmds/getconf/getconf_test.go#TestUnsupportedSpecificationIsRefused;cmds/getconf/getconf_test.go#TestPOSIXArityAndOptionForms;cmds/getconf/getconf_test.go#TestMissingOperand`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:getconf:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [getconf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html).

## `getopts`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
getopts optstring name [arg...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `optstring; name`. Parse options from arg operands when supplied, otherwise from the shell positional parameters. optstring lists recognized option characters; a following colon requires an option-argument. name must be a valid shell variable name and receives the current option character.

**Special tokens:** OPTIND starts at 1 and advances through option arguments, including clusters and attached option-arguments; resetting OPTIND to 1 restarts scanning. -- ends option scanning. A leading colon in optstring selects silent error reporting. OPTARG receives an option-argument or the offending option in silent error mode and is unset where Issue 7 requires.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; OPTIND`.

**Standard output:** Not used.

**Standard error:** In normal mode, diagnose an unknown option or missing option-argument; leading-colon mode suppresses those diagnostics. Usage and invalid-name errors are diagnosed.

**Effects:** `Updates name, OPTIND, and OPTARG in the current shell execution environment; repeated calls continue the same scan.`.

**Exit status:** 0 when an option, including an option error represented by ? or :, is processed; greater than 0 at end of options or on a getopts usage/name error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:getopts`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestGetoptsIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteGetopts`; provider=`-`; clauses=`XCU:getopts:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [getopts](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getopts.html).

## `grep`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
grep [-E|-F] [-c|-l|-q] [-insvx] -e pattern_list [-e pattern_list]... [-f pattern_file]... [file...]
grep [-E|-F] [-c|-l|-q] [-insvx] [-e pattern_list]... -f pattern_file [-f pattern_file]... [file...]
grep [-E|-F] [-c|-l|-q] [-insvx] pattern_list [file...]
```

**Issue 7 required-option candidate:** `-E; -F; -c; -e; -f; -i; -l; -n; -q; -s; -v; -x`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-e=<pattern_list>; -f=<pattern_file>`.

**Operands:** `pattern_list; file`. Pattern lists come from -e operands, -f files, or the first non-option operand; newline-separated and empty patterns are honored. File operands are searched in order, with no file operand selecting standard input. POSIXLY_CORRECT disables GNU option permutation after the first operand.

**Special tokens:** - as a pattern file reads stdin only when no file named - exists in the invocation directory; - as a file operand reads standard input; -- ends option parsing.

**Standard input:** Read as input when no file operands are supplied, for file operand -, and for -f - when no file named - exists.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. LC_ALL/category/LANG precedence is evidenced for C/POSIX and the built-in de_DE ISO-8859-1 certification locale;  broader public-locale regex behavior remains residual.`.

**Standard output:** Write selected lines, counts, file names, or no normal output under -q according to the selected options; multiple files prefix names unless suppressed by extension flags.

**Standard error:** Used for diagnostics; -s suppresses file-open/read diagnostics as implemented.

**Effects:** `Read-only search of input files/streams; no files are modified.`.

**Exit status:** 0 selected lines found; 1 none found; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/grep`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/grep`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/grep/grep_test.go#TestGrepStdin;cmds/grep/grep_test.go#TestGrepFlagsOnFile;cmds/grep/grep_test.go#TestGrepQuiet;cmds/grep/grep_test.go#TestGrepBRE;cmds/grep/grep_test.go#TestGrepEREAndFixed;cmds/grep/grep_test.go#TestGrepMultiplePatterns;cmds/grep/grep_test.go#TestGrepPOSIXPatternListAndOptionArgument;cmds/grep/grep_test.go#TestGrepPatternFileNamedDashAndCombinedLists;cmds/grep/grep_test.go#TestGrepPOSIXOperandsStopOptionParsing;cmds/grep/grep_test.go#TestGrepVSCLocalePrecedence;cmds/grep/grep_test.go#TestGrepPOSIXDiagnosticsAndPatternFiles;cmds/grep/grep_test.go#TestGrepPOSIXRegexConformance;cmds/grep/grep_test.go#TestGrepOnlyMatchingUsesLeftmostLongest;cmds/grep/grep_test.go#TestGrepREDUPMAX2047Intervals;cmds/grep/literal_test.go#TestLiteralFastPathDifferential`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:grep:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [grep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/grep.html).

## `hash`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
hash [utility...]
hash -r
```

**Issue 7 required-option candidate:** `-r`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `utility`. Each utility operand asks the shell to locate and remember a PATH-searched external utility. Shell built-ins, functions, and names containing a slash are not added. With no operands, report remembered locations; -r forgets all remembered locations.

**Special tokens:** The Bash extensions -p pathname name, -t name, and -d name explicitly install, query, or delete entries and are outside the Issue 7 hash [utility...] / hash -r surface.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** With no operands, write the remembered utility table; POSIX leaves its format unspecified. POSIX operand and -r forms otherwise write no output.

**Standard error:** Used for usage, unresolved-utility, and extension query diagnostics.

**Effects:** `Updates or clears the current shell execution environment's remembered utility-location table; subshell changes do not escape.`.

**Exit status:** 0 when the requested table operation succeeds; greater than 0 when a utility cannot be found or another error occurs (usage errors use status 2).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:hash`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestHashIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteHash`; provider=`-`; clauses=`XCU:hash:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [hash](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/hash.html).

## `head`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
head [-n number] [file...]
```

**Issue 7 required-option candidate:** `-n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<number>`.

**Operands:** `file`. Copy the first number lines of every file operand, or 10 lines when -n is absent; copy an entire shorter file without error; no operands selects standard input; process multiple operands in order and continue after an open/read failure.

**Special tokens:** -- ends option parsing; this implementation treats a file operand of - as standard input, an Issue 7-permitted implementation choice.

**Standard input:** Used when no file operand is supplied and for each - operand; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write each designated input portion; with multiple operands prefix the first with ==> pathname <== and later ones with a preceding newline before the same header form.

**Standard error:** Used only for diagnostic messages, including read and standard-output failures.

**Effects:** `Reads inputs without modifying them; output is standard output only.`.

**Exit status:** 0 for successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/head`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/head`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/head/head_test.go#TestHead;cmds/head/head_test.go#TestHeadHeaders;cmds/head/head_test.go#TestHeadErrors;cmds/head/head_test.go#TestHeadWriteError;cmds/head/head_test.go#TestHeadStandardInputOperandAndReadError`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:head:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [head](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/head.html).

## `iconv`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
iconv [-cs] -f frommap -t tomap [file...]
iconv -f fromcode [-cs] [-t tocode] [file...]
iconv -t tocode [-cs] [-f fromcode] [file...]
iconv -l
```

**Issue 7 required-option candidate:** `-c; -f; -l; -s; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<fromcodeset>; -t=<tocodeset>`.

**Operands:** `file`. Convert each file operand in order into one continuing output stream; no operands selects standard input; an open failure diagnoses the operand, continues with the rest, and fails the invocation.

**Special tokens:** -- ends option parsing; a file operand of - selects standard input; a / in a -f/-t value (the charmap-file form) is not supported and is rejected before any I/O.

**Standard input:** Used when no file operands are given and for each - operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** The input characters converted to the target codeset; -l writes the supported codeset names.

**Standard error:** Used only for diagnostic messages; -s suppresses invalid-character conversion messages without changing the exit status.

**Effects:** `Reads inputs without modifying them; -c omits invalid input characters and characters unavailable in the target while preserving failure status; codeset operands use the carried encodings and aliases, slash-containing operands symbolically join two charmap files, decoders reset per operand, and one encoder produces the concatenated output stream.`.

**Exit status:** 0 successful conversion of all input; greater than 0 if an error occurs; -s never changes the status and -c keeps discarded characters counted as a failure.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/iconv`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/iconv`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/iconv/iconv_test.go#TestUTF8ToISO88591;cmds/iconv/iconv_test.go#TestFilesResolveAgainstRunContextDir;cmds/iconv/iconv_test.go#TestMalformedInputFailsAndSilentOnlySuppressesConversionMessage;cmds/iconv/iconv_test.go#TestUnrepresentableOutputFails;cmds/iconv/iconv_test.go#TestUnsupportedEncodingFailsLoudly;cmds/iconv/iconv_test.go#TestDiscardInvalidOmitsUntranslatableCharacters;cmds/iconv/iconv_test.go#TestOmittedEncodingUsesLocaleCodeset;cmds/iconv/iconv_test.go#TestOmittedEncodingHonorsLocaleCodeset;cmds/iconv/iconv_test.go#TestListEncodings;cmds/iconv/iconv_test.go#TestSuffixRejectionPreIO;cmds/iconv/iconv_discard_state_test.go#TestDiscardStatusSurvivesLaterEmptyOperand;cmds/iconv/iconv_discard_state_test.go#TestDiscardTruncatedGB18030FourByteTailFails;cmds/iconv/issue723_posix_test.go#TestIssue723MalformedStatusDoesNotDependOnDiscard;cmds/iconv/issue723_posix_test.go#TestIssue723CharmapPathnamesUseSymbolicJoin;cmds/iconv/issue723_posix_test.go#TestIssue723SilentDoesNotSuppressInputIOErrors;cmds/iconv/issue723_posix_test.go#TestIssue723ShortWritesFailForCharmapAndList;cmds/iconv/issue732_audit_test.go#TestIssue732DiscardMalformedAndTruncatedSequences;cmds/iconv/issue732_audit_test.go#TestIssue732SilentDoesNotSuppressShortOutputWrite;cmds/iconv/issue732_audit_test.go#TestIssue732LocalePrecedenceForOmittedEncoding;cmds/iconv/issue732_audit_test.go#TestIssue732FileAndStandardInputOperandsAreOrderedStreams;cmds/iconv/issue732_audit_test.go#TestIssue732ValidMultibyteCharactersCrossEveryReadBoundary;cmds/iconv/issue732_audit_test.go#TestIssue732OpenFailureContinuesAndSilentDoesNotHideIt;cmds/iconv/issue732_audit_test.go#TestIssue732HelpShowsAllPOSIXSynopses;cmds/iconv/issue732_audit_test.go#TestIssue732CharmapDropsWholeUnrepresentableMultibyteCharacter`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:iconv:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [iconv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/iconv.html).

## `id`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
id [user]
id -G [-n] [user]
id -g [-nr] [user]
id -u [-nr] [user]
```

**Issue 7 required-option candidate:** `-G; -g; -n; -r; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `user`. At most one user operand naming a login; with an operand the user database supplies the IDs and group set and the effective IDs are treated as identical to the real IDs; without an operand the invoking process is reported, including differing effective IDs.

**Special tokens:** -- ends option parsing; no operand token is special.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Default form writes "uid=%u(%s) gid=%u(%s)" with euid=/egid= inserted only when effective and real IDs differ and a groups= list of distinct affiliations; -u/-g write one ID as "%u" (the name with -n, the real ID with -r); -G writes all distinct group IDs space-separated; an unmappable name falls back to the bare number.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Read-only query of the process credentials and the user and group databases.`.

**Exit status:** 0 successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/id`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/id`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/id/id_test.go#TestIDDefault;cmds/id/id_test.go#TestIDDefaultIncludesNames;cmds/id/id_test.go#TestIDDefaultReportsRealAndEffectiveWhenDifferent;cmds/id/id_test.go#TestIDDefaultOmitsEffectiveWhenEqual;cmds/id/id_test.go#TestIDCurrentGroupsUseLiveProcessVector;cmds/id/id_test.go#TestIDOnlyFlags;cmds/id/id_unix_test.go#TestIDRealAndEffectiveSelectors;cmds/id/id_test.go#TestIDRealFlagWithOptions;cmds/id/id_test.go#TestIDRealFlag;cmds/id/id_test.go#TestIDNamedUser;cmds/id/id_test.go#TestIDNamedUserOperand;cmds/id/id_test.go#TestIDNamedUserOperandCombinations;cmds/id/id_test.go#TestIDOutputErrors;cmds/id/id_test.go#TestIDErrors;cmds/id/id_test.go#TestIDRejectsExtraUserOperand`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:id:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [id](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html).

## `jobs`

**Evidence state:** `partial`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] jobs [-l|-p] [job_id...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-l,-p`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `job_id`. Each job_id uses shell job-control ID syntax and selects a job whose status is displayed; with none, display all jobs known to the current shell.

**Special tokens:** The job IDs %+ and %% identify the current job and %- identifies the previous job; + and - markers in output identify those jobs.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** With -p, write one process ID per line. Otherwise write job number, current-job marker, state, and command; -l also includes process-group/process IDs as specified.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Reads the current shell job table without changing job state.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:jobs`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_jobcontrol_interface_test.go#TestJobsIssue7ParserAndFormatting`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteJobs`; provider=`-`; clauses=`XCU:jobs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [jobs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/jobs.html).

## `join`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
join [-a file_number|-v file_number] [-e string] [-o list] [-t char] [-1 field] [-2 field] file1 file2
```

**Issue 7 required-option candidate:** `-a; -e; -o; -t; -v; -1; -2`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-a=<file_number>; -e=<string>; -o=<list>; -t=<char>; -v=<file_number>; -1=<field>; -2=<field>`.

**Operands:** `file1, file2`. Exactly two file operands are required; either file may be -, but not both; inputs are expected to be sorted on their join fields under the active collation sequence.

**Special tokens:** Default field separation is runs of blanks with leading blanks ignored; -t char uses that character as both input and output separator; -o list accepts 0 for the join field and file.field entries for selected output fields.

**Standard input:** Read only for one file operand spelled -; using - for both operands is rejected.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. C/POSIX byte-collation behavior is evidenced;  locale collation/blank handling remain residual.`.

**Standard output:** Write joined and requested unpairable lines according to -a, -v, -e, -o, -t, and the selected join fields.

**Standard error:** Used only for diagnostics, including operand arity errors, invalid option arguments, unreadable files, and sorted-order diagnostics.

**Effects:** `No filesystem effects other than reading the two input streams/files.`.

**Exit status:** 0 on successful completion; greater than 0 on input or sorting errors; usage errors return 2 in this implementation.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/join`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/join`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/join/join_test.go#TestJoin;cmds/join/join_test.go#TestJoinStdin;cmds/join/join_test.go#TestJoinOrderCheck;cmds/join/join_test.go#TestJoinPOSIXOutputListAndUnpairableAggregation;cmds/join/join_test.go#TestJoinPOSIXFieldSeparators;cmds/join/join_test.go#TestJoinPOSIXOperandArityAndStderr`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:join:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [join](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/join.html).

## `kill`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
kill -s signal_name pid...
kill -l [exit_status]
kill [-signal_number] pid...
[xsi] kill [-signal_name] pid...
```

**Issue 7 required-option candidate:** `-l; -s; -<signal_number>`.

**Issue 7 conditional-option candidate:** `xsi:-<signal_name>`.

**Issue 7 option-argument candidate:** `-s=<signal_name>`.

**Operands:** `pid; exit_status`. A pid operand names a process ID or a job-control job ID known to the current shell execution environment; -s accepts a case-insensitive signal name without the SIG prefix, the bare numeric form selects a signal number, and -l either lists supported signal names or maps an exit-status operand to its signal name; signal 0 performs existence and permission checking without delivery; every operand is attempted and any failure makes the invocation fail.

**Special tokens:** -- ends option recognition so a negative process-group operand is not parsed as a signal selector; the XSI bare -signal_name spelling is accepted; Bash's -n spelling is retained only as an extension.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Used by -l for signal names; otherwise not used.

**Standard error:** Used for diagnostics when a signal, process, process group, or job operand is invalid or cannot be signaled.

**Effects:** `Requests delivery of the selected signal to each named process, process group, or current-shell job; signal 0 changes no process state.`.

**Exit status:** 0 when at least one matching process exists for every operand and the requested signal was sent or successfully checked; greater than 0 for an invalid signal or any unsatisfied operand. A job terminated by a signal yields a distinct wait status greater than 128; Bash compatibility chooses 128 plus the signal number.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:kill`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestKillIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteKill`; provider=`-`; clauses=`XCU:kill:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [kill](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/kill.html).

## `ln`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
ln [-fs] [-L|-P] source_file target_file
ln [-fs] [-L|-P] source_file... target_dir
```

**Issue 7 required-option candidate:** `-f; -L; -P; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `source_file; target_file; target_dir`. Both synopsis forms are implemented: with two operands where the last is not an existing directory the new entry is target_file, and with an existing-directory last operand each source_file is processed in operand order at target_dir/basename(source_file); more than two operands with a non-directory last operand is a diagnostic at exit >0. An existing destination without -f is diagnosed and left untouched while later sources continue; a destination naming the same directory entry as its source is diagnosed and left untouched even under -f, including hard-link aliases. With -f, a non-identical destination is unlinked before link creation is attempted; unlink failure leaves it intact, and a later link failure does not restore it or stop later sources. With -s the source need not exist and its operand text is stored verbatim, including dangling and self-referential names.

**Special tokens:** -L and -P affect only hard links whose source_file is a symbolic link: -L links the referent and -P the symlink directory entry; both together are accepted and the last short option wins, including combined forms; -s silently ignores both. With neither, the documented default is -P on darwin, dragonfly, freebsd, linux, netbsd, and openbsd via linkat without AT_SYMLINK_FOLLOW, and on AIX and Solaris via the POSIX link syscall; other targets retain os.Link's platform-defined default. -- ends option parsing; the accepted single-operand form and -b/-S/-i/-n/-r/-t/-T/-v and long options are extensions.

**Standard input:** Not used by any POSIX form; the extension -i prompt reads confirmations from standard input.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used by any POSIX form; the extension -v prints one line per created link.

**Standard error:** Used only for diagnostic messages (and the extension -i prompt), including existing destinations, same-entry identity, unlink failure, and link/symlink failure.

**Effects:** `Creates one hard-link directory entry per successful source, or with -s a symbolic link containing source_file verbatim. -f uses unlink semantics and therefore never removes a destination directory; same-entry detection precedes removal. No output files beyond the requested links are created.`.

**Exit status:** 0 when every specified source was linked successfully; greater than 0 if any error occurred while later source operands continued (usage errors exit 2 per the documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/ln`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/ln`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/ln/ln_test.go#TestLnHard;cmds/ln/ln_test.go#TestLnSymbolic;cmds/ln/ln_test.go#TestLnIntoDirectory;cmds/ln/ln_test.go#TestLnForce;cmds/ln/ln_test.go#TestLnForceDoesNotRemoveDestinationDirectory;cmds/ln/ln_test.go#TestLnForceRemovalPrecedesLinkErrorAndProcessingContinues;cmds/ln/ln_test.go#TestLnLogicalAndPhysicalSourceSymlink;cmds/ln/ln_test.go#TestLnDefaultHardLinkToSymlinkIsPhysical;cmds/ln/ln_test.go#TestLnLastOfLogicalPhysicalWins;cmds/ln/ln_test.go#TestLnSymbolicIgnoresLogicalPhysical;cmds/ln/ln_test.go#TestLnSymbolicDanglingSource;cmds/ln/ln_test.go#TestLnExistingDestinationDiagnosesAndContinues;cmds/ln/ln_test.go#TestLnSameFile;cmds/ln/ln_test.go#TestLnSameFileThroughHardLinkAlias;cmds/ln/ln_test.go#TestLnSymbolicSameFile;cmds/ln/ln_test.go#TestLnSameFileDirectoryForm;cmds/ln/ln_test.go#TestLnErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ln:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ln](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ln.html).

## `locale`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
locale [-a|-m]
locale [-ck] name...
```

**Issue 7 required-option candidate:** `-a; -c; -k; -m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `name`. With -a or -m no operands are accepted; with -c or -k at least one name operand is required; with no options and no operands write current locale environment variables; with name operands write information about each named locale category, keyword, or charmap, in order, continuing past unknown names with exit status >0.

**Special tokens:** -- ends option parsing so following arguments are treated as name operands.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_MONETARY; LC_NUMERIC; LC_TIME; xsi:NLSPATH`.

**Standard output:** Write requested locale environment variables (bare if set, double-quoted if derived/overridden), available locale names (-a), charmap names (-m), or keyword values (preceded by category name if -c, formatted as name="value" if -k).

**Standard error:** Used for diagnostic messages, including unknown keyword/category names, invalid option combinations, and standard-output write failures.

**Effects:** `Read-only; no filesystem or execution environment modifications.`.

**Exit status:** 0 when all requested information was written successfully; greater than 0 if an error occurred (unknown name, usage error, or stdout write failure).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/locale`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/locale`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/locale/locale_test.go#TestEnvironmentListing;cmds/locale/locale_test.go#TestEnvironmentListingOrder;cmds/locale/locale_test.go#TestKeywordValues;cmds/locale/locale_test.go#TestCharmapOperand;cmds/locale/locale_test.go#TestCategoryOperandWritesEveryKeyword;cmds/locale/locale_test.go#TestCategoryHeaderFlag;cmds/locale/locale_test.go#TestCollateHasNoKeywordsButIsNotAnError;cmds/locale/locale_test.go#TestMultipleOperands;cmds/locale/locale_test.go#TestUnknownNameIsAnErrorButLaterOperandsStillRun;cmds/locale/locale_test.go#TestUnavailableLocaleIsRefusedByName;cmds/locale/locale_test.go#TestAllLocales;cmds/locale/locale_test.go#TestCharmaps;cmds/locale/locale_test.go#TestMutuallyExclusiveOptions;cmds/locale/locale_test.go#TestLocaleDoubleDashTerminatesOptions;cmds/locale/locale_test.go#TestLocaleOutputWriteErrorsFail`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:locale:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [locale](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/locale.html).

## `localedef`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
localedef [-c] [-f charmap] [-i sourcefile] [-u code_set_name] name
```

**Issue 7 required-option candidate:** `-c; -f; -i; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<charmap>; -i=<inputfile>; -u=<code_set_name>`.

**Operands:** `name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#localedef`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:localedef:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [localedef](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/localedef.html).

## `logger`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
logger string...
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string`. One or more string operands joined in order with single spaces form the message; with no operands this implementation reads standard input one message per line (a documented non-POSIX extension).

**Special tokens:** -- ends option parsing so a leading-dash string can be logged; before -- a leading-dash word is parsed as an extension option.

**Standard input:** POSIX: not used; read only by the non-POSIX zero-operand extension form.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages (the -s extension additionally copies each logged message).

**Effects:** `Saves each message through the implementation-defined system log: on Unix, log/syslog discovers conventional local Unix-domain sockets and tries datagram then stream transport; on Windows the unavailable system log is refused loudly.`.

**Exit status:** 0 successful completion; greater than 0 if an error occurs (transport open, send, close, or read failure 1; usage 2).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/logger`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/logger`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/logger/logger_test.go#TestOperandsJoinWithSingleSpaces;cmds/logger/logger_test.go#TestNoOperandsReadsStdinOneRecordPerLine;cmds/logger/logger_test.go#TestDefaultPriorityAndTag;cmds/logger/logger_test.go#TestDashDashEndsOptions;cmds/logger/logger_test.go#TestSinkOpenFailureIsReported;cmds/logger/logger_test.go#TestSendFailureIsReported;cmds/logger/logger_test.go#TestCloseFailureIsReported;cmds/logger/logger_test.go#TestSendFailureIsNotMaskedByClose;cmds/logger/logger_test.go#TestUnsupportedFlagFailsLoudly;cmds/logger/logger_test.go#TestEmptyStdinLogsNothingAndSucceeds;cmds/logger/logger_test.go#TestSingleEmptyStringOperandLogsAnEmptyMessage;cmds/logger/logger_test.go#TestAllEmptyStringOperandsStillJoinWithSpaces;cmds/logger/logger_test.go#TestLeadingAndTrailingEmptyOperandsJoinWithSpaces;cmds/logger/sink_unix_test.go#TestLoggerCommandDeliversOverARealLocalSyslogSocket;cmds/logger/sink_unix_test.go#TestRealSyslogSinkAlwaysStampsPIDRegardlessOfDashI;cmds/logger/sink_unix_test.go#TestRealSyslogSinkOpenSendCloseLifecycle;cmds/logger/sink_unix_test.go#TestRealSyslogSinkOpenFailureIsReported;cmds/logger/sink_windows_test.go#TestLoggerCommandRefusesToClaimDeliveryOnWindows`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:logger:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [logger](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logger.html).

## `logname`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
logname
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. Any operand is rejected as a usage error.

**Special tokens:** -- ends option parsing; trailing words are still rejected operands.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** The login name followed by a newline, the "%s\n" format.

**Standard error:** Used only for diagnostic messages; "logname: no login name" when no name can be determined.

**Effects:** `The name comes from a getlogin()-equivalent source only, never the environment or the effective user: the kernel audit login uid mapped through the user database on Linux; every other platform currently always reports no login name.`.

**Exit status:** 0 after writing the name; 1 when no login name can be determined; 2 usage.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/logname`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/logname`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/logname/logname_test.go#TestLogname;cmds/logname/logname_test.go#TestLognameIgnoresEnvironmentAccountNames;cmds/logname/logname_test.go#TestLognameNoLoginName;cmds/logname/logname_test.go#TestLognameRejectsOperandsAndUnknownOptions;cmds/logname/logname_test.go#TestResolveLoginUID;cmds/logname/logname_test.go#TestLoginNameHasNoEffectiveUserFallback;cmds/logname/logname_test.go#TestLognameOutputErrorsAndRunContextIsolation;cmds/logname/logname_test.go#TestLoginNameFromLoginUIDEmptyOffLinux`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:logname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [logname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logname.html).

## `lp`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
lp [-c] [-d dest] [-n copies] [-msw] [-o option]... [-t title] [file...]
```

**Issue 7 required-option candidate:** `-c; -d; -m; -n; -o; -s; -t; -w`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-d=<dest>; -n=<copies>; -o=<option>; -t=<title>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; LPDEST; xsi:NLSPATH; PRINTER; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#lp`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:lp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [lp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/lp.html).

## `ls`

**Evidence state:** `partial`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] ls [-ikqrs] [-glno] [-A|-a] [-C|-m|-x|-1] [-F|-p] [-H|-L] [-R|-d] [-S|-f|-t] [-c|-u] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-A,-C,-F,-H,-L,-R,-S,-a,-c,-d,-f,-g,-i,-k,-l,-m,-n,-o,-p,-q,-r,-s,-t,-u,-x,-1`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. With no operands default to current directory; for each non-directory file write its entry; for each directory operand (unless -d) list its contents; -R recurses subdirectories; non-accessible operands draw diagnostics to stderr and continue processing remaining operands at exit status >0.

**Special tokens:** -- ends option parsing so following arguments are treated as file operands.

**Standard input:** Not used.

**Environment:** `COLUMNS; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; POSIXLY_CORRECT; TZ`.

**Standard output:** Write entry names or long-format records (-l/-g/-n/-o), multi-column (-C/-x), stream (-m), or single-column (-1); include block counts (-s) using 512-byte blocks in POSIX mode (POSIXLY_CORRECT) or 1024-byte blocks with -k.

**Standard error:** Used only for diagnostic messages (unaccessible file/directory operands, unopenable directories, invalid option usage, or unsupported capabilities).

**Effects:** `Read-only; directory traversal reads filesystem metadata without modifying files.`.

**Exit status:** 0 when all specified files were listed successfully; greater than 0 if an error occurred (inaccessible operand, directory open/read failure, usage error).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/ls`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/ls`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/ls/ls_test.go#TestDefaultSortAndOnePerLine;cmds/ls/ls_test.go#TestMixedOperandsHeaders;cmds/ls/ls_test.go#TestLongFormat;cmds/ls/ls_test.go#TestAllAndAlmostAll;cmds/ls/ls_test.go#TestDirectoryFlag;cmds/ls/ls_test.go#TestRecursive;cmds/ls/ls_test.go#TestReverse;cmds/ls/ls_test.go#TestSortTime;cmds/ls/ls_test.go#TestSortSize;cmds/ls/ls_test.go#TestInode;cmds/ls/ls_test.go#TestNonexistentOperand;cmds/ls/ls_posix_test.go#TestCommaFormatNoTrailingSeparator;cmds/ls/ls_posix_test.go#TestCommaFormatWrapsAtWidth;cmds/ls/ls_posix_test.go#TestColumnsDown;cmds/ls/ls_posix_test.go#TestColumnsAcross;cmds/ls/ls_posix_test.go#TestColumnsHonorsColumnsEnv;cmds/ls/ls_posix_test.go#TestHideControlChars;cmds/ls/ls_posix_test.go#TestSizeBlocksShortFormat;cmds/ls/ls_posix_test.go#TestDereferenceDirectoryEntries;cmds/ls/ls_posix_test.go#TestOrderIndicatorClassifyAndSlash;cmds/ls/ls_posix_test.go#TestSizeBlocksPOSIX512ByteDefault;cmds/ls/ls_posix_test.go#TestLsDoubleDashTerminatesOptions`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ls:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ls](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html).

## `m4`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
m4 [-s] [-D name[=val]]... [-U name]... file...
```

**Issue 7 required-option candidate:** `-s; -D; -U`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-D=<name[=val]>; -U=<name>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#m4`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:m4:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [m4](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/m4.html).

## `mailx`

**Evidence state:** `missing`.

**Applicability:** `base; optional`.

**Issue 7 synopsis candidate:**

```text
mailx [-s subject] address...
mailx [-HiNn] [-F] [-u user]
mailx -f [-HiNn] [-F] [file]
[optional] mailx -e
```

**Issue 7 required-option candidate:** `-f; -F; -H; -i; -n; -N; -s; -u`.

**Issue 7 conditional-option candidate:** `optional:-e`.

**Issue 7 option-argument candidate:** `-s=<subject>; -u=<user>`.

**Operands:** `address; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `DEAD; EDITOR; HOME; LANG; LC_ALL; LC_CTYPE; LC_TIME; LC_MESSAGES; LISTER; MAILRC; MBOX; xsi:NLSPATH; PAGER; SHELL; TERM; TZ; VISUAL`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#mailx`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:mailx:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [mailx](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mailx.html).

## `make`

**Evidence state:** `missing`.

**Applicability:** `development`.

**Issue 7 synopsis candidate:**

```text
[development] make [-einpqrst] [-f makefile]... [-k|-S] [macro=value...] [target_name...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `development:-e,-f,-i,-k,-n,-p,-q,-r,-S,-s,-t`.

**Issue 7 option-argument candidate:** `-f=<makefile>`.

**Operands:** `target_name; macro=value`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; MAKEFLAGS; xsi:NLSPATH; PROJECTDIR`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#make`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:make:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [make](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/make.html).

## `man`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
man [-k] name...
```

**Issue 7 required-option candidate:** `-k`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PAGER`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#man`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:man:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [man](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/man.html).

## `mesg`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mesg [y|n]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `y; n`. With no operand, report without changing the current permission state of the first terminal associated with standard input, standard output, or standard error, in that order. In the POSIX locale y grants and n denies messages by setting or clearing only group-write permission; extra or invalid operands are usage errors and cause no permission change.

**Special tokens:** -- ends option parsing so a following y or n is an operand; no options are defined.

**Standard input:** No bytes are read. The descriptors associated with standard input, then standard output, then standard error are examined in order until a terminal is found.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** With no operand, write the implementation's terminal-state report ("is y\n" or "is n\n"); y and n write nothing. Query output failures are errors.

**Standard error:** Used only for diagnostics: unavailable terminal, metadata or permission failure, invalid operands, and query-output failure.

**Effects:** `Queries or changes only the group-write bit of the selected real terminal device. Real PTY evidence proves y sets and n clears that bit without disturbing other permission bits; Windows refuses because it has no equivalent terminal message permission.`.

**Exit status:** 0 when receiving messages is allowed, including after y; 1 when receiving messages is denied, including after n; greater than 1 for every error (2 in this implementation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mesg`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mesg`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/mesg/mesg_test.go#TestQueryReportsStateThroughExitStatus;cmds/mesg/mesg_test.go#TestSetTogglesOnlyTheGroupWriteBit;cmds/mesg/mesg_test.go#TestBadOperandAndExtraOperand;cmds/mesg/mesg_test.go#TestMesgDoubleDashTerminatesOptions;cmds/mesg/mesg_test.go#TestMesgOutputWriteError;cmds/mesg/mesg_test.go#TestMesgTerminalMetadataErrors;cmds/mesg/tty_unix_test.go#TestDefaultTTYNameUsesRunContextStreamsInOrder;cmds/mesg/tty_unix_test.go#TestDefaultTTYNameRejectsCharacterDeviceThatIsNotTerminal;cmds/mesg/tty_unix_test.go#TestMesgChangesRealPTYPermissionAndStatus`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:mesg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [mesg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html).

## `mkdir`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mkdir [-p] [-m mode] dir...
```

**Issue 7 required-option candidate:** `-m; -p`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-m=<mode>`.

**Operands:** `dir`. Each dir operand is processed independently and a failure continues with the rest; without -p an operand naming an existing file or directory is an error, and with -p an operand naming an existing directory is ignored without error; a trailing / or /. names the same directory as the trimmed path; without -m the directory is created with a=rwx filtered by the file mode creation mask; -p creates each missing intermediate with mode (S_IWUSR|S_IXUSR|~filemask)&0777 so descent succeeds under a restrictive umask, while -m applies to the final directory only; an empty operand is a diagnosed error.

**Special tokens:** -m accepts octal modes (including setuid/setgid/sticky digits) and the chmod symbolic_mode grammar; + and - are relative to assumed initial mode a=rwx and omitted-who clauses consult the effective umask; the retained GNU +MODE numeric extension is accepted; - is an ordinary pathname; -m is refused loudly on Windows because POSIX mode bits are unavailable.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used (-v created-directory messages are a documented extension).

**Standard error:** Used only for diagnostic messages.

**Effects:** `Creates each named directory with the -m mode or the umask-filtered a=rwx default; with -p also creates missing ancestors, each retaining owner write and search; nothing else in the filesystem is touched.`.

**Exit status:** 0 when every specified directory was created or already existed under -p; 1 when any operand failed (remaining operands still processed); 2 on usage errors (documented repo deviation within the >0 latitude).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mkdir`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mkdir`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/mkdir/mkdir_test.go#TestMkdirSimple;cmds/mkdir/mkdir_test.go#TestMkdirExisting;cmds/mkdir/mkdir_test.go#TestMkdirParents;cmds/mkdir/mkdir_test.go#TestMkdirParentsTrailingSlash;cmds/mkdir/mkdir_test.go#TestMkdirContinuesAfterOperandError;cmds/mkdir/mkdir_test.go#TestMkdirMissingParentWithoutP;cmds/mkdir/mkdir_test.go#TestMkdirMode;cmds/mkdir/mkdir_test.go#TestMkdirModeErrors;cmds/mkdir/mkdir_test.go#TestMkdirSymbolicMode;cmds/mkdir/mkdir_test.go#TestMkdirSymbolicModeStartsAtDefault;cmds/mkdir/mkdir_test.go#TestMkdirSymbolicModeSubtractsFromDefault;cmds/mkdir/mkdir_test.go#TestMkdirSymbolicModeApply;cmds/mkdir/mkdir_test.go#TestMkdirUsageErrors;cmds/mkdir/mkdir_unix_test.go#TestMkdirParentsRetainOwnerWriteAndSearch;cmds/mkdir/mkdir_unix_test.go#TestMkdirParentsTrailingSlashFinalMode;cmds/mkdir/mkdir_unix_test.go#TestMkdirParentsRejectsDanglingSymlink;cmds/mkdir/mkdir_test.go#TestMkdirDashOperandIsPathname;cmds/mkdir/mkdir_test.go#TestMkdirEmptyOperandFailsAndContinues;cmds/mkdir/mkdir_test.go#TestMkdirVirtualUmaskDefaultAndParents;cmds/mkdir/mkdir_test.go#TestMkdirVirtualUmaskSymbolicImplicitWho;cmds/mkdir/mkdir_test.go#TestMkdirVirtualUmaskRestrictsInitialMkdirModes;cmds/mkdir/mkdir_test.go#TestMkdirVirtualUmaskCorrectionPreservesInheritedSpecialBits;cmds/mkdir/mkdir_test.go#TestMkdirInjectedFilesystemErrors;cmds/mkdir/mkdir_test.go#TestMkdirInjectedChmodErrorLeavesCreatedDirectoryAndFails;cmds/mkdir/mkdir_unix_test.go#TestMkdirParentsAcceptsSymlinkToDirectory;cmds/mkdir/mkdir_unix_test.go#TestMkdirPermissionDeniedContinuesAfterOperand;cmds/mkdir/mkdir_unix_test.go#TestMkdirVirtualUmaskPreservesInheritedSetgid;cmds/mkdir/mkdir_windows_test.go#TestMkdirVirtualUmaskDoesNotApproximatePOSIXModesOnWindows`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:mkdir:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [mkdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkdir.html).

## `mkfifo`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mkfifo [-m mode] file...
```

**Issue 7 required-option candidate:** `-m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-m=<mode>`.

**Operands:** `file`. Each file operand is a pathname at which a FIFO is created and each operand is processed independently: a failure (including an existing entry) draws a cannot-create diagnostic and processing continues with the remaining operands at exit >0; without -m the FIFO is created with a=rw (0666) modified by the file mode creation mask (the embedding shell's virtual umask when supplied, otherwise the process umask applied by mkfifo(2)); zero operands is a missing-operand diagnostic at exit 1.

**Special tokens:** -- ends option parsing so a following dash-prefixed token (including one spelled like the -m option) is an ordinary pathname; a file operand of - is likewise an ordinary pathname naming a FIFO called -; no other token is special.

**Standard input:** Not used; standard input is never read.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Creates a FIFO special file at each operand pathname; with -m the file permission bits are set to exactly the chmod-grammar mode value (octal up to 7777 or symbolic clauses); the FIFO is first created with the requested mode reduced by the creation mask and a follow-up chmod widens it up to the exact -m value, so the entry is never momentarily less restrictive (more permissive) than -m and the creation mask cannot leak into the final bits, with + and - in symbolic strings interpreted relative to an assumed initial mode of a=rw and clauses that omit who leaving umask-selected bits unchanged; on Windows every invocation fails loudly per operand rather than approximating.`.

**Exit status:** 0 when all the specified FIFO special files were created successfully; greater than 0 if an error occurred (invalid -m and missing operand exit 1; unknown options exit 2 per the documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mkfifo`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mkfifo`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/mkfifo/mkfifo_test.go#TestMkfifoCreatesFIFO;cmds/mkfifo/mkfifo_test.go#TestMkfifoDefaultModeHonorsVirtualUmask;cmds/mkfifo/mkfifo_umask_unix_test.go#TestMkfifoDefaultModeHonorsProcessUmask;cmds/mkfifo/mkfifo_test.go#TestMkfifoMode;cmds/mkfifo/mkfifo_test.go#TestMkfifoSymbolicMode;cmds/mkfifo/mkfifo_test.go#TestMkfifoSymbolicModeHonorsOmittedWhoUmask;cmds/mkfifo/mkfifo_umask_unix_test.go#TestMkfifoSymbolicModeHonorsProcessUmask;cmds/mkfifo/mkfifo_test.go#TestMkfifoOctalSpecialBits;cmds/mkfifo/mkfifo_test.go#TestMkfifoMultipleOperands;cmds/mkfifo/mkfifo_test.go#TestMkfifoPartialFailureContinues;cmds/mkfifo/mkfifo_test.go#TestMkfifoDashOperandIsPathname;cmds/mkfifo/mkfifo_test.go#TestMkfifoDoubleDashEndsOptions;cmds/mkfifo/mkfifo_test.go#TestMkfifoDoesNotConsumeStdin;cmds/mkfifo/mkfifo_test.go#TestMkfifoErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:mkfifo:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [mkfifo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkfifo.html).

## `more`

**Evidence state:** `partial`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] more [-ceisu] [-n number] [-p command] [-t tagstring] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-c,-e,-i,-n,-p,-s,-t,-u`.

**Issue 7 option-argument candidate:** `-n=<number>; -p=<command>; -t=<tagstring>`.

**Operands:** `file`. With no file operands reads standard input; otherwise processes file operands in order, continuing after open/read/close errors and returning failure if any source failed; -n requires a positive decimal number; when stdout is not a terminal, only -s affects output and the input is copied without pagination; when stdout is a terminal, a controlling-terminal command channel is required and missing terminal access is a diagnosed failure rather than silent pass-through.

**Special tokens:** -- ends option parsing; - is standard input; -number is accepted as a screen-size extension; on terminal output the required -i behavior and most required -p commands remain unsupported and fail loudly; -t is conditionally required with ctags and remains unsupported; the non-POSIX -d, -l, and -f extensions also fail loudly.

**Standard input:** Read when no file operand is supplied or an operand is -.

**Environment:** `COLUMNS; EDITOR; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; LINES; MORE; TERM`.

**Standard output:** Copied input content, paginated only when stdout is a terminal; prompts and terminal UI are kept off stdout.

**Standard error:** Used for diagnostics and terminal prompts/UI; no diagnostics are emitted for successful nonterminal copying.

**Effects:** `No filesystem changes; terminal mode is made raw only through the controlling-terminal seam while awaiting a command.`.

**Exit status:** 0 when all selected input was displayed or copied successfully; greater than 0 for input, output, terminal, unsupported-feature, or usage errors (usage follows the repository status-2 convention).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/more`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/more`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/more/more_test.go#TestMoreReadsStdin;cmds/more/more_test.go#TestMoreConcatenatesFiles;cmds/more/more_test.go#TestMoreSqueezeAndFromLine;cmds/more/more_test.go#TestMoreNonTerminalIgnoresFAndP;cmds/more/more_test.go#TestMorePOSIXNonTerminalOnlySqueezeTakesEffect;cmds/more/more_test.go#TestMorePOSIXLineCountMustBePositive;cmds/more/pager_test.go#TestPagerSeparatesContentAndTerminalUI;cmds/more/pager_test.go#TestPagerStreamsFirstScreenBeforeSourceEOF;cmds/more/pager_test.go#TestPagerPromptVisibleBeforeRead;cmds/more/pager_test.go#TestPagerOpenTTYFailureIsNotCopyFallback;cmds/more/tty_windows_test.go#TestWindowsControllingTerminalIsExplicitlyUnsupported;cmds/more/more_test.go#TestMoreAcceptsSupportedDisplayFlags;cmds/more/more_test.go#TestMoreEnvironmentMORE;cmds/more/more_test.go#TestMoreReadWriteErrors;cmds/more/more_test.go#TestNormalizeTerminalLinePOSIXOverstrikesAndPlainMode;cmds/more/posix_interactive_test.go#TestPOSIXCommandGrammarParsesCountsArgumentsAndControls;cmds/more/posix_interactive_test.go#TestPOSIXMovementMarksAndPositionCommands;cmds/more/posix_interactive_test.go#TestPOSIXSearchIgnoreCaseInvertRepeatAndReverse;cmds/more/posix_interactive_test.go#TestPOSIXTagLineAndPatternStart;cmds/more/posix_interactive_test.go#TestPOSIXEditorSelectionAndResumePosition;cmds/more/posix_interactive_test.go#TestPOSIXMoreLinesColumnsPrecedence;cmds/more/posix_interactive_test.go#TestPOSIXInitialCommandRunsForEveryNewFile`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:more:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [more](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/more.html).

## `mv`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mv [-if] source_file target_file
mv [-if] source_file... target_dir
```

**Issue 7 required-option candidate:** `-f; -i`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `source_file; target_file; target_dir`. A final operand naming an existing directory selects the target_dir form and each source_file moves to target_dir/basename; otherwise exactly two operands and source_file is renamed to target_file; each source is processed independently, with a per-operand failure or non-affirmative prompt reply skipping that source and continuing; the move is rename() equivalence first (an existing empty destination directory is replaced, a non-empty one is a diagnosed error), and on cross-device failure a copy+remove fallback that first unlinks/rmdirs the existing destination entry, refuses directory/non-directory type mismatches, duplicates the hierarchy with symbolic links copied as links, and removes the source only after successful duplication; a source and destination naming the same directory-entry identity are diagnosed and neither is removed (the diagnostic-and-status branch of the Issue 7 same-file alternatives); final symlinks are compared as links, so distinct symlinks to one referent remain distinct; the step-1 prompt precedes same-file resolution per the page's step order.

**Special tokens:** -- ends option parsing; -f and -i override each other by last occurrence, including within short clusters and exact long spellings; abbreviated long spellings are an extension whose textual arbitration is not guaranteed; a lone - is an ordinary pathname, not standard input.

**Standard input:** Used only to read one response line per overwrite prompt written to standard error (affirmative per the C-locale yesexpr plus the provisioned de_DE catalog; other locales are refused loudly); otherwise not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Overwrite prompts (under -i, or when an existing destination's permissions deny writing, standard input is a terminal, and -f is absent) and diagnostic messages, including non-fatal characteristic-preservation warnings.

**Effects:** `Renames each source to its destination path as by rename(); across filesystems removes the existing destination entry, duplicates the source hierarchy (symbolic links as links; last-data-modification and last-access times, user and group ID, and file mode duplicated, with S_ISUID/S_ISGID dropped when ownership cannot be duplicated), then removes the source hierarchy.`.

**Exit status:** 0 when all input files were moved successfully (a non-affirmative prompt response skips the source without error); greater than 0 if an error occurred; failure to duplicate file characteristics in the cross-device fallback is diagnosed but does not modify the exit status; usage errors exit 2 (documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mv`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mv`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/mv/mv_test.go#TestMvRenameFile;cmds/mv/mv_test.go#TestMvIntoDir;cmds/mv/mv_test.go#TestMvDirRename;cmds/mv/mv_test.go#TestMvDirectoryOntoExistingDirectoryInTarget;cmds/mv/mv_test.go#TestMvMultipleToNonDir;cmds/mv/mv_test.go#TestMvMissingSource;cmds/mv/mv_test.go#TestMvSameFile;cmds/mv/mv_test.go#TestMvPromptsBeforeSameFileResolution;cmds/mv/mv_test.go#TestMvLastOverwriteOptionWins;cmds/mv/mv_test.go#TestMvPromptUnwritable;cmds/mv/mv_test.go#TestMvInteractiveRefusal;cmds/mv/mv_test.go#TestMvInteractiveRefusalContinuesAndFails;cmds/mv/mv_test.go#TestMvInteractiveLeadingWhitespaceIsNotAffirmative;cmds/mv/mv_test.go#TestMvUnsupportedLCMessagesFailsClosed;cmds/mv/mv_test.go#TestMvCopyFallback;cmds/mv/mv_test.go#TestMvCopyFallbackFailures;cmds/mv/mv_test.go#TestMvEXDEVCharacteristicFailuresDiagnoseButStillMove;cmds/mv/mv_test.go#TestMvEXDEVDirectoryCharacteristicFailureDiagnosesButStillMoves;cmds/mv/mv_test.go#TestMvEXDEVSymlinkCharacteristicFailuresDiagnoseButStillMove;cmds/mv/mv_test.go#TestMvEXDEVReplacesEmptyDestinationDirectoryBeforeDuplication;cmds/mv/mv_test.go#TestMvEXDEVDoesNotRecursivelyRemoveNonemptyDestination;cmds/mv/mv_test.go#TestMvContinuesPastOperandErrors;cmds/mv/mv_test.go#TestMvSymlinkOperandMovesLinkItself;cmds/mv/mv_test.go#TestMvDistinctSymlinksToSameReferentAreNotSameFile;cmds/mv/mv_test.go#TestMvUpdateDoesNotBypassSameFileDiagnostic;cmds/mv/mv_test.go#TestMvUsageErrors;cmds/mv/mv_symlink_attrs_unix_test.go#TestMvEXDEVPreservesSymlinkAttributes`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:mv:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [mv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mv.html).

## `newgrp`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
newgrp [-l] [group]
```

**Issue 7 required-option candidate:** `-l`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `group`. No operand targets the user's primary group from the password database and rebuilds supplementary groups from the user/group databases; a group operand resolves by name before numeric gid; membership in the primary, supplementary, or group member list permits the change; non-members with a usable group password are challenged; refused or failed group changes diagnose and still start the shell unchanged; usage errors and an unreadable current user record start no shell.

**Special tokens:** -- ends option parsing; -l requests a login shell with argv0 prefixed by - and a login-style environment; any other option is a usage error.

**Standard input:** Not used; password input is read from the terminal device, not standard input.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used for diagnostics and for the terminal password prompt.

**Effects:** `Starts a child shell rather than replacing the embedding process; on Unix the child receives equal requested real/effective gids and the required supplementary-group plan when privilege permits, otherwise authorization/database/kernel refusal diagnoses and starts the shell with inherited credentials; no operand rebuilds the password-database primary and supplementary groups; non-login preserves cwd, environment, streams, and virtual umask, while -l supplies login argv0, home cwd, and the documented clean login environment; Windows fails loudly because POSIX group identity has no equivalent.`.

**Exit status:** If a shell is created, exits with that shell's status whether or not the group changed; otherwise exits greater than 0 for usage, database, platform, or shell-start errors.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/newgrp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/newgrp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/newgrp/newgrp_test.go#TestNoOperandRevertsToThePrimaryGroup;cmds/newgrp/newgrp_test.go#TestNoOperandDatabaseGroupFailureStillLaunchesUnchanged;cmds/newgrp/newgrp_test.go#TestNumericOperandPrefersTheGroupName;cmds/newgrp/newgrp_test.go#TestNumericOperandUsesTheNonNegativeGIDValue;cmds/newgrp/newgrp_test.go#TestSupplementaryGroupChangeRules;cmds/newgrp/newgrp_test.go#TestRefusedChangeStillStartsTheShellWithTheGroupUnchanged;cmds/newgrp/newgrp_test.go#TestKernelRefusalRetriesWithoutTheCredential;cmds/newgrp/newgrp_test.go#TestSuccessfulGroupChangePropagatesShellStatusThroughSpawnSeam;cmds/newgrp/newgrp_test.go#TestLoginShellArgv0AndDirectory;cmds/newgrp/newgrp_test.go#TestLoginEnvironment;cmds/newgrp/newgrp_test.go#TestLoginEnvironmentAndStatusSurviveKernelAssignmentFailure;cmds/newgrp/newgrp_test.go#TestUsageErrorsStartNoShell;cmds/newgrp/newgrp_test.go#TestAuthorize;cmds/newgrp/newgrp_test.go#TestPromptFailureIsDeniedNotAssumed;cmds/newgrp/spawn_unix_test.go#TestSyscallCredentialImplementsThePlan;cmds/newgrp/spawn_unix_test.go#TestSupplementaryCapacityFallbackRetainsMandatoryGIDPlan;cmds/newgrp/spawn_unix_test.go#TestSupplementaryCapacityFallbackIsNarrow;cmds/newgrp/spawn_unix_test.go#TestPasswordPromptUsesStderrAndReadsControllingTTY;cmds/newgrp/spawn_unix_test.go#TestDefaultSpawnShellPreservesArgumentsDirectoryEnvironmentAndIO;cmds/newgrp/spawn_unix_test.go#TestDefaultSpawnShellAppliesRunContextVirtualUmask;cmds/newgrp/spawn_unix_test.go#TestRunUmaskHelperRequiresCompleteControlAndPreservesExecInputs;cmds/newgrp/spawn_unix_test.go#TestRunUmaskHelperRejectsInvalidControlOrMaskWithoutExec;cmds/newgrp/spawn_unix_test.go#TestDefaultSpawnShellPropagatesSignalStatus;cmds/newgrp/spawn_windows_test.go#TestWindowsSpawnFailsLoudly`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:newgrp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [newgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/newgrp.html).

## `nice`

**Evidence state:** `implemented`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
nice [-n increment] utility [argument...]
```

**Issue 7 required-option candidate:** `-n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<increment>`.

**Operands:** `utility; argument`. The first non-option operand is the utility and all following arguments are passed through unchanged; -n requires a decimal integer increment and defaults to +10 when absent; when POSIXLY_CORRECT is present a utility operand is mandatory; GNU-compatible long and obsolete adjustment spellings remain available because POSIX does not require extensions to be rejected.

**Special tokens:** -- ends option parsing; a lone - is a utility operand; -n may be separate or attached; obsolete -NUM/--NUM/-+NUM and --adjustment are GNU-compatible extensions retained in every mode.

**Standard input:** Passed through unchanged to the invoked utility.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** Not used by nice itself; inherited by the invoked utility.

**Standard error:** Diagnostics only, including non-fatal niceness-adjustment warnings.

**Effects:** `Invokes the utility as the documented command-operand exception; on Unix a blocked pure-Go child helper cannot begin user code until the parent has attempted its absolute nice value, then overlays itself with the utility; adjustment failure emits a permitted warning but still invokes the utility unchanged, and the embedding host's priority is never modified; non-Unix invokes with a deterministic unsupported-adjustment warning.`.

**Exit status:** If the utility is invoked, exits with the utility's status, including 128+signal for signaled children while recording the raw signal in RunContext.ExitSignal for a standalone boundary; otherwise 125 for nice utility errors, 126 when found but not invokable, and 127 when the utility cannot be found.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/nice`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/nice`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/nice/nice_posix_test.go#TestParseNiceOptions;cmds/nice/nice_posix_test.go#TestNicePOSIXModeRequiresUtilityOperand;cmds/nice/nice_posix_test.go#TestNiceCommandExitStatuses;cmds/nice/nice_posix_test.go#TestResolveCommandResolvesPathEntriesFromRunContext;cmds/nice/nice_resolve_test.go#TestNicePathUnsetFindsCommand;cmds/nice/nice_resolve_test.go#TestNiceChildExitPropagates;cmds/nice/nice_resolve_test.go#TestNiceDoubleDashStopsOptionParsing;cmds/nice/nice_priority_test.go#TestNiceDoesNotAlterOwnPriority;cmds/nice/nice_priority_test.go#TestNiceReportsSignalExitCode;cmds/nice/nice_exec_unix_test.go#TestNiceRunsExecutableTextWithoutShebang;cmds/nice/nice_barrier_unix_test.go#TestPriorityHelperWaitsForBarrierBeforeExec;cmds/nice/nice_barrier_unix_test.go#TestNicePreservesEnvironmentThatCollidesWithFormerHelperMarker;cmds/nice/nice_barrier_unix_test.go#TestPriorityHelperPreservesENOEXECScriptFallback;cmds/nice/nice_barrier_unix_test.go#TestNiceUtilityStartsAtAdjustedPriority;cmds/nice/nice_barrier_unix_test.go#TestNicePassesUtilityArgumentsAndStandardStreamsUnchanged;cmds/nice/nice_barrier_unix_test.go#TestNiceAdjustmentFailureStillInvokesUtilityAndUtilityStatusWins;cmds/nice/nice_barrier_unix_test.go#TestPriorityHelperExecFailuresUsePOSIXStatuses`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:nice:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [nice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nice.html).

## `nm`

**Evidence state:** `missing`.

**Applicability:** `xsi; development`.

**Issue 7 synopsis candidate:**

```text
[development] nm [-APv] [-g|-u] [-t format] file...
[xsi] nm [-APv] [-efox] [-g|-u] [-t format] file...
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `development:-A,-g,-P,-t,-u,-v; xsi:-e,-f,-o,-x`.

**Issue 7 option-argument candidate:** `-t=<format>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#nm`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:nm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [nm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nm.html).

## `nohup`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
nohup utility [argument...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `utility; argument`. The first operand is the utility, PATH-searched when it has no separator and invoked with the remaining operands verbatim; no option parsing is performed apart from a sole --help/--version argument; a missing utility operand is an error with status 127.

**Special tokens:** No - or -- token is special; every word after the utility passes through unchanged.

**Standard input:** A terminal standard input is redirected to an unreadable /dev/null before command lookup; a non-terminal standard input passes through to the utility.

**Environment:** `HOME; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** A terminal (or absent) standard output is appended to nohup.out in the invocation directory, else $HOME/nohup.out, created 0600; otherwise the utility inherits it unchanged and nohup writes nothing to it.

**Standard error:** Diagnostics only, including the required appending-output notice; a terminal (or absent) standard error is redirected to the same destination as standard output.

**Effects:** `Invokes the utility (documented exec-wrapper exception) with SIGHUP ignored in the utility via a trap-and-exec shell wrapper on unix, while the nohup invocation itself also survives SIGHUP; may create or append nohup.out; the utility receives exactly the invocation environment.`.

**Exit status:** 126 when the utility was found but cannot be invoked; 127 for an error in nohup or a utility that could not be found, unconditionally; otherwise the exit status of the utility.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/nohup`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/nohup`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/nohup/nohup_test.go#TestNohupMissing;cmds/nohup/nohup_test.go#TestNohupInternalErrorStatusIsUnconditional;cmds/nohup/nohup_test.go#TestNohupRunsCommand;cmds/nohup/nohup_test.go#TestNohupFoundButNotExecutableReturns126;cmds/nohup/nohup_test.go#TestNohupNotFoundReturns127;cmds/nohup/nohup_test.go#TestNohupRedirectsTerminalEquivalentOutput;cmds/nohup/nohup_test.go#TestNohupRedirectsTerminalOutput;cmds/nohup/nohup_test.go#TestNohupFallsBackToHomeNohupOut;cmds/nohup/nohup_test.go#TestNohupRedirectsTerminalInput;cmds/nohup/nohup_test.go#TestNohupTerminalRedirectionDiagnostics;cmds/nohup/nohup_test.go#TestNohupDevNullOpenFailure;cmds/nohup/nohup_signal_unix_test.go#TestNohupIgnoresHangupForChild;cmds/nohup/nohup_signal_unix_test.go#TestNohupInvocationSurvivesHangup;cmds/nohup/nohup_signal_unix_test.go#TestNohupPreservesInvocationEnvironment`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:nohup:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [nohup](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html).

## `od`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
od [-v] [-A address_base] [-j skip] [-N count] [-t type_string]... [file...]
[xsi] od [-bcdosx] [file] [[+]offset[.][b]]
```

**Issue 7 required-option candidate:** `-A; -j; -N; -t; -v`.

**Issue 7 conditional-option candidate:** `xsi:-b,-c,-d,-o,-s,-x`.

**Issue 7 option-argument candidate:** `-A=<address_base>; -j=<skip>; -N=<count>; -t=<type_string>`.

**Operands:** `file; [+]offset[.][b]`. Read file operands in order as one logical input. With no more than two operands, no -A/-j/-N/-t/-v option, and a qualifying final numeric or plus-prefixed operand, XSI interprets the final operand as an offset.

**Special tokens:** A file operand of - selects standard input in this implementation. The XSI [+]offset[.][b] form is normally octal; a trailing . selects decimal and b scales by 512 bytes.

**Standard input:** Used when no file operand is present and for each file operand of -; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; xsi:NLSPATH`.

**Standard output:** Write the selected bytes in the requested formats, with the input offset at the start and after the final byte, suppressing duplicate groups unless -v is specified.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Reads the ordered input stream and emits a formatted dump; creates no output files.`.

**Exit status:** 0 if every input file is processed successfully; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/od`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/od`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/od/od_test.go#TestODConcatenatedFormatStrings;cmds/od/od_test.go#TestODMultipleFileConcatenation;cmds/od/od_test.go#TestODXSIOffsetGating;cmds/od/od_test.go#TestODSkipAndCountAcrossFiles;cmds/od/od_test.go#TestODCTypeSizesFollowTargetABI;cmds/od/od_test.go#TestODPartialItemAppendsNullBytes;cmds/od/od_test.go#TestODOutputAndCloseErrorsSetStatus;cmds/od/od_test.go#TestODMissingFileContinues;cmds/od/od_test.go#TestODCTypeLocaleRenderingAndPrecedence;cmds/od/od_test.go#TestODCTypeUTF8ContinuationAcrossOutputGroups;cmds/od/od_test.go#TestODUTF8FullASCIIGroupDoesNotWaitForLookahead;cmds/od/od_test.go#TestODLocaleCategoriesFailClosedOnlyWhenUsed;cmds/od/od_test.go#TestODNumericLocaleAllFloatingTypesAndABIs;cmds/od/od_test.go#TestODNumericLocalePrecedenceAndScientificRadix`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:od:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [od](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html).

## `paste`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
paste [-s] [-d list] file...
```

**Issue 7 required-option candidate:** `-d; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-d=<list>`.

**Operands:** `file`. One or more file operands (zero operands default to standard input as an extension); parallel mode concatenates corresponding lines replacing each newline except the last file's with the next delimiter and treats EOF on some files as empty lines; -s writes one line per file in operand order, an empty file yielding a bare newline; without -s an unopenable operand suppresses all standard output, with -s processing continues.

**Special tokens:** A file operand of - reads standard input one line at a time, circularly across the - instances; -d list elements cycle and reset per output line (per file with -s); \n, \t, \\ and \0 (empty string) are the delimiter escapes.

**Standard input:** Used only when a file operand is - (or, as an extension, when no operands are given).

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. -d LIST is split into delimiter characters per LC_CTYPE (LC_ALL > LC_CTYPE > LANG > POSIX default): C/POSIX and the carried de_DE.ISO-8859-1 alias use one byte per character, the carried C/POSIX UTF-8 aliases decode one rune per character, original bytes are preserved exactly, and an unsupported LC_CTYPE fails before any operand is opened.`.

**Standard output:** The concatenated lines separated by tabs or the -d delimiter elements, each output line newline-terminated.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Reads inputs without modifying them; no output files.`.

**Exit status:** 0 successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/paste`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/paste`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/paste/paste_test.go#TestPasteParallel;cmds/paste/paste_test.go#TestPasteSerial;cmds/paste/paste_test.go#TestPasteStdin;cmds/paste/paste_test.go#TestPasteErrors;cmds/paste/paste_test.go#TestPasteUnknownFlag;cmds/paste/paste_test.go#TestPasteHelpAndVersion;cmds/paste/issue738_posix_test.go#TestIssue738LocaleAwareDelimiterDecoding;cmds/paste/issue738_posix_test.go#TestIssue738LCCTypePrecedence;cmds/paste/issue738_posix_test.go#TestIssue738UnsupportedLocaleFailsBeforeOpeningOperand;cmds/paste/issue738_posix_test.go#TestIssue738RepeatedDashUnderSerial;cmds/paste/issue738_posix_test.go#TestIssue738TwelveOperands;cmds/paste/issue738_posix_test.go#TestIssue738BackslashEscapedDelimiter;cmds/paste/issue738_posix_test.go#TestIssue738SerialZeroDelimiter;cmds/paste/issue738_posix_test.go#TestIssue738InjectedReadErrorParallel;cmds/paste/issue738_posix_test.go#TestIssue738InjectedReadErrorSerial;cmds/paste/issue738_posix_test.go#TestIssue738OutputWriteErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:paste:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [paste](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/paste.html).

## `patch`

**Evidence state:** `missing`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
patch [-blNR] [-c|-e|-n|-u] [-d dir] [-D define] [-i patchfile] [-o outfile] [-p num] [-r rejectfile] [file]
```

**Issue 7 required-option candidate:** `-b; -c; -d; -D; -e; -i; -l; -n; -N; -o; -p; -R; -r; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-d=<dir>; -D=<define>; -i=<patchfile>; -o=<outfile>; -p=<num>; -r=<rejectfile>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; LC_TIME`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#patch`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:patch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [patch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/patch.html).

## `pathchk`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
pathchk [-p] [-P] pathname...
```

**Issue 7 required-option candidate:** `-p; -P`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `pathname`. One or more pathname operands are checked independently; the exit status aggregates all operand failures; missing future path components are not errors when they could be created without violating the selected checks.

**Special tokens:** -p replaces filesystem checks with POSIX portable limits and portable filename character-set checks; -P adds empty-name and leading-hyphen checks to the default filesystem checks; -- terminates option parsing.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostics; diagnostics identify the failing operand or component and the violated check.

**Effects:** `No filesystem effects; existing directory prefixes may be inspected for directory and searchability checks.`.

**Exit status:** 0 when every operand passes all selected checks; greater than 0 when any operand fails; usage errors return 2 in this implementation.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/pathchk`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/pathchk`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/pathchk/pathchk_test.go#TestPathchkPosixPathLimitIncludesTerminator;cmds/pathchk/pathchk_test.go#TestPathchkPosixNameLimitAndPortableCharacters;cmds/pathchk/pathchk_test.go#TestPathchkPosixDoesNotRejectLeadingHyphen;cmds/pathchk/pathchk_test.go#TestPathchkMultipleOperandsAggregateStatus;cmds/pathchk/pathchk_test.go#TestPathchkMissingOperandUsage;cmds/pathchk/pathchk_test.go#TestPathchkAllowsMissingDirectoryPrefix;cmds/pathchk/pathchk_test.go#TestPathchkRejectsNonDirectoryPrefix;cmds/pathchk/pathchk_unix_test.go#TestPathchkRejectsUnsearchableDirectoryPrefix;cmds/pathchk/pathchk_unix_test.go#TestPathchkRejectsDanglingSymlinkPrefix;cmds/pathchk/pathchk_test.go#TestPathchkDefaultPathLimitIncludesTerminator;cmds/pathchk/pathchk_test.go#TestPathchkDefaultUsesContainingDirectoryNameLimit;cmds/pathchk/pathchk_test.go#TestPathchkDefaultPreservesResolutionSyntax;cmds/pathchk/pathchk_unix_test.go#TestPathchkPreservesSymlinkBeforeDotDotResolution;cmds/pathchk/issue741_posix_test.go#TestIssue741LimitsQueryErrorIsDiagnosed;cmds/pathchk/issue741_posix_test.go#TestIssue741IndeterminateLimitsSkipLengthChecks;cmds/pathchk/issue741_posix_test.go#TestIssue741DeepContainingDirectoryQueryError;cmds/pathchk/issue741_posix_test.go#TestIssue741DeepContainingDirectoryIndeterminateNameLimit;cmds/pathchk/issue741_posix_test.go#TestIssue741LimitsFailuresAggregateAcrossOperands;cmds/pathchk/issue741_linux_test.go#TestIssue741LinuxAcceptsFilesystemValidNonUTF8Name`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:pathchk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [pathchk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html).

## `pax`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
pax [-dv] [-c|-n] [-H|-L] [-o options] [-f archive] [-s replstr]... [pattern...]
pax -r[-c|-n] [-dikuv] [-H|-L] [-f archive] [-o options]... [-p string]... [-s replstr]... [pattern...]
pax -w [-dituvX] [-H|-L] [-b blocksize] [[-a] [-f archive]] [-o options]... [-s replstr]... [-x format] [file...]
pax -r -w [-diklntuvX] [-H|-L] [-o options]... [-p string]... [-s replstr]... [file...] directory
```

**Issue 7 required-option candidate:** `-r; -w; -a; -b; -c; -d; -f; -H; -i; -k; -l; -L; -n; -o; -p; -s; -t; -u; -v; -x; -X`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-b=<blocksize>; -f=<archive>; -o=<options>; -p=<string>; -s=<replstr>; -x=<format>`.

**Operands:** `directory; file; pattern`. directory is the copy-mode destination; file names a source to copy or archive; pattern uses pathname pattern-matching notation to select archive members, and an omitted pattern selects all members.

**Special tokens:** In write mode with no file operands, newline-terminated pathnames are read from standard input. Repeated -s and -o/-p option-arguments are applied in command-line order as specified by their keyword grammars.

**Standard input:** In write mode, used only when no file operand is present and then contains newline-terminated pathnames. In list/read mode, supplies the archive when -f is absent. Otherwise not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TMPDIR; TZ`.

**Standard output:** In write mode without -f, write the archive. In list mode, write selected member pathnames or the specified list format; -v uses ls -l-shaped listings and identifies hard links.

**Standard error:** Write -v processed pathnames, trailing-p substitution reports, prompts, conversion failures, and diagnostics; required per-path records are flushed as processing proceeds.

**Effects:** `Reads, lists, creates, appends, or updates an archive; read mode extracts members; copy mode creates a destination hierarchy, subject to selection, replacement, and preservation rules.`.

**Exit status:** 0 if all files are processed successfully; greater than 0 if an error occurs, with specified per-file failures diagnosed while processing continues.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/pax`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/pax`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/pax/pax_test.go#TestWriteThenListThenExtractRoundTrips;cmds/pax/archive_lane_test.go#TestBlockSizeGrammar;cmds/pax/archive_lane_test.go#TestWriteWithoutOperandsReadsStdinAndContinuesAfterPathFailure;cmds/pax/follow_test.go#TestDashHFollowsCommandLineSymlinksOnly;cmds/pax/wave_test.go#TestModeOptionLegality;cmds/pax/list_io_test.go#TestPaxListModePropagatesOutputErrors;cmds/pax/issue715_test.go#TestInteractiveReadPreflightsAndRenamesHardlinks;cmds/pax/issue715_test.go#TestInteractiveFailuresAreImmediateAndNonzero;cmds/pax/issue715_test.go#TestCopyLinkRegularAndSymlinkFollowModes;cmds/pax/issue715_test.go#TestResetAccessTimesWriteAndCopyAndFailureStatus;cmds/pax/issue716_test.go#TestPreservationGrammarOrderedAndRepeated;cmds/pax/issue716_test.go#TestExtractDefaultCreationAndPreservedMode;cmds/pax/issue716_test.go#TestPreservationFailuresKeepFilesClearSetIDAndContinue;cmds/pax/issue716_test.go#TestSymlinkPreservationUsesNoFollowOwnerAndTimes;cmds/pax/issue717_test.go#TestPAXOptionGrammarWhitespaceEscapedCommaAndTrailingComma;cmds/pax/issue717_test.go#TestPAXOptionRepeatedPrecedenceDeleteAdditiveAndListConcatenation;cmds/pax/issue717_test.go#TestPAXOptionModeAndFormatApplicabilityFailClosed;cmds/pax/issue717_test.go#TestPAXReadAndListHeaderPrecedence;cmds/pax/issue717_test.go#TestPAXInvalidActionsBypassAndUTF8;cmds/pax/issue717_test.go#TestPAXListFormatConversionsAndConcatenation;cmds/pax/issue717_test.go#TestPAXCarriedCodesetByteTranscoding;cmds/pax/issue717_test.go#TestPAXListTimeCompleteIssue7DateConversions;cmds/pax/issue717_test.go#TestPAXMalformedListFormatFailsClosed`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:pax:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html).

## `pr`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
pr [+page] [-column] [-adFmrt] [-e[char][gap]] [-h header] [-i[char][gap]] [-l lines] [-n[char][width]] [-o offset] [-s[char]] [-w width] [-p] [file...]
[xsi] pr [+page] [-column] [-adFmrt] [-e[char][gap]] [-h header] [-i[char][gap]] [-l lines] [-n[char][width]] [-o offset] [-s[char]] [-w width] [-fp] [file...]
```

**Issue 7 required-option candidate:** `+<page>; -<column>; -a; -d; -e; -F; -h; -i; -l; -m; -n; -o; -p; -r; -s; -t; -w`.

**Issue 7 conditional-option candidate:** `xsi:-f`.

**Issue 7 option-argument candidate:** `-e[char][gap]; -h=<header>; -i[char][gap]; -l=<lines>; -n[char][width]; -o=<offset>; -s[char]; -w=<width>`.

**Operands:** `file`. Read file operands in order; a missing file operand or a file operand of - selects standard input. +page selects the first output page and -column selects the column count.

**Special tokens:** A file operand of - selects standard input. pr is exempt from several Utility Syntax Guidelines; this interface makes no -- delimiter claim.

**Standard input:** Read as a text file when no file operand is given or when a file operand is -; /dev/tty supplies -p responses.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TZ`.

**Standard output:** Write paginated input, including headers and trailers unless -t suppresses them; formatting is controlled by the declared page, column, tab, numbering, width, and merge options.

**Standard error:** Used for diagnostics and to alert the terminal when -p is specified.

**Effects:** `stdout_and_terminal_prompt`.

**Exit status:** 0 for successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/pr`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/pr`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/pr/pr_test.go#TestPRDefaultPageStructure;cmds/pr/pr_test.go#TestPRHeaderHonorsInvocationTZAndLCTime;cmds/pr/pr_test.go#TestPRNumberIndentAndDoubleSpace;cmds/pr/pr_test.go#TestPRPagesRangeAndDateFormat;cmds/pr/pr_test.go#TestPRPausePerPageWritesAlertAndReadsDevTTY;cmds/pr/pr_test.go#TestPRPauseInterrupted;cmds/pr/pr_test.go#TestPRVerticalColumns;cmds/pr/pr_test.go#TestPRMerge;cmds/pr/pr_test.go#TestPRMergeAssumesPOSIXTabExpansionAndReplacement;cmds/pr/pr_test.go#TestPROptionalExpandArgument;cmds/pr/pr_test.go#TestPRTerminalDefersFileDiagnosticsUntilOutputCompletes;cmds/pr/pr_test.go#TestPRStreamReadAndShortWriteFailuresAreNonzero;cmds/pr/pr_test.go#TestPRPOSIXLYCorrectDifferentials`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:pr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [pr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pr.html).

## `printf`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
printf format [argument...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `format; argument`. A format operand is required. Interpret its ordinary characters, backslash escapes, and conversion specifications; consume arguments in order, reuse the format until all arguments are consumed, and supply zero or empty-string values for missing arguments as required by the conversion.

**Special tokens:** In the format, \\ddd is an octet escape; in a %b argument, \\0ddd is an octet escape and \\c stops all further output. %% writes a literal percent. A leading single or double quote on a numeric operand selects the value of the following character. Empty %c output is POSIX-unspecified; Bashy's NUL choice is retained as Bash compatibility, not POSIX evidence.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; xsi:NLSPATH`.

**Standard output:** Write the expanded format, reusing it as necessary, with no implicit newline beyond one present in the format.

**Standard error:** Used for a missing format, invalid conversion or numeric operand, and standard-output write failure.

**Effects:** `No files or shell state are modified; only standard output is written.`.

**Exit status:** 0 when formatting and output succeed; greater than 0 for an operand, conversion, or output error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:printf`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestPrintfIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRoutePrintf`; provider=`-`; clauses=`XCU:printf:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [printf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/printf.html).

## `ps`

**Evidence state:** `partial`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] ps [-aA] [-defl] [-g grouplist] [-G grouplist] [-n namelist] [-o format]... [-p proclist] [-t termlist] [-u userlist] [-U userlist]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-a,-A,-d,-e,-f,-g,-G,-l,-n,-o,-p,-t,-u,-U`.

**Issue 7 option-argument candidate:** `-g=<grouplist>; -G=<grouplist>; -n=<namelist>; -o=<format>; -p=<proclist>; -t=<termlist>; -u=<userlist>; -U=<userlist>`.

**Operands:** `none`. No operands are accepted. Selection options are combined by inclusive OR and suppress the default same-effective-user/same-terminal selection; -f, -l, -n, and -o do not alter selection. Empty mandatory option-arguments are diagnosed.

**Special tokens:** The XSI terminal selector accepts both a tty device filename and the identifier after its tty prefix. -n is accepted as an alternate namelist designation, but the Linux procfs provider does not consult a kernel namelist. -x is not an Issue 7 option and is rejected.

**Standard input:** Not used.

**Environment:** `COLUMNS; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TZ`.

**Standard output:** Write information about selected processes. COLUMNS overrides a live output-terminal width. Without -o the base format is unspecified; XSI default, full, and long layouts and -o fields use the required headings, ordering, values, user-supplied header overrides, textual identity lookup with decimal fallback, and hyphens for unavailable optional data.

**Standard error:** Used only for usage, locale, process-provider, and output-error diagnostic messages.

**Effects:** `On Linux, reads a live procfs snapshot including stat field 22 start time, AT_CLKTCK, boot time, status identities, terminal number, CPU, memory, and wait-channel data; disappearing or unavailable data remains absent rather than fabricated. Other platforms fail explicitly because exact selection data is unavailable. No process is modified.`.

**Exit status:** 0 on successful completion; greater than 0 for syntax, locale, process-provider, or output errors (usage errors use 2).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/ps`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/ps`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/ps/ps_test.go#TestPSAllPOSIXSelectionOptionsEndToEnd;cmds/ps/ps_test.go#TestPSXIsNotAnIssue7Option;cmds/ps/ps_test.go#TestPSPOSIXListSeparatorsAndFormatHeaders;cmds/ps/ps_test.go#TestPSRejectsEmptyMandatoryOptionArgumentsAndOperands;cmds/ps/ps_test.go#TestPSIdentityLookupAndNumericFallback;cmds/ps/ps_test.go#TestPSHermeticEnumeratorSelectionOrderingAndFields;cmds/ps/ps_test.go#TestPSXSIStandardDefaultLayouts;cmds/ps/ps_test.go#TestPSPOSIXRequiredFormatNamesAndHeaders;cmds/ps/ps_test.go#TestPSTerminalFormatMatchesWhoAndWchanDistinguishesRunning;cmds/ps/ps_test.go#TestPSEnvironmentIsInvocationLocal;cmds/ps/ps_test.go#TestPSEnrichLinuxProcFixture;cmds/ps/ps_test.go#TestPSLinuxTimingUnavailableDoesNotInventValues;cmds/ps/process_linux_test.go#TestPSTTYNameDecodesLinuxDevptsMajorRange;cmds/ps/process_linux_test.go#TestPSEnrichLinuxCmdlinePreservesArgvZeroAndEmptyArguments;cmds/ps/process_linux_test.go#TestPSLinuxTimingRejectsOverflowAndEmptyWchan;cmds/ps/ps_test.go#TestPSLiveLinuxEnumeratorOwnProcess;cmds/ps/ps_sigttin_unix_test.go#TestPSUsesLiveOutputTerminalWidthWhenColumnsUnset;cmds/ps/ps_sigttin_unix_test.go#TestPSRetainsDefaultSIGTTINDisposition;cmds/ps/ps_nonlinux_test.go#TestPSNonLinuxLiveSourceFailsExplicitly`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:ps:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [ps](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ps.html).

## `pwd`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
pwd [-L|-P]
```

**Issue 7 required-option candidate:** `-L; -P`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No operands are specified. -L writes the logical pathname from PWD when it is an absolute pathname without dot or dot-dot components naming the current directory; otherwise it falls back to the physical pathname. -P resolves symbolic links physically, with the last -L/-P option winning.

**Special tokens:** Bash accepts and ignores extra operands as an extension outside the Issue 7 synopsis.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_MESSAGES; xsi:NLSPATH; PWD`.

**Standard output:** Write one absolute pathname of the current working directory followed by a newline; a standard-output failure is diagnosed and returns failure.

**Standard error:** Used for invalid-option, pathname-resolution, and standard-output failure diagnostics.

**Effects:** `Does not change the current working directory. Bash updates PWD to the physical pathname after a successful -P query; this is an implementation extension.`.

**Exit status:** 0 when the pathname is determined and written; greater than 0 on error (usage errors use status 2).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:pwd`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestPwdIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRoutePwd`; provider=`-`; clauses=`XCU:pwd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [pwd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pwd.html).

## `read`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
read [-r] var...
```

**Issue 7 required-option candidate:** `-r`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `var`. One or more valid variable-name operands receive fields from one logical input line. IFS controls field splitting; excess variables become empty and the last variable receives the remaining fields and separators according to Issue 7.

**Special tokens:** Without -r, an unescaped backslash escapes the following character and backslash-newline continues the logical line; -r makes backslashes literal. Omitting every var and assigning REPLY is a Bash extension, not POSIX evidence.

**Standard input:** Read one logical line from standard input.

**Environment:** `IFS; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PS2`.

**Standard output:** Not used by read itself.

**Standard error:** Used for option, variable-name, assignment, and input diagnostics.

**Effects:** `Assign the requested variables in the current shell execution environment; no files are modified.`.

**Exit status:** 0 when a complete logical line is read and assigned; greater than 0 on end-of-file or error. Behavior for a non-text unterminated final line is retained as Bash compatibility outside the POSIX input domain.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:read`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestReadIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteRead`; provider=`-`; clauses=`XCU:read:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [read](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/read.html).

## `renice`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
renice [-g|-p|-u] -n increment ID...
```

**Issue 7 required-option candidate:** `-g; -n; -p; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<increment>`.

**Operands:** `ID`. Each ID is interpreted under the selector in effect at its position: process ID by default or after -p, process-group ID after -g, and user name or numeric user ID after -u. Selectors may be interspersed and affect following IDs; the single -n increment may also be interspersed and applies invocation-wide. At least one ID and exactly one -n are required; failures are diagnosed while later IDs continue.

**Special tokens:** Issue 7 requires -n; the removed obsolescent first-operand form is refused loudly. -- ends option parsing. A -u operand is looked up as a user name first and only an exact unknown-user result permits numeric UID fallback. ID 0 process/group semantics are supported only for a dedicated command process and fail closed in an in-process embedding.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `For each selected process, adds increment to that process's current nice value, subject to the host's implementation limits and permissions. On Linux, process-group and saved-set-user-ID selectors are expanded through a /proc snapshot and adjusted per process; other Unix platforms fail those collective selectors explicitly when exact membership enumeration is unavailable.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`manual`).

**Implementation:** `cmds/renice`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/renice`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/renice/renice_test.go#TestMissingIncrementOptionIsUsageError;cmds/renice/renice_test.go#TestObsolescentFirstOperandFormIsRefusedLoudly;cmds/renice/renice_test.go#TestIncrementForms;cmds/renice/renice_test.go#TestDuplicateIncrementIsRefusedAndLateIncrementIsAccepted;cmds/renice/renice_test.go#TestPositionalSelectorSwitchingDispatchesExactWhich;cmds/renice/renice_test.go#TestRetainedLongOptionsAndHelpVersionAliases;cmds/renice/renice_test.go#TestOrderedMixedSuccessAndFailureContinues;cmds/renice/renice_test.go#TestSuccessIsSilentOnStdout;cmds/renice/renice_test.go#TestIncrementIsRelativeAndBoundsAreSchedulerClamped;cmds/renice/renice_test.go#TestUserOperandResolution;cmds/renice/renice_test.go#TestZeroProcessIDFailsClosedInEmbeddingAndPassesInDedicatedProcess;cmds/renice/renice_test.go#TestCollectiveIncrementIsPerProcessAndContinuesWithinSelector;cmds/renice/renice_test.go#TestFullWidthNumericUIDWhereHostIntPermits;cmds/renice/prio_linux_test.go#TestLinuxMembersUsesPGroupAndSavedUID;cmds/renice/prio_unix_test.go#TestRealPositiveIncrementOnOwnedChildAndGroup`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:renice:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [renice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html).

## `rm`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
rm [-iRr] file...
rm -f [-iRr] [file...]
```

**Issue 7 required-option candidate:** `-f; -i; -R; -r`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Each file operand is processed independently in order, with failures diagnosed and processing continuing; an operand whose basename portion is dot or dot-dot, or one that resolves to the root directory where a removal would be attempted (-r/-R and the -d extension), is refused before any prompt or filesystem operation; a nonexistent operand is a diagnosed error unless -f; a directory requires -r/-R (diagnosed Is-a-directory otherwise) and is descended depth-first excluding dot and dot-dot without following symbolic links, each directory removed after its entries; a symbolic link operand is unlinked, never followed; with -f the file operand list may be empty.

**Special tokens:** -- ends option parsing; -R is equivalent to -r; -f and -i override each other by last occurrence including within short-option clusters; a lone - is an ordinary pathname.

**Standard input:** Used only to read one response line per prompt (before each non-directory, before descending a directory whose permissions deny writing when standard input is a terminal or under -i, and again after a directory is emptied under -i; affirmative per the C-locale yesexpr plus the provisioned de_DE catalog, other locales refused loudly); otherwise not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Prompts (-i before each entry and again for each directory after its entries; write-protected non-symlink entries when standard input is a terminal and -f is absent) and diagnostic messages.

**Effects:** `Removes each named directory entry as by unlink()/rmdir(); -r removes hierarchies depth-first, removing each directory after its entries; a non-affirmative response leaves the entry (and, pre-descent, the whole hierarchy below it) untouched.`.

**Exit status:** 0 when each named directory entry was removed or its removal was canceled by a non-affirmative prompt response, and under -f also with no operands or nonexistent operands (no diagnostic, no status effect); greater than 0 if an error occurred; usage errors exit 2 (documented repo deviation).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/rm`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/rm`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/rm/rm_test.go#TestRmFile;cmds/rm/rm_test.go#TestRmMissing;cmds/rm/rm_test.go#TestRmOperandErrors;cmds/rm/rm_test.go#TestRmDirWithoutR;cmds/rm/rm_test.go#TestRmRecursive;cmds/rm/rm_test.go#TestRmCapitalRAlias;cmds/rm/rm_test.go#TestRmInteractivePrompt;cmds/rm/rm_test.go#TestRmRecursiveInteractivePrompts;cmds/rm/rm_test.go#TestRmRecursiveInteractiveDeclinedBranch;cmds/rm/rm_test.go#TestRmImplicitPromptForUnwritable;cmds/rm/rm_test.go#TestRmImplicitDirectoryPromptPrecedesDescent;cmds/rm/rm_test.go#TestRmImplicitPromptSkipsSymlinkOperands;cmds/rm/rm_test.go#TestRmLastPromptOptionWins;cmds/rm/rm_test.go#TestRmRejectsDotAndDotDotOperands;cmds/rm/rm_test.go#TestRmRejectsDotComponentsBeforeTraversal;cmds/rm/rm_test.go#TestRmAllowsDotComponentsBeforeFinalName;cmds/rm/rm_test.go#TestRmRootRefused;cmds/rm/rm_test.go#TestRmPreserveRootFinalSymlinkPolicy;cmds/rm/rm_test.go#TestRmPreserveRootGuardsDashDRemoval;cmds/rm/rm_test.go#TestRmContinuesPastErrors;cmds/rm/rm_test.go#TestRmInteractiveLeadingWhitespaceIsNotAffirmative;cmds/rm/rm_test.go#TestRmInteractiveUsesGermanYesexpr;cmds/rm/rm_test.go#TestRmInteractiveRejectsUnsupportedLCMessages`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:rm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [rm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rm.html).

## `rmdir`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
rmdir [-p] dir...
```

**Issue 7 required-option candidate:** `-p`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `dir`. Process each dir pathname in operand order and remove its empty directory entry as by rmdir(); under -p, after removing a multi-component operand, repeatedly remove its dirname ancestors until completion or the first failure. A final pathname component of dot is refused with Invalid argument before any filesystem call. A final component of dot-dot is passed to the host pathname walk without lexical cleaning; the operation must fail, while Issue 7 leaves its errno unspecified, so invalid prefixes retain their native missing-component or non-directory errors.

**Special tokens:** -- ends option parsing; a lone - after it is a literal directory pathname, not standard input.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Removes each named empty directory and, with -p, removable pathname ancestors.`.

**Exit status:** 0 only when every directory entry specified by an operand is removed successfully; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/rmdir`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/rmdir`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/rmdir/rmdir_test.go#TestRmdirEmpty;cmds/rmdir/rmdir_test.go#TestRmdirNonEmpty;cmds/rmdir/rmdir_test.go#TestRmdirIgnoreFailOnNonEmpty;cmds/rmdir/rmdir_test.go#TestRmdirParents;cmds/rmdir/rmdir_test.go#TestRmdirParentsStopsOnNonEmpty;cmds/rmdir/rmdir_test.go#TestRmdirParentsWithTrailingSlash;cmds/rmdir/rmdir_test.go#TestRmdirOperandOrderMatters;cmds/rmdir/rmdir_test.go#TestRmdirContinuesPastErrors;cmds/rmdir/rmdir_test.go#TestRmdirDotBare;cmds/rmdir/rmdir_test.go#TestRmdirDotDotBareFailsNaturally;cmds/rmdir/rmdir_test.go#TestRmdirRealDirectoryDotDotMayBeIgnored;cmds/rmdir/rmdir_test.go#TestRmdirMissingPrefixDotDotIsNotCleaned;cmds/rmdir/rmdir_test.go#TestRmdirNonDirectoryPrefixDotDotIsNotIgnored;cmds/rmdir/rmdir_test.go#TestRmdirTrailingDotComponent;cmds/rmdir/rmdir_test.go#TestRmdirTrailingDotDotComponent;cmds/rmdir/rmdir_test.go#TestRmdirTrailingSlash;cmds/rmdir/rmdir_test.go#TestRmdirIgnoreNonEmptyWithParents;cmds/rmdir/rmdir_test.go#TestRmdirIgnoreNonEmptyDoesNotIgnoreOtherErrors;cmds/rmdir/rmdir_test.go#TestRmdirDoubleDashOperand;cmds/rmdir/rmdir_test.go#TestRmdirDashOperand;cmds/rmdir/rmdir_test.go#TestRmdirDoesNotConsumeStdin;cmds/rmdir/rmdir_test.go#TestRmdirDiagnosticWriteFailureStillFails;cmds/rmdir/rmdir_unix_test.go#TestRmdirPermissionDeniedContinuesAfterOperand`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:rmdir:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [rmdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rmdir.html).

## `sed`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
sed [-n] script [file...]
sed [-n] -e script [-e script]... [-f script_file]... [file...]
sed [-n] [-e script]... -f script_file [-f script_file]... [file...]
```

**Issue 7 required-option candidate:** `-e; -f; -n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-e=<script>; -f=<script_file>`.

**Operands:** `file; script`. file operands are read in order and edited as one concatenated input. With no file operands, use standard input. script is an editing-command string whose final newline may be omitted.

**Special tokens:** A file operand of - selects standard input in this implementation. When -e or -f is absent, the first non-option operand is the script; otherwise all remaining operands are files.

**Standard input:** Used when no file operand is present and for a file operand of -; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write input after applying the editing script; with -n, write only lines explicitly selected for output by the script.

**Standard error:** Used only for diagnostic and warning messages.

**Effects:** `Applies the Issue 7 sed command language to one logical input and may create or truncate files named by w commands.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/sed`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/sed`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/sed/sed_test.go#TestSedPreservesMixedExpressionFileOrder;cmds/sed/sed_test.go#TestSedPreservesMissingFinalNewline;cmds/sed/sed_test.go#TestSedBREGroupsAndBackrefs;cmds/sed/sed_test.go#TestSedBREInterval;cmds/sed/sed_test.go#TestSedDeleteAndRange;cmds/sed/sed_test.go#TestSedPreparesWriteFilesBeforeProcessing;cmds/sed/sed_test.go#TestSedPOSIXRegexConformance;cmds/sed/sed_test.go#TestSedExitCodeSeverityTiers`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:sed:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html).

## `sh`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
sh [-abCefhimnuvx] [-o option]... [+abCefhimnuvx] [+o option]... [command_file [argument...]]
sh -c [-abCefhimnuvx] [-o option]... [+abCefhimnuvx] [+o option]... command_string [command_name [argument...]]
sh -s [-abCefhimnuvx] [-o option]... [+abCefhimnuvx] [+o option]... [argument...]
```

**Issue 7 required-option candidate:** `-a; -b; -C; -e; -f; -h; -i; -m; -n; -u; -v; -x; -o; +a; +b; +C; +e; +f; +h; +i; +m; +n; +u; +v; +x; +o; -c; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-o=<option>; +o=<option>`.

**Operands:** `-; argument; command_file; command_name; command_string`. command_file names a script and sets $0; -c interprets command_string, sets $0 from command_name when supplied, and assigns remaining arguments to positional parameters; -s or absence of -c and operands reads commands from standard input.

**Special tokens:** A lone - as the first operand is ignored. -- ends option parsing. +letter and +o disable the corresponding option; an empty command_string exits successfully.

**Standard input:** Used with -s, when neither -c nor operands are present, and by executed commands that inherit it. The shell must not read ahead past commands and must enable blocking reads on FIFO or terminal standard input.

**Environment:** `ENV; FCEDIT; HISTFILE; HISTSIZE; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; MAIL; MAILCHECK; MAILPATH; xsi:NLSPATH; PATH; PWD`.

**Standard output:** Used by executed shell commands and by shell constructs whose specifications require standard output.

**Standard error:** Except where an invoked utility or interactive behavior specifies otherwise, used only for diagnostic messages.

**Effects:** `Parses and executes the Issue 7 shell command language, setting positional parameters, variables, options, traps, jobs, redirections, and current-shell state as specified.`.

**Exit status:** 0 for an empty/comment-only script; 1-125 for specified non-interactive shell errors; 126 for an ENOEXEC command_file; 127 when command_file is not found; otherwise the status of the last command invoked or attempted.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_entrypoint`).

**Implementation:** `shell:sh`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/interp_test.go#TestRunnerPosixStdinArgv0;sh:interp/startup_env_test.go#TestPosixStartupExportAttributes;sh:interp/strictposix_test.go#TestStrictPosixPropagation`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteSh;bashy:internal/cli/main_test.go#TestStrictPosixEngagedByArgv0Sh;bashy:internal/cli/profile_b_sh_entrypoint_unix_test.go#TestProfileBShUtilityEntrypointContract`; provider=`-`; clauses=`XCU:sh:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [sh](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sh.html).

## `sleep`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
sleep time
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `time`. Accept the Issue 7 time operand as a non-negative decimal integer and suspend execution for at least that many integral seconds.

**Special tokens:** -- ends option parsing; Issue 7 defines no command-specific special token.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Suspends the invoking execution for at least time seconds; SIGALRM may complete successfully, be ignored, or take its default action, and other signals take their standard action.`.

**Exit status:** 0 after a successful suspension of at least time seconds or an allowed successful SIGALRM disposition; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/sleep`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/sleep`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/sleep/sleep_test.go#TestSleepZeroish;cmds/sleep/sleep_test.go#TestSleepIssue7IntegralDuration;cmds/sleep/sleep_test.go#TestSleepErrors;cmds/sleep/sleep_test.go#TestSleepEndOfOptions;cmds/sleep/sleep_test.go#TestSleepDoesNotConsumeStdin;cmds/sleep/sleep_test.go#TestSleepCancel;cmds/sleep/sleep_test.go#TestSleepSuffixMath;cmds/sleep/sleep_signal_unix_test.go#TestSleepSIGALRMPermittedDisposition;cmds/sleep/sleep_signal_unix_test.go#TestSleepSIGTERMStandardAction`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:sleep:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [sleep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sleep.html).

## `sort`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
sort [-m] [-o output] [-bdfinru] [-t char] [-k keydef]... [file...]
sort [-c|-C] [-bdfinru] [-t char] [-k keydef] [file]
```

**Issue 7 required-option candidate:** `-c; -C; -m; -o; -u; -d; -f; -i; -n; -r; -b; -t; -k`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-o=<output>; -t=<char>; -k=<keydef>`.

**Operands:** `file`. Read each file as input to sort, merge, or check. With no file operands or for each file operand of -, use standard input. An open/read error may terminate without output or later operand processing.

**Special tokens:** A file operand of - selects standard input. Field/key positions, modifiers, and the -t separator follow the Issue 7 key-definition grammar.

**Standard input:** Used only when no file operand is present or when a file operand is -.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; xsi:NLSPATH`.

**Standard output:** Unless -o or -c is in effect, write the sorted input. Merge and uniqueness affect the result; check mode writes no sorted result.

**Standard error:** Used for diagnostics; -c disorder or duplicate-key detection identifies the offending input line.

**Effects:** `Sorts, merges, or checks input records; -o writes the result to output, including when output is also an input pathname.`.

**Exit status:** 0 when every input is ordered under -c/-C or when sorting succeeds; greater than 0 for disorder under -c/-C or any error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/sort`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/sort`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/sort/sort_test.go#TestOpenOperandsValidatesAllBeforeReading;cmds/sort/sort_test.go#TestSortBasic;cmds/sort/sort_test.go#TestSortKeys;cmds/sort/sort_test.go#TestSortCheck;cmds/sort/sort_test.go#TestSortFilesAndOutput;cmds/sort/sort_test.go#TestSortMerge;cmds/sort/sort_test.go#TestSortLCNumeric;cmds/sort/collator_test.go#TestRunWithCollatorOrdersTextualComparisons`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:sort:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [sort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sort.html).

## `split`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
split [-l line_count] [-a suffix_length] [file [name]]
split -b n[k|m] [-a suffix_length] [file [name]]
```

**Issue 7 required-option candidate:** `-a; -b; -l`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-a=<suffix_length>; -b=<n>; -l=<line_count>`.

**Operands:** `file; name`. file names the input and name supplies the output prefix; omitted file or file - selects standard input, and omitted name defaults to x. At most one file and one name operand are accepted.

**Special tokens:** A file operand of - selects standard input. Output names append a suffix of suffix_length lowercase letters; the default length is two.

**Standard input:** Used when file is omitted or is -; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Creates sequentially named output files containing line_count lines or the selected byte count, without changing input bytes.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/split`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/split`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/split/split_test.go#TestSplitLines;cmds/split/split_test.go#TestSplitBytes;cmds/split/split_test.go#TestSplitNumericAndSuffixLen;cmds/split/split_test.go#TestSplitPOSIXDefaultSuffixExhaustion;cmds/split/split_test.go#TestSplitOperandsAndPrefix;cmds/split/split_test.go#TestSplitErrors;cmds/split/split_posix_test.go#TestSplitEmptyInputCreatesNoFiles;cmds/split/split_posix_test.go#TestSplitPartialFinalLine;cmds/split/split_posix_test.go#TestSplitDashStdinOperand;cmds/split/split_posix_test.go#TestSplitInputReadErrorIsFailure;cmds/split/split_error_unix_test.go#TestSplitOutputCreateErrorIsFailure`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:split:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [split](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/split.html).

## `strings`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
strings [-a] [-t format] [-n number] [file...]
```

**Issue 7 required-option candidate:** `-a; -n; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<number>; -t=<format>`.

**Operands:** `file`. Process regular-file operands in order and continue after an open or read error; with none, read standard input. -a selects the already-documented whole-file scan; -n takes a positive decimal minimum counted in printable characters (default four); -t takes exactly d, o, or x and reports each string's byte offset from the start of its input file, resetting for each file.

**Special tokens:** -- ends option parsing. Issue 7 leaves a first argument of - unspecified; this implementation treats - as a literal pathname, and standard input is not consumed when any file operands are supplied.

**Standard input:** Used only when no file operand is present; stdin read errors are diagnosed and fail the invocation.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. LC_ALL overrides LC_CTYPE, which overrides LANG. C/POSIX ASCII printability, UTF-8 character-granular classification, exact input-byte preservation, and the carried single-byte provider are evidenced;  arbitrary installed locale databases remain residual.`.

**Standard output:** Write each qualifying printable-character sequence followed by newline. Without -t write only the sequence; in POSIX mode -t writes the unpadded Issue 7 format "%d %s", "%o %s", or "%x %s" using a byte offset. Outside POSIX mode the documented GNU-shaped seven-column padding remains an extension.

**Standard error:** Used only for diagnostics, including invalid option arguments, unsupported locale, operand open/read errors, stdin read errors, and standard-output write errors.

**Effects:** `Scans inputs without modifying them and creates no output files. -a scans every byte; qualifying UTF-8 output preserves the original bytes rather than transcoding.`.

**Exit status:** 0 only when every selected input was read and all output was written; greater than 0 for option, locale, open, read, or write error while successful file operands remain reported.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/strings`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/strings`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/strings/strings_test.go#TestStrings;cmds/strings/strings_test.go#TestStringsFiles;cmds/strings/strings_test.go#TestStringsPOSIXOffsetFormatIsUnpadded;cmds/strings/strings_test.go#TestStringsFileOrderContinuesAfterOpenError;cmds/strings/strings_test.go#TestStringsDashPathname;cmds/strings/strings_test.go#TestStringsUTF8;cmds/strings/strings_test.go#TestStringsUTF8ReplacementCharacterPreservesBytesAndOffset;cmds/strings/strings_test.go#TestStringsLocalePrecedenceControlsPrintability;cmds/strings/strings_test.go#TestStringsIOErrorsAreFailures;cmds/strings/strings_test.go#TestStringsErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:strings:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [strings](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strings.html).

## `strip`

**Evidence state:** `missing`.

**Applicability:** `development`.

**Issue 7 synopsis candidate:**

```text
[development] strip file...
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#strip`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:strip:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [strip](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strip.html).

## `stty`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
stty [-a|-g]
stty operand...
```

**Issue 7 required-option candidate:** `-a; -g`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `parenb (-parenb); parodd (-parodd); cs5 cs6 cs7 cs8; number; ispeed number; ospeed number; hupcl (-hupcl); hup (-hup); cstopb (-cstopb); cread (-cread); clocal (-clocal); ignbrk (-ignbrk); brkint (-brkint); ignpar (-ignpar); parmrk (-parmrk); inpck (-inpck); istrip (-istrip); inlcr (-inlcr); igncr (-igncr); icrnl (-icrnl); ixon (-ixon); ixany (-ixany); ixoff (-ixoff); opost (-opost); onlcr (-onlcr); ocrnl (-ocrnl); onocr (-onocr); onlret (-onlret); ofill (-ofill); ofdel (-ofdel); cr0 cr1 cr2 cr3; nl0 nl1; tab0 tab1 tab2 tab3; tabs (-tabs); bs0 bs1; ff0 ff1; vt0 vt1; isig (-isig); icanon (-icanon); iexten (-iexten); echo (-echo); echoe (-echoe); echok (-echok); echonl (-echonl); noflsh (-noflsh); tostop (-tostop); <control>-character string; min number; time number; saved settings; evenp or parity; oddp; -parity, -evenp, or -oddp; raw (-raw or cooked); nl (-nl); ek; sane`. With no operands, report selected settings; -a reports all settings and -g writes a reusable representation. Setting operands are processed as one requested terminal-state change and invalid settings must not leave a partial change.

**Special tokens:** A leading - on a setting operand negates that setting where defined; - is part of the operand rather than a general option introducer.

**Standard input:** The standard input shall be associated with the terminal whose characteristics are read or changed.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write the default, -a, or -g terminal settings report; setting-only invocations need not write output.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Queries or changes terminal characteristics associated with standard input, including modes, speeds, control characters, minimum bytes, and timeout.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/stty`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/stty`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/stty/stty_test.go#TestSttyRejectsNonTTY;cmds/stty/stty_test.go#TestParseArgsKeepsHyphenSettings;cmds/stty/stty_posix_test.go#TestSttyRowsColsRejectsOverflow;cmds/stty/stty_posix_test.go#TestSttyRequiredReportsPropagateWriteErrors;cmds/stty/stty_termios_test.go#TestApplyTermiosModeRaw;cmds/stty/stty_termios_test.go#TestApplyTermiosValueMinTime`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:stty:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [stty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html).

## `tabs`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
tabs [-T type] n[[sep[+]n]...]
[xsi] tabs [-n|-a|-a2|-c|-c2|-c3|-f|-p|-s|-u] [-T type]
```

**Issue 7 required-option candidate:** `-T`.

**Issue 7 conditional-option candidate:** `xsi:-<n>,-a,-a2,-c,-c2,-c3,-f,-p,-s,-u`.

**Issue 7 option-argument candidate:** `-T=<type>`.

**Operands:** `n[[sep[+]n]...]`. n[[sep[+]n]...] defines repetitive or explicit ascending tab stops; XSI preset operands select the specified assembly-language layout. If no operand or preset is supplied, set tabs every eight columns.

**Special tokens:** A single -n XSI token selects repetitive tabs every n columns and is not an ordinary option requiring a separate argument.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; TERM`.

**Standard output:** When standard output is a terminal, write the terminal-control sequence that clears and sets the requested tab stops; results are undefined when standard output is not a terminal.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Changes the terminal tab-stop settings through control sequences; creates no output files.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tabs`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tabs`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tabs/tabs_test.go#TestPresetColumnsMatchPOSIX;cmds/tabs/tabs_test.go#TestRepetitiveSpec;cmds/tabs/tabs_test.go#TestExplicitList;cmds/tabs/tabs_test.go#TestIncrementForm;cmds/tabs/tabs_test.go#TestTerminalTypeResolution;cmds/tabs/tabs_test.go#TestTabsOutputWriteErrorIsReported`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tabs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tabs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tabs.html).

## `tail`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tail [-f] [-c number|-n number] [file]
```

**Issue 7 required-option candidate:** `-c; -f; -n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-c=<number>; -n=<number>`.

**Operands:** `file`. POSIX admits one optional file operand; more than one is accepted as a documented extension with each preceded by a ==> name <== header and, under -f, followed sequentially rather than concurrently; with no operand (or the operand -) standard input is read; the copied portion defaults to the last 10 lines; when -c and -n are both given the last one on the command line wins (extension resolution of a combination the POSIX synopsis makes exclusive); each failing operand is diagnosed and the rest still processed.

**Special tokens:** -c/-n numbers take an optional sign with origin 1: +N copies from byte/line N onward (the N-1 skip is performed by reading, so it works on non-seekable input), unsigned or -N copies the last N; multiplier suffixes (b, K, KiB, kB, ...) are accepted as extensions; a leading -NUM first argument is rewritten to -n NUM (obsolescent-form extension retained) while +NUM is an ordinary pathname (Issue 7 removed the obsolescent plus form); - means standard input.

**Standard input:** Used when no file operand is given or the operand is -; with -f, a pipe or FIFO on standard input is read once to EOF and -f is ignored per the Issue 7 rule, while a regular file on standard input is followed by descriptor.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** The designated portion of the input file; headers appear only with -v or multiple file operands (extension).

**Standard error:** Used only for diagnostic messages (--debug tracing is an extension).

**Effects:** `Reads only; with -f does not terminate at EOF but polls at --sleep-interval for growth until context cancellation or --pid death, keeping a FIFO's read descriptor open across writer disconnects; --follow=name reopens on rotation, truncation, or max-unchanged-stats (shrink/rename handling is implementation-defined per POSIX; descriptor mode leaves a truncated file at its old offset and emits nothing further).`.

**Exit status:** 0 on success including a cancelled -f follow; 1 on open, read, or standard-output write failure; 2 on usage errors (documented repo deviation within the >0 latitude).

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tail`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tail`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tail/tail_test.go#TestTail;cmds/tail/tail_test.go#TestTailSingleFileExactStdout;cmds/tail/tail_test.go#TestTailHeaders;cmds/tail/tail_test.go#TestTailErrors;cmds/tail/tail_test.go#TestTailWriteFailure;cmds/tail/tail_test.go#TestTailFollowDescriptor;cmds/tail/tail_test.go#TestTailFollowStdinPipeIgnoresFollow;cmds/tail/tail_test.go#TestTailFollowStdinRegularFile;cmds/tail/tail_test.go#TestTailFollowByName;cmds/tail/tail_test.go#TestTailFollowNotSupported;cmds/tail/tail_test.go#TestTailFollowSleepIntervalInvalid;cmds/tail/tail_fifo_unix_test.go#TestTailFollowFIFOAcrossWriters;cmds/tail/tail_posix_test.go#TestTailInputReadErrorIsFailure;cmds/tail/tail_posix_test.go#TestTailBytesFromStartNonSeekable`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tail:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tail](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tail.html).

## `talk`

**Evidence state:** `missing`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] talk address [terminal]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `address; terminal`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#talk`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:talk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [talk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/talk.html).

## `tee`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tee [-ai] [file...]
```

**Issue 7 required-option candidate:** `-a; -i`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Copy standard input without buffering to standard output and every file operand; without -a create or truncate named files, with -a append; support at least 13 file operands; after an open, write, or close failure on one file diagnose it and continue the other opened files and standard output.

**Special tokens:** -- ends option parsing; a file operand of - always names a literal file called -, never standard output; Retained output-error, long-option, and help/version extensions remain available when POSIXLY_CORRECT is present because they do not conflict with the required forms.

**Standard input:** Read as a byte stream of any type and copy without buffering.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write an exact copy of standard input; output failures, including short writes, are diagnosed and return failure.

**Standard error:** Used only for diagnostic messages, including non-signal output errors.

**Effects:** `Creates or truncates each named output file by default using mode 0666 filtered by the invocation umask without changing an existing file mode, appends under -a, ignores SIGINT while -i is active, and otherwise retains the default SIGINT disposition.`.

**Exit status:** 0 only when standard input is copied successfully to standard output and all output files; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tee`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tee`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tee/tee_test.go#TestTeeStdoutOnly;cmds/tee/tee_test.go#TestTeeWritesFiles;cmds/tee/tee_test.go#TestTeeIssue7AtLeastThirteenFileOperands;cmds/tee/tee_test.go#TestTeeTruncatesByDefault;cmds/tee/tee_test.go#TestTeeAppend;cmds/tee/tee_test.go#TestTeeIssue7VirtualUmaskAndExistingMode;cmds/tee/tee_test.go#TestTeeOpenErrorContinues;cmds/tee/tee_test.go#TestTeeDashIsLiteralFileName;cmds/tee/tee_test.go#TestTeeStdoutWriteErrorPOSIX;cmds/tee/tee_test.go#TestTeeIssue7ShortWriteFails;cmds/tee/tee_test.go#TestTeeIssue7InputReadFailurePreservesReadBytes;cmds/tee/tee_test.go#TestTeeIssue7StreamsBeforeEOF;cmds/tee/tee_test.go#TestTeeIssue7OutputFailuresContinue;cmds/tee/tee_test.go#TestTeeIssue7OpenFailureContinuesPortably;cmds/tee/tee_test.go#TestTeeExtensionsRemainAvailableWithPOSIXEnvironment;cmds/tee/run_signal_test.go#TestTeeDefaultInterruptDisposition;cmds/tee/run_signal_test.go#TestTeeDefaultSIGPIPEDisposition;cmds/tee/run_signal_test.go#TestTeeIgnoreInterruptsActual;cmds/tee/tee_posix_linux_test.go#TestTeeIssue7FileWriteFailureContinues`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tee:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tee](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tee.html).

## `test`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
test [expression]
[ [expression] ]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `-b pathname; -c pathname; -d pathname; -e pathname; -f pathname; -g pathname; -h pathname; -L pathname; -n string; -p pathname; -r pathname; -S pathname; -s pathname; -t file_descriptor; -u pathname; -w pathname; -x pathname; -z string; string; s1 = s2; s1 != s2; n1 -eq n2; n1 -ne n2; n1 -gt n2; n1 -ge n2; n1 -lt n2; n1 -le n2; expression1 -a expression2; expression1 -o expression2; ! expression; ( expression )`. Evaluate the expression using the POSIX zero-through-four-argument rules and the specified primaries and operators.

**Special tokens:** The operand -- is expression data: test does not recognize it as the Guideline 10 end-of-options delimiter. A lone - is likewise an expression operand.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `status_only`.

**Exit status:** 0 if expression evaluates true; 1 if it evaluates false; greater than 1 on error or invalid expression.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:test`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestTestIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteTest`; provider=`-`; clauses=`XCU:test:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [test](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/test.html).

## `time`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
time [-p] utility [argument...]
```

**Issue 7 required-option candidate:** `-p`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `utility; argument`. The first operand selects the utility and remaining operands are its arguments; the shell keyword times that command or pipeline and -p selects the portable real, user, and sys report format; command lookup uses PATH and preserves the invoked utility's status.

**Special tokens:** No command-specific special operand is defined; Bash grammar and extensions remain available while the parser and runner are both in POSIX mode.

**Standard input:** Passed unchanged to the invoked utility; time itself does not read it.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; xsi:NLSPATH; PATH`.

**Standard output:** Used only by the invoked utility; the timing report is not written here.

**Standard error:** Receives the portable timing report with real, user, and sys values and any utility or lookup diagnostics.

**Effects:** `Invokes and waits for the selected utility or pipeline and accounts for elapsed, shell, and child user/system CPU time without otherwise changing files or shell state.`.

**Exit status:** The utility or pipeline status when invocation succeeds; 127 when the utility cannot be found; greater than 0 for other timing or invocation errors.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_keyword`).

**Implementation:** `shell:time`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/time_issue7_test.go#TestTimeIssue7CommandInterface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteTime`; provider=`-`; clauses=`XCU:time:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [time](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/time.html).

## `touch`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
touch [-acm] [-r ref_file|-t time|-d date_time] file...
```

**Issue 7 required-option candidate:** `-a; -c; -d; -m; -r; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-d=<date_time>; -r=<ref_file>; -t=<time>`.

**Operands:** `file`. Each file operand is processed independently and a failure continues with the rest; an absent file is created empty with mode 0666 unless -c (whose suppressed creation is not an error); if neither -a nor -m is supplied, both timestamps are selected; without -r, -t, or -d the current time is used; - is an ordinary pathname.

**Special tokens:** -r, -t, and -d are mutually exclusive time sources; -t accepts [[CC]YY]MMDDhhmm[.SS] with 69-99/00-68 century windowing and SS=60 meaning one second after :59 (rolling into the next day at 23:59); -d accepts YYYY-MM-DDThh:mm:SS with T or space separator, period or comma fraction, and empty (local) or Z (UTC) timezone; option parsing stops at the first operand.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; TZ`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Sets the access and/or modification times selected by -a/-m of each file to the current time, the -r reference file's corresponding times, or the -t/-d time interpreted in TZ from the invocation environment; creates absent files unless -c.`.

**Exit status:** 0 when all requested time changes were made; greater than 0 if an error occurred.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/touch`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/touch`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/touch/touch_test.go#TestTouchCreates;cmds/touch/touch_test.go#TestTouchStamp;cmds/touch/touch_test.go#TestTouchStampUsesInvocationTZAndCurrentYear;cmds/touch/touch_test.go#TestTouchStopsOptionParsingAtFirstOperand;cmds/touch/touch_test.go#TestTouchDate;cmds/touch/touch_test.go#TestTouchDateISOSeconds60AndFractions;cmds/touch/touch_test.go#TestTouchNoCreate;cmds/touch/touch_test.go#TestTouchReference;cmds/touch/touch_test.go#TestTouchAccessOnly;cmds/touch/touch_test.go#TestTouchCurrentTimeExisting;cmds/touch/touch_test.go#TestTouchCurrentTimePartial;cmds/touch/touch_test.go#TestTouchErrors;cmds/touch/touch_test.go#TestTouchDashIsAnOrdinaryPathname;cmds/touch/touch_stamp_boundary_test.go#TestTouchStampSecond60RollsForward;cmds/touch/touch_test.go#TestTouchReferenceAtimeUnavailableFailsOnlyWhenNeeded;cmds/touch/touch_pathmax_unix_test.go#TestTouchNearPathMaxRelativeOperands`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:touch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [touch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/touch.html).

## `tput`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tput [-T type] operand...
```

**Issue 7 required-option candidate:** `-T`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-T=<type>`.

**Operands:** `clear; init; reset`. Process clear, init, and reset operands in order; each requests the corresponding terminal sequence, and an operation unavailable for the terminal is not itself an error.

**Special tokens:** NONE

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; TERM`.

**Standard output:** When standard output is a terminal, write the requested clear, initialization, and reset sequences; results are undefined when standard output is not a terminal.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Queries terminal information and emits terminal-control sequences; continues with later operands when an operation is unavailable.`.

**Exit status:** 0 when requested strings are written successfully; 2 for usage error; 3 when terminal information is unavailable; 4 for an invalid operand; greater than 4 for another error; 1 is unspecified.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tput`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tput`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tput/tput_test.go#TestExitStatuses;cmds/tput/tput_test.go#TestTerminalTypeResolution;cmds/tput/tput_test.go#TestInitAndReset;cmds/tput/tput_test.go#TestPOSIXOperationSequence;cmds/tput/tput_test.go#TestPOSIXOperationSequenceAvailabilityAndErrors;cmds/tput/tput_test.go#TestWriteErrorIsReported`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tput:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tput](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tput.html).

## `tr`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tr [-c|-C] [-s] string1 string2
tr -s [-c|-C] string1
tr -d [-c|-C] string1
tr -ds [-c|-C] string1 string2
```

**Issue 7 required-option candidate:** `-c; -C; -d; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string1, string2`. string1 and string2 are translation-control strings interpreted as character arrays. Operand count and use depend on whether translating, deleting, squeezing, or deleting then squeezing.

**Special tokens:** The control-string grammar includes escapes, octal values, ranges, character classes, equivalence classes, and repeated expressions; their meaning is locale-sensitive.

**Standard input:** Read bytes or characters to transform from standard input; any file type is permitted.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write input identically except for the translations, deletions, and repeated-character squeezing requested.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Transforms the standard-input stream and writes the result; creates no output files.`.

**Exit status:** 0 if all input is processed successfully; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tr`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tr`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tr/tr_test.go#TestTrTranslate;cmds/tr/tr_test.go#TestTrDeleteSqueeze;cmds/tr/tr_test.go#TestTrOperandErrors;cmds/tr/tr_test.go#TestTrClasses;cmds/tr/tr_test.go#TestTrInputReadErrorIsReported;cmds/tr/tr_test.go#TestTrEPIPE;cmds/tr/ctype_test.go#TestCTypeProviderBacked;cmds/tr/ctype_test.go#TestCTypeCaseTranslation`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tr.html).

## `true`

**Evidence state:** `implemented`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
true
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No operands are specified.

**Special tokens:** NONE

**Standard input:** Not used.

**Environment:** `none`.

**Standard output:** Not used.

**Standard error:** Not used.

**Effects:** `Changes no state, consumes no input, and produces no output; only the exit status is observable.`.

**Exit status:** Always 0.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:true`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestTrueIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteTrue`; provider=`-`; clauses=`XCU:true:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [true](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/true.html).

## `tsort`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tsort [file]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Read blank-separated pairs of non-empty items; unequal items impose first-before-second ordering and identical items declare presence; write every item once in a total order consistent with all imposed relations; diagnose every detected cycle and continue sorting.

**Special tokens:** -- ends option parsing; this implementation treats a file operand of - as standard input; The retained -w and help/version extensions remain available when POSIXLY_CORRECT is present because they do not conflict with the required operand form.

**Standard input:** Used when file is omitted or is -; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write a text file containing one total ordering consistent with the partial-order input; output errors and short writes are diagnosed.

**Standard error:** Used only for diagnostic messages, including odd input, cycles, and stream failures.

**Effects:** `Reads and closes the selected input without modifying it; output is standard output only.`.

**Exit status:** 0 for successful completion; greater than 0 for malformed input, a cycle, an input open/read/close error, or an output error; usage errors use 2.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tsort`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tsort`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tsort/tsort_test.go#TestTsort;cmds/tsort/tsort_test.go#TestTsortLoop;cmds/tsort/tsort_test.go#TestTsortFileOperand;cmds/tsort/tsort_test.go#TestTsortErrors;cmds/tsort/tsort_test.go#TestTsortPOSIXExample;cmds/tsort/tsort_test.go#TestTsortIssue7BlankSeparatorsAreExact;cmds/tsort/tsort_test.go#TestTsortIssue7StreamFailures;cmds/tsort/tsort_test.go#TestTsortIssue7InputCloseFailure;cmds/tsort/tsort_test.go#TestTsortExtensionsRemainAvailableWithPOSIXEnvironment`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tsort:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tsort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tsort.html).

## `tty`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tty
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No operands are accepted; examine standard input to determine whether it is a terminal and, if so, obtain a ttyname()-equivalent pathname.

**Special tokens:** -- ends option parsing; Issue 7 specifies no tty options or operands.

**Standard input:** Examined but not read to determine terminal status and name.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** For a terminal write its ttyname()-equivalent pathname and newline; otherwise write an informative message, exactly not a tty followed by newline in the POSIX locale.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Examines the standard-input descriptor without consuming input or modifying files.`.

**Exit status:** 0 when standard input is a terminal; 1 when it is not; greater than 1 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tty`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tty`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/tty/tty_posix_test.go#TestTTYTerminalName;cmds/tty/tty_console_darwin_test.go#TestTTYDevConsole;cmds/tty/tty_test.go#TestTTYNotAFile;cmds/tty/tty_test.go#TestTTYWriteError;cmds/tty/tty_test.go#TestTTYErrors;cmds/tty/tty_posix_test.go#TestTTYTerminalSilent;cmds/tty/tty_posix_test.go#TestTTYTerminalWriteError;cmds/tty/tty_test.go#TestTTYShortWriteAndInvalidDescriptorAreErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:tty:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [tty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tty.html).

## `umask`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
umask [-S] [mask]
```

**Issue 7 required-option candidate:** `-S`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `mask`. With mask, accept an octal or symbolic file-creation mask and replace the current shell mask. With no mask, write a representation reusable as a subsequent umask operand; POSIX leaves its default style unspecified. -S selects symbolic output.

**Special tokens:** Symbolic clauses describe permissions that remain enabled, the complement of the mask bits. Bashy's default four-digit octal display is an allowed Bash-compatible output choice; the round-trip property is normative.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** With no mask, write the current mask in reusable default or -S symbolic form followed by a newline; otherwise write nothing.

**Standard error:** Used for invalid options or mask operands and output failures.

**Effects:** `Update the file-creation mask of the current shell execution environment, inherited by subsequently invoked utilities; no files are directly modified.`.

**Exit status:** 0 when the mask is displayed or set successfully; greater than 0 on an invalid option/operand or output error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:umask`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestUmaskIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteUmask`; provider=`-`; clauses=`XCU:umask:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [umask](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/umask.html).

## `unalias`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
unalias alias-name...
unalias -a
```

**Issue 7 required-option candidate:** `-a`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `alias-name`. With alias-name operands, remove each named alias in order; -a removes every alias in the current shell execution environment; an undefined name is diagnosed and does not restore aliases already removed.

**Special tokens:** -- ends option parsing in this implementation, allowing an alias name beginning with - to be selected as an operand.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages, including an undefined alias name or invalid invocation.

**Effects:** `Removes selected aliases or all aliases from the current shell execution environment; a removal in a subshell does not alter the parent.`.

**Exit status:** 0 when all requested aliases are removed; greater than 0 for an undefined name, missing operand, invalid option, or other error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:unalias`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestUnaliasIssue7Interface`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteUnalias`; provider=`-`; clauses=`XCU:unalias:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [unalias](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unalias.html).

## `uname`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uname [-amnrsv]
```

**Issue 7 required-option candidate:** `-a; -m; -n; -r; -s; -v`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. Any operand is rejected as a usage error.

**Special tokens:** -- ends option parsing; trailing words are still rejected operands; Retained GNU selectors, long options, and help/version aliases remain available when POSIXLY_CORRECT is present because they do not conflict with the required selectors.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** One line of the selected symbols in the fixed order sysname nodename release version machine regardless of flag order, separated by single spaces; no flags means -s; -a selects exactly -mnrsv; every selected POSIX symbol must be non-empty; repeated selectors do not duplicate fields; output errors and short writes are diagnosed.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Values come from uname(2) on unix (whitespace runs collapsed); on Windows the kernel name is Windows_NT, release is major.minor.build, version is Build N from RtlGetVersion, and machine maps GOARCH to the GNU spelling; other targets use the Go target, runtime version, hostname, and architecture; an unavailable selected provider value fails loudly.`.

**Exit status:** 0 when the requested information was written; 1 when the system probe, selected provider value, or output fails; 2 usage.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uname`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uname`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/uname/uname_test.go#TestUnameDefaultIsKernelName;cmds/uname/uname_test.go#TestUnameFields;cmds/uname/uname_test.go#TestUnameCombinedAndAll;cmds/uname/uname_test.go#TestUnameAllIsExactlyMNRSV;cmds/uname/uname_test.go#TestUnameErrors;cmds/uname/uname_test.go#TestUnameIssue7SelectorCompositionAndOrder;cmds/uname/uname_test.go#TestUnameIssue7ProviderAndOutputFailures;cmds/uname/uname_test.go#TestUnameExtensionsRemainAvailableWithPOSIXEnvironment;cmds/uname/uname_assemble_test.go#TestAssembleSkipsSyntheticEmptyVersion;cmds/uname/uname_assemble_test.go#TestAssembleFixedOrderWithVersion;cmds/uname/uname_windows_test.go#TestWindowsProbeHasPOSIXVersion`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:uname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [uname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uname.html).

## `unexpand`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
unexpand [-a|-t tablist] [file...]
```

**Issue 7 required-option candidate:** `-a; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-t=<tablist>`.

**Operands:** `file`. Read file operands in order as text; with no file operands or for a file operand of -, read standard input.

**Special tokens:** A file operand of - selects standard input. -t tablist implies -a; tablist is one positive decimal or an ascending comma/blank-separated list.

**Standard input:** Used when no file operands are present and for a file operand of -.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write input equivalently, replacing the maximum eligible runs of spaces with tabs according to the default or selected tab stops.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Converts eligible spaces to tabs while preserving other input and creates no output files.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/unexpand`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/unexpand`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/unexpand/unexpand_test.go#TestUnexpandLeadingBlanks;cmds/unexpand/unexpand_test.go#TestUnexpandAllAndFile;cmds/unexpand/unexpand_test.go#TestUnexpandTabsImpliesAll;cmds/unexpand/unexpand_test.go#TestUnexpandBlanksBeyondLastStopUnchanged;cmds/unexpand/unexpand_test.go#TestUnexpandBackspaceDecrementsColumn;cmds/unexpand/unexpand_test.go#TestUnexpandRejectsBadTabs;cmds/unexpand/locale_test.go#TestPOSIXUTF8UsesDisplayColumnsAndLocaleBlanks;cmds/unexpand/locale_test.go#TestPOSIXSingleByteLocaleUsesProviderAndPrecedence`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:unexpand:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [unexpand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unexpand.html).

## `uniq`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uniq [-c|-d|-u] [-f fields] [-s char] [input_file [output_file]]
```

**Issue 7 required-option candidate:** `-c; -d; -f; -s; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<fields>; -s=<chars>`.

**Operands:** `input_file; output_file`. With no input_file, or input_file -, read standard input; an optional output_file names the destination, with - meaning standard output; only adjacent matching lines are grouped.

**Special tokens:** Fields are runs of non-blank characters after leading blanks; -f skips fields before -s skips characters; in POSIX mode their option-arguments must be positive decimal integers, while zero remains an accepted GNU extension outside POSIX mode; -c, -d, and -u select count, duplicate, and unique output modes; unsupported GNU extensions remain outside POSIX evidence.

**Standard input:** Read when no input_file is given or input_file is -.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH. C/POSIX byte-unit behavior is evidenced with LC_ALL=C and POSIXLY_CORRECT;  interpretation under any non-C LC_CTYPE and locale-specific blank classes remain residual.`.

**Standard output:** Write the selected first line of each adjacent group by default, or counted/duplicate/unique output according to options, unless an output_file operand redirects output.

**Standard error:** Used only for diagnostics, including invalid counts, operand errors, unreadable input, unwritable output, and output errors.

**Effects:** `May create or truncate the output_file operand when one is supplied; otherwise no filesystem effects beyond reading input.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurred; usage errors return 2 in this implementation.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uniq`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uniq`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/uniq/uniq_test.go#TestUniq;cmds/uniq/uniq_test.go#TestUniqPOSIXCCharacterUnits;cmds/uniq/uniq_test.go#TestUniqPOSIXRequiresPositiveSkipArguments;cmds/uniq/uniq_test.go#TestUniqOperands;cmds/uniq/uniq_test.go#TestUniqErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:uniq:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [uniq](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html).

## `uudecode`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uudecode [-o outfile] [file]
```

**Issue 7 required-option candidate:** `-o`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-o=<outfile>`.

**Operands:** `file`. file names one encoded input; when omitted, read standard input. The encoded header names the output unless -o overrides it.

**Special tokens:** An encoded output pathname of - or /dev/stdout, or -o /dev/stdout, selects standard output.

**Standard input:** Used when file is omitted; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Used only when the encoded header names - or /dev/stdout, or -o names /dev/stdout; then write the decoded original byte stream.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Decodes the historical or Base64 representation, creates or overwrites the selected output, and sets access permission bits from the header subject to required safety rules.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uudecode`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uudecode`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/uudecode/uudecode_test.go#TestDecodeHeaderOutputAndMode;cmds/uudecode/uudecode_test.go#TestDecodeBase64AndHeaderScanning;cmds/uudecode/uudecode_test.go#TestPOSIXRejectsMultipleInputFiles;cmds/uudecode/uudecode_test.go#TestHeaderPathnamesAndSymbolicModes;cmds/uudecode/uudecode_test.go#TestHeaderAndOutputOverrideStdoutDistinctions;cmds/uudecode/uudecode_test.go#TestChmodFailureIsWarningAndNonfatal;cmds/uudecode/uudecode_unix_test.go#TestHeaderModeIgnoresUmask;cmds/uudecode/uudecode_unix_test.go#TestDecodeRefusesResolvedTargetWithoutEffectiveWriteAccess`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:uudecode:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [uudecode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uudecode.html).

## `uuencode`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uuencode [-m] [file] decode_pathname
```

**Issue 7 required-option candidate:** `-m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `decode_pathname; file`. decode_pathname is required and becomes the decoded output pathname in the header; optional file supplies input, otherwise standard input is used.

**Special tokens:** A decode_pathname of /dev/stdout instructs uudecode to use standard output. The ordinary file operand - is not specified as a stdin token by Issue 7.

**Standard input:** Used only when the file operand is omitted.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write the encoded text: historical uuencode framing by default or begin-base64 framing under -m, including the input access mode, decode pathname, bounded data lines, and required terminator.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Encodes input to standard output and creates no output files.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uuencode`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uuencode`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/uuencode/uuencode_test.go#TestEncodeKnownVectorFromStdin;cmds/uuencode/uuencode_test.go#TestClassicUsesSpacesForZeroSextets;cmds/uuencode/uuencode_test.go#TestEncodeFileAndMode;cmds/uuencode/uuencode_test.go#TestEncodeBase64AndModes;cmds/uuencode/uuencode_test.go#TestStandardInputFileModeComesFromFstat;cmds/uuencode/uuencode_test.go#TestExplicitInputFileDoesNotInspectStandardInput;cmds/uuencode/uuencode_test.go#TestErrors`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:uuencode:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [uuencode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uuencode.html).

## `vi`

**Evidence state:** `missing`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] vi [-rR] [-c command] [-t tagstring] [-w size] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-c,-r,-R,-t,-w`.

**Issue 7 option-argument candidate:** `-c=<command>; -t=<tagstring>; -w=<size>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `COLUMNS; EXINIT; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LINES; xsi:NLSPATH; PATH; SHELL; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#vi`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:vi:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [vi](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/vi.html).

## `wait`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
wait [pid...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `pid`. With no operands, waits for all active background jobs known to the current shell; each pid operand names a known background process ID or job-control job ID, completed statuses remain available until requested, multiple operands are processed in order, and the final operand determines the result.

**Special tokens:** No POSIX option or special token is defined; Bash's -n, -p, and -f forms are retained and tested separately as extensions.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostics such as an unknown PID or job ID.

**Effects:** `Blocks until the selected background jobs terminate, consumes explicitly requested retained statuses, and with no operands waits for all active known jobs.`.

**Exit status:** 0 after a successful no-operand wait; otherwise the status of the last requested job, 127 for an unknown final PID/job, or a value greater than 128 when interrupted by a signal.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:wait`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell semantic=`sh:interp/issue7_command_interface_test.go#TestWaitIssue7Interface;sh:interp/issue7_command_interface_test.go#TestWaitIssue7CompletedStatusIsRetainedUntilRequested;sh:interp/issue7_command_interface_test.go#TestWaitIssue7ZeroOperandsBlocksUntilAllJobsComplete`; shell routing=`bashy:internal/cli/profile_b_routing_test.go#TestProfileBRouteWait`; provider=`-`; clauses=`XCU:wait:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [wait](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wait.html).

## `wc`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
wc [-c|-m] [-lw] [file...]
```

**Issue 7 required-option candidate:** `-c; -l; -m; -w`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. Read each file operand; with no file operands, read standard input. A file operand of - selects standard input in this implementation.

**Special tokens:** A file operand of - selects standard input. -c and -m are mutually exclusive count selectors.

**Standard input:** Used when no file operands are present and for a file operand of -; otherwise not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write selected newline, word, byte, or character counts in required order for each input; omit the pathname for sole stdin, and append a total line when more than one file operand is supplied.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Counts the selected properties of each input and writes totals; creates no output files.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/wc`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/wc`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/wc/wc_test.go#TestWcStdin;cmds/wc/wc_test.go#TestWcFile;cmds/wc/wc_test.go#TestWcMultipleAndTotal;cmds/wc/wc_test.go#TestWcCharsAndMaxLine;cmds/wc/wc_test.go#TestWcWordRules;cmds/wc/wc_test.go#TestWcErrors;cmds/wc/locale_test.go#TestPOSIXUTF8CountsCharactersAndLocaleWords;cmds/wc/locale_test.go#TestPOSIXSingleByteLocaleUsesProviderAndPrecedence`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:wc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [wc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wc.html).

## `who`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
who [-mTu] [file]
[xsi] who [-abdHlprt] [file]
[xsi] who [-mu] -s [-bHlprt] [file]
[xsi] who -q [file]
[xsi] who am i
[xsi] who am I
```

**Issue 7 required-option candidate:** `-m; -T; -u`.

**Issue 7 conditional-option candidate:** `xsi:-a,-b,-d,-H,-l,-p,-q,-r,-s,-t`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file; am i; am I`. file substitutes for the implementation-defined login database. On XSI systems in the POSIX locale, separate operands am i or am I limit output to the invoking user, equivalent to -m.

**Special tokens:** am i and am I are two-operand XSI forms, not a single string; -q is an XSI quick-list selector.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; xsi:NLSPATH; TZ`.

**Standard output:** Write implementation-defined login information; XSI formats include the required fields, terminal-state form under -T, selector-specific records, and quick-list output under -q.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Reads the selected login-accounting database and terminal state; creates no output files.`.

**Exit status:** 0 on successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/who`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/who`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/who/who_test.go#TestWhoOperands;cmds/who/who_test.go#TestWhoQuietIgnoresOtherOptions;cmds/who/who_test.go#TestWhoTExactNoOptionalComment;cmds/who/who_test.go#TestWhoAllIsExactAndTruthful;cmds/who/who_test.go#TestWhoLCtimeProviderAndFailClosedResidual;cmds/who/who_binary_posix_test.go#TestWhoBinaryABIBehavior;cmds/who/who_native_linux_test.go#TestWhoNativeLinuxDifferential`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:who:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [who](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/who.html).

## `write`

**Evidence state:** `partial`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
write user_name [terminal]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `user_name; terminal`. user_name is the recipient login name. Optional terminal identifies one of that user's logged-in terminals in the format reported by who.

**Special tokens:** NONE

**Standard input:** Read lines to copy to the recipient terminal.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH`.

**Standard output:** Write an informational message when the recipient is logged in more than once.

**Standard error:** Used only for diagnostic messages.

**Effects:** `Writes the prescribed banner, message lines, and end marker to an eligible recipient terminal, subject to login and message-permission checks.`.

**Exit status:** 0 on successful completion; greater than 0 when the addressed user is not logged in or denies permission.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/write`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/write`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/write/write_test.go#TestDeliversBannerBodyAndEOF;cmds/write/write_test.go#TestMultiLoginNoticeGoesToStdoutAndAlertsGoToControllingTerminal;cmds/write/write_test.go#TestTypedBELReachesTheRecipientAsAByte;cmds/write/write_test.go#TestCanonicalEOLAndNewlineBothDelimit;cmds/write/write_test.go#TestSIGINTWritesEOTReturnsSuccessAndLeaksNothing;cmds/write/write_test.go#TestWriteErrorToTerminalIsReported;cmds/write/locale_test.go#TestPOSIXSingleByteLocaleUsesAllClassesAndPrecedence;cmds/write/pty_linux_test.go#TestPTYBackedWriteLinux`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:write:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [write](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/write.html).

## `xargs`

**Evidence state:** `partial`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] xargs [-ptx] [-E eofstr] [-I replstr|-L number|-n number] [-s size] [utility [argument...]]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-E,-I,-L,-n,-p,-s,-t,-x`.

**Issue 7 option-argument candidate:** `-E=<eofstr>; -I=<replstr>; -L=<number>; -n=<number>; -s=<size>`.

**Operands:** `utility; argument`. Use utility and each initial argument first, then append arguments constructed from logical lines read from standard input.

**Special tokens:** -- ends xargs option parsing; a lone - begins the utility operand because it is not an xargs option.

**Standard input:** Read logical lines and construct arguments using blank, newline, quote, and backslash rules; /dev/tty supplies -p responses.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; xsi:NLSPATH; PATH`.

**Standard output:** Not used by xargs itself; the invoked utility can write to its inherited standard output.

**Standard error:** Used for diagnostics, every generated command line under -t, and both the generated command line and prompt under -p.

**Effects:** `invokes_utility`.

**Exit status:** 0 if all utility invocations return 0; 1-125 if a command line cannot be assembled, an invocation returns non-zero, or another error occurs; 126 if utility is found but cannot be invoked; 127 if utility cannot be found.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`manual`).

**Implementation:** `cmds/xargs`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/xargs`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/xargs/xargs_test.go#TestXargsQuotesAndBackslash;cmds/xargs/xargs_test.go#TestXargsLogicalLines;cmds/xargs/xargs_test.go#TestXargsReplaceIssue7Limits;cmds/xargs/xargs_test.go#TestXargsSizeLimitAndExactMode;cmds/xargs/xargs_test.go#TestXargsInteractiveReadsControllingTerminal;cmds/xargs/xargs_test.go#TestXargsInteractiveUsesLCMessagesYesexpr;cmds/xargs/xargs_test.go#TestXargsPOSIXLYCorrectDoesNotDisableExtensions;cmds/xargs/xargs_test.go#TestXargsEmptyReplacementState;cmds/xargs/xargs_test.go#TestXargsPOSIXStrictSize;cmds/xargs/xargs_test.go#TestXargsPOSIXArgMaxCeilingAllowsEquality;cmds/xargs/xargs_test.go#TestXargsLastReplacementOrInputLimitOptionWins;cmds/xargs/xargs_test.go#TestXargsChildEnvironmentAndTerminalStatuses;cmds/xargs/xargs_test.go#TestXargsReadErrorStopsBeforeExecution;cmds/xargs/xargs_resolve_test.go#TestXargsFoundNotExecutableIs126;cmds/xargs/xargs_resolve_test.go#TestXargsNotFoundIs127;cmds/xargs/xargs_resolve_test.go#TestXargsChildExitMapsTo123`; shell semantic=`-`; shell routing=`-`; provider=`-`; clauses=`XCU:xargs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Integration/full-profile evidence:** `-`.

**Issue 7 source:** [xargs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/xargs.html).
