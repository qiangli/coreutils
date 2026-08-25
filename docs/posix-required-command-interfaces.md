# POSIX-required command interface evidence ledger

Generated from `docs/posix-required-command-interfaces.tsv` by
`scripts/posix_manifest.py`. This ledger is an audit aid, not a normative
specification or a claim of complete POSIX conformance. `UNVERIFIED` means
the command-specific Issue 7 semantics have not yet been transcribed and
backed by focused repository evidence.

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
| Evidence | Partial | 2 |
| Evidence | Unverified | 114 |

Completion is deliberately fail-closed: `scripts/posix_manifest.py
--require-complete` fails until all 116 rows have focused behavioral evidence
and complete normative semantics. The parser scan below is only a conservative
source-token audit; finding a token is never proof of runtime behavior.

Evidence is lane-specific. Go references stay in `cmds/<command>`; provider
references name a command-specific test in `cmds/posixproviders`; shell
references use `sh:<path>#<TestID>` against the sibling sh repository. A
missing cross-repository shell reference cannot support partial or verified state.

For verified rows, `NONE` explicitly records an empty option-argument or
operand set; `-` in those normative slots means missing data. Likewise, paired
`-` synopsis or option fields are incomplete, and normative prose cannot be `-`.

## `alias`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
alias [alias-name[=string]...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `alias-name; alias-name=string`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:alias`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:alias:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [alias](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/alias.html).

## `ar`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TMPDIR; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ar`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ar:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ar](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ar.html).

## `at`

**Evidence state:** `unverified`.

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

**Operands:** `at_job_id; timespec; time; midnight; noon; now; date; today; tomorrow; increment`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; LC_TIME; SHELL; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/at`.

**Conservative source-token audit:** token gaps: options=-m; argument-form gaps=none; source `cmds/at`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:at:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [at](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/at.html).

## `awk`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
awk [-F sepstring] [-v assignment]... program [argument...]
awk [-F sepstring] -f progfile [-f progfile]... [-v assignment]... [argument...]
```

**Issue 7 required-option candidate:** `-F; -f; -v`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-F=<sepstring>; -f=<progfile>; -v=<assignment>`.

**Operands:** `program; argument; file; assignment`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/awk`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/awk`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:awk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [awk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/awk.html).

## `basename`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
basename string [suffix]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string; suffix`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/basename`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/basename`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:basename:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [basename](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/basename.html).

## `batch`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
batch
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; SHELL; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/batch`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/batch`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:batch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [batch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/batch.html).

## `bc`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#bc`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:bc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [bc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bc.html).

## `bg`

**Evidence state:** `unverified`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] bg [job_id...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `job_id`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:bg`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:bg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [bg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bg.html).

## `cat`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cat [-u] [file...]
```

**Issue 7 required-option candidate:** `-u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cat`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cat`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:cat:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [cat](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html).

## `cd`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cd [-L|-P] [directory]
cd -
```

**Issue 7 required-option candidate:** `-L; -P`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `directory; -`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `CDPATH; HOME; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; OLDPWD; PWD`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:cd`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:cd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [cd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cd.html).

## `chgrp`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
chgrp [-h] group file...
chgrp -R [-H|-L|-P] group file...
```

**Issue 7 required-option candidate:** `-h; -H; -L; -P; -R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `group; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/chgrp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/chgrp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:chgrp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [chgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chgrp.html).

## `chmod`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
chmod [-R] mode file...
```

**Issue 7 required-option candidate:** `-R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `mode; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/chmod`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/chmod`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:chmod:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [chmod](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chmod.html).

## `chown`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
chown [-h] owner[:group] file...
chown -R [-H|-L|-P] owner[:group] file...
```

**Issue 7 required-option candidate:** `-h; -H; -L; -P; -R`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `owner[:group]; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/chown`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/chown`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:chown:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [chown](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html).

## `cksum`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cksum [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cksum`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cksum`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:cksum:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [cksum](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cksum.html).

## `cmp`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
cmp [-l|-s] file1 file2
```

**Issue 7 required-option candidate:** `-l; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file1; file2`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** 0 files identical; 1 files differ; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cmp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cmp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:cmp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [cmp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cmp.html).

## `comm`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
comm [-123] file1 file2
```

**Issue 7 required-option candidate:** `-1; -2; -3`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file1; file2`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/comm`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/comm`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:comm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [comm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/comm.html).

## `command`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
command [-p] command_name [argument...]
command [-p][-v|-V] command_name
```

**Issue 7 required-option candidate:** `-p; -v; -V`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `argument; command_name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:command`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:command:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [command](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/command.html).

## `cp`

**Evidence state:** `unverified`.

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

**Operands:** `source_file; target_file; target`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:cp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [cp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html).

## `crontab`

**Evidence state:** `unverified`.

**Applicability:** `base; optional`.

**Issue 7 synopsis candidate:**

```text
crontab [file]
[optional] crontab [-e|-l|-r]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-e,-l,-r`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `EDITOR; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/crontab`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/crontab`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:crontab:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [crontab](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/crontab.html).

## `csplit`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
csplit [-ks] [-f prefix] [-n number] file arg...
```

**Issue 7 required-option candidate:** `-f; -k; -n; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<prefix>; -n=<number>`.

**Operands:** `file; /rexp/[offset]; %rexp%[offset]; line_no; {num}`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/csplit`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/csplit`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:csplit:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [csplit](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/csplit.html).

## `ctags`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ctags`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ctags:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ctags](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ctags.html).

## `cut`

**Evidence state:** `unverified`.

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

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/cut`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/cut`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:cut:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [cut](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html).

## `date`

**Evidence state:** `unverified`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
date [-u] [+format]
[xsi] date [-u] mmddhhmm[[cc]yy]
```

**Issue 7 required-option candidate:** `-u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `+format; mmddhhmm[[cc]yy]`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/date`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/date`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:date:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [date](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/date.html).

## `dd`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
dd [operand...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `if=file; of=file; ibs=expr; obs=expr; bs=expr; cbs=expr; skip=n; seek=n; count=n; conv=value[,value ...]; ascii; ebcdic; ibm; block; unblock; lcase; ucase; swab; noerror; notrunc; sync`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/dd`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/dd`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:dd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [dd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dd.html).

## `df`

**Evidence state:** `unverified`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] df [-k] [-P|-t] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-k,-P,-t`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/df`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/df`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:df:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [df](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/df.html).

## `diff`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
diff [-c|-e|-f|-u|-C n|-U n] [-br] file1 file2
```

**Issue 7 required-option candidate:** `-b; -c; -C; -e; -f; -r; -u; -U`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-C=<n>; -U=<n>`.

**Operands:** `file1, file2`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** 0 no differences; 1 differences found; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/diff`.

**Conservative source-token audit:** token gaps: options=none; argument-form gaps=-C=<n>, -U=<n>; source `cmds/diff`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:diff:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [diff](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/diff.html).

## `dirname`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
dirname string
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/dirname`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/dirname`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:dirname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [dirname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html).

## `du`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
du [-a|-s] [-kx] [-H|-L] [file...]
```

**Issue 7 required-option candidate:** `-a; -H; -k; -L; -s; -x`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/du`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/du`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:du:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [du](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/du.html).

## `echo`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
echo [string...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string; \a; \b; \c; \f; \n; \r; \t; \v; \\; \0num`. Zero or more string operands are separated by one space in output; XSI escape sequences in operands are interpreted.

**Special tokens:** The operand -- is data: echo does not recognize it as the Guideline 10 end-of-options delimiter. A lone - is also a string operand.

**Standard input:** Not used.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** Write the string operands separated by single spaces and followed by a newline; with no operands, write only a newline. XSI escape processing can suppress that newline with \c.

**Standard error:** Used only for diagnostic messages.

**Effects:** `stdout_only`.

**Exit status:** 0 for successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:echo`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:echo:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [echo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/echo.html).

## `ed`

**Evidence state:** `unverified`.

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

**Environment:** `HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ed`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ed:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ed.html).

## `env`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
env [-i] [name=value]... [utility [argument...]]
```

**Issue 7 required-option candidate:** `-i`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `name=value; utility; argument`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/env`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/env`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:env:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [env](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html).

## `ex`

**Evidence state:** `unverified`.

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

**Environment:** `COLUMNS; EXINIT; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LINES; NLSPATH; PATH; SHELL; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#ex`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ex:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ex](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ex.html).

## `expand`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
expand [-t tablist] [file...]
```

**Issue 7 required-option candidate:** `-t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-t=<tablist>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/expand`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/expand`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:expand:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [expand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html).

## `expr`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
expr operand...
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `operand`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** 0 non-null/non-zero result; 1 null/zero result; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/expr`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/expr`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:expr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [expr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html).

## `false`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
false
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No operands are specified.

**Special tokens:** UNVERIFIED

**Standard input:** Not used.

**Environment:** `none`.

**Standard output:** Not used.

**Standard error:** Not used.

**Effects:** `status_only`.

**Exit status:** Always greater than 0.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:false`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:false:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [false](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/false.html).

## `fc`

**Evidence state:** `unverified`.

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

**Operands:** `first, last; [+]number; -number; string; old=new`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `FCEDIT; HISTFILE; HISTSIZE; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:fc`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:fc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [fc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fc.html).

## `fg`

**Evidence state:** `unverified`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] fg [job_id]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `job_id`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:fg`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:fg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [fg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fg.html).

## `file`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
file [-dh] [-M file] [-m file] file...
file -i [-h] file...
```

**Issue 7 required-option candidate:** `-d; -h; -i; -M; -m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-M=<file>; -m=<file>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/file`.

**Conservative source-token audit:** token gaps: options=-M, -m; argument-form gaps=-M=<file>, -m=<file>; source `cmds/file`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:file:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [file](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/file.html).

## `find`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
find [-H|-L] path... [operand_expression...]
```

**Issue 7 required-option candidate:** `-H; -L`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `+n; n; -n; -name pattern; -path pattern; -nouser; -nogroup; -xdev; -prune; -perm [-]mode; -perm [-]onum; -type c; -links n; -user uname; -group gname; -size n[c]; -atime n; -ctime n; -mtime n; -exec utility_name [argument ...] ; ; -exec utility_name [argument ...] {} +; -ok utility_name [argument ...] ; ; -print; -newer file; -depth; ( expression ); ! expression; expression [-a] expression; expression -o expression`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/find`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/find`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:find:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [find](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/find.html).

## `fold`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
fold [-bs] [-w width] [file...]
```

**Issue 7 required-option candidate:** `-b; -s; -w`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-w=<width>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/fold`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/fold`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:fold:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [fold](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html).

## `getconf`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
getconf [-v specification] system_var
getconf [-v specification] path_var pathname
```

**Issue 7 required-option candidate:** `-v`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-v=<specification>`.

**Operands:** `path_var; pathname; system_var`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/getconf`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/getconf`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:getconf:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [getconf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html).

## `getopts`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
getopts optstring name [arg...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `optstring; name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; OPTIND`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:getopts`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:getopts:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [getopts](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getopts.html).

## `grep`

**Evidence state:** `unverified`.

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

**Operands:** `pattern_list; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** 0 selected lines found; 1 none found; greater than 1 on error.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/grep`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/grep`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:grep:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [grep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/grep.html).

## `hash`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
hash [utility...]
hash -r
```

**Issue 7 required-option candidate:** `-r`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `utility`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:hash`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:hash:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [hash](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/hash.html).

## `head`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
head [-n number] [file...]
```

**Issue 7 required-option candidate:** `-n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<number>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/head`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/head`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:head:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [head](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/head.html).

## `iconv`

**Evidence state:** `unverified`.

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

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/iconv`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/iconv`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:iconv:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [iconv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/iconv.html).

## `id`

**Evidence state:** `unverified`.

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

**Operands:** `user`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/id`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/id`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:id:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [id](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html).

## `jobs`

**Evidence state:** `unverified`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] jobs [-l|-p] [job_id...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-l,-p`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `job_id`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:jobs`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:jobs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [jobs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/jobs.html).

## `join`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
join [-a file_number|-v file_number] [-e string] [-o list] [-t char] [-1 field] [-2 field] file1 file2
```

**Issue 7 required-option candidate:** `-a; -e; -o; -t; -v; -1; -2`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-a=<file_number>; -e=<string>; -o=<list>; -t=<char>; -v=<file_number>; -1=<field>; -2=<field>`.

**Operands:** `file1, file2`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/join`.

**Conservative source-token audit:** token gaps: options=none; argument-form gaps=-1=<field>, -2=<field>, -a=<file_number>, -e=<string>, -o=<list>, -t=<char>, -v=<file_number>; source `cmds/join`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:join:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [join](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/join.html).

## `kill`

**Evidence state:** `unverified`.

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

**Operands:** `pid; exit_status`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:kill`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:kill:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [kill](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/kill.html).

## `ln`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
ln [-fs] [-L|-P] source_file target_file
ln [-fs] [-L|-P] source_file... target_dir
```

**Issue 7 required-option candidate:** `-f; -L; -P; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `source_file; target_file; target_dir`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/ln`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/ln`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ln:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ln](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ln.html).

## `locale`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
locale [-a|-m]
locale [-ck] name...
```

**Issue 7 required-option candidate:** `-a; -c; -k; -m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/locale`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/locale`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:locale:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [locale](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/locale.html).

## `localedef`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#localedef`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:localedef:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [localedef](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/localedef.html).

## `logger`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
logger string...
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `string`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/logger`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/logger`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:logger:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [logger](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logger.html).

## `logname`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
logname
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/logname`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/logname`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:logname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [logname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logname.html).

## `lp`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; LPDEST; NLSPATH; PRINTER; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#lp`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:lp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [lp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/lp.html).

## `ls`

**Evidence state:** `unverified`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] ls [-ikqrs] [-glno] [-A|-a] [-C|-m|-x|-1]  [-F|-p] [-H|-L] [-R|-d] [-S|-f|-t] [-c|-u] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-A,-C,-F,-H,-L,-R,-S,-a,-c,-d,-f,-g,-i,-k,-l,-m,-n,-o,-p,-q,-r,-s,-t,-u,-x,-1`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `COLUMNS; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/ls`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/ls`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ls:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ls](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html).

## `m4`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#m4`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:m4:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [m4](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/m4.html).

## `mailx`

**Evidence state:** `unverified`.

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

**Environment:** `DEAD; EDITOR; HOME; LANG; LC_ALL; LC_CTYPE; LC_TIME; LC_MESSAGES; LISTER; MAILRC; MBOX; NLSPATH; PAGER; SHELL; TERM; TZ; VISUAL`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#mailx`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:mailx:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [mailx](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mailx.html).

## `make`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; MAKEFLAGS; NLSPATH; PROJECTDIR`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#make`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:make:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [make](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/make.html).

## `man`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PAGER`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#man`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:man:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [man](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/man.html).

## `mesg`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mesg [y|n]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `y; n`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mesg`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mesg`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:mesg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [mesg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html).

## `mkdir`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mkdir [-p] [-m mode] dir...
```

**Issue 7 required-option candidate:** `-m; -p`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-m=<mode>`.

**Operands:** `dir`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mkdir`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mkdir`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:mkdir:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [mkdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkdir.html).

## `mkfifo`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mkfifo [-m mode] file...
```

**Issue 7 required-option candidate:** `-m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-m=<mode>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mkfifo`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mkfifo`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:mkfifo:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [mkfifo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkfifo.html).

## `more`

**Evidence state:** `unverified`.

**Applicability:** `optional`.

**Issue 7 synopsis candidate:**

```text
[optional] more [-ceisu] [-n number] [-p command] [-t tagstring] [file...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `optional:-c,-e,-i,-n,-p,-s,-t,-u`.

**Issue 7 option-argument candidate:** `-n=<number>; -p=<command>; -t=<tagstring>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `COLUMNS; EDITOR; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; LINES; MORE; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/more`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/more`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:more:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [more](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/more.html).

## `mv`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
mv [-if] source_file target_file
mv [-if] source_file... target_dir
```

**Issue 7 required-option candidate:** `-f; -i`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `source_file; target_file; target_dir`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/mv`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/mv`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:mv:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [mv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mv.html).

## `newgrp`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
newgrp [-l] [group]
```

**Issue 7 required-option candidate:** `-l`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `group`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/newgrp`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/newgrp`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:newgrp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [newgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/newgrp.html).

## `nice`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
nice [-n increment] utility [argument...]
```

**Issue 7 required-option candidate:** `-n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<increment>`.

**Operands:** `utility; argument`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/nice`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/nice`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:nice:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [nice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nice.html).

## `nm`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#nm`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:nm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [nm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nm.html).

## `nohup`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
nohup utility [argument...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `utility; argument`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `HOME; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/nohup`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/nohup`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:nohup:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [nohup](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html).

## `od`

**Evidence state:** `unverified`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
od [-v] [-A address_base] [-j skip] [-N count] [-t type_string]... [file...]
[xsi] od [-bcdosx] [file] [[+]offset[.][b]]
```

**Issue 7 required-option candidate:** `-A; -j; -N; -t; -v`.

**Issue 7 conditional-option candidate:** `xsi:-b,-c,-d,-o,-s,-x`.

**Issue 7 option-argument candidate:** `-A=<address_base>; -j=<skip>; -N=<count>; -t=<type_string>`.

**Operands:** `file; [+]offset[.][b]`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/od`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/od`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:od:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [od](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html).

## `paste`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
paste [-s] [-d list] file...
```

**Issue 7 required-option candidate:** `-d; -s`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-d=<list>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/paste`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/paste`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:paste:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [paste](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/paste.html).

## `patch`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; LC_TIME`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#patch`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:patch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [patch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/patch.html).

## `pathchk`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
pathchk [-p] [-P] pathname...
```

**Issue 7 required-option candidate:** `-p; -P`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `pathname`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/pathchk`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/pathchk`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:pathchk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [pathchk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html).

## `pax`

**Evidence state:** `unverified`.

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

**Operands:** `directory; file; pattern`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TMPDIR; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/pax`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/pax`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:pax:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html).

## `pr`

**Evidence state:** `partial`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
pr [+page] [-column] [-adFmrt] [-e[char][gap]] [-h header] [-i[char][gap]] [-l lines] [-n[char][width]] [-o offset] [-s[char]] [-w width] [-fp] [file...]
```

**Issue 7 required-option candidate:** `+<page>; -<column>; -a; -d; -e; -f; -F; -h; -i; -m; -n; -o; -p; -r; -s; -t; -w`.

**Issue 7 conditional-option candidate:** `xsi:-l`.

**Issue 7 option-argument candidate:** `-e[char][gap]; -h=<header>; -i[char][gap]; -l=<lines>; -n[char][width]; -o=<offset>; -s[char]; -w=<width>`.

**Operands:** `file`. Read file operands in order; a missing file operand or a file operand of - selects standard input. +page selects the first output page and -column selects the column count.

**Special tokens:** A file operand of - selects standard input. pr is exempt from several Utility Syntax Guidelines; this interface makes no -- delimiter claim.

**Standard input:** Read as a text file when no file operand is given or when a file operand is -; /dev/tty supplies -p responses.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Standard output:** Write paginated input, including headers and trailers unless -t suppresses them; formatting is controlled by the declared page, column, tab, numbering, width, and merge options.

**Standard error:** Used for diagnostics and to alert the terminal when -p is specified.

**Effects:** `stdout_and_terminal_prompt`.

**Exit status:** 0 for successful completion; greater than 0 if an error occurs.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/pr`.

**Conservative source-token audit:** token gaps: options=+<page>; argument-form gaps=none; source `cmds/pr`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/pr/pr_test.go`; shell=`-`; provider=`-`; clauses=`XCU:pr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [pr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pr.html).

## `printf`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
printf format [argument...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `format; argument`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:printf`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:printf:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [printf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/printf.html).

## `ps`

**Evidence state:** `unverified`.

**Applicability:** `xsi`.

**Issue 7 synopsis candidate:**

```text
[xsi] ps [-aA] [-defl] [-g grouplist] [-G grouplist] [-n namelist] [-o format]... [-p proclist] [-t termlist] [-u userlist] [-U userlist]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `xsi:-a,-A,-d,-e,-f,-g,-G,-l,-n,-o,-p,-t,-u,-U`.

**Issue 7 option-argument candidate:** `-g=<grouplist>; -G=<grouplist>; -n=<namelist>; -o=<format>; -p=<proclist>; -t=<termlist>; -u=<userlist>; -U=<userlist>`.

**Operands:** `none`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `COLUMNS; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/ps`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/ps`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:ps:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [ps](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ps.html).

## `pwd`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
pwd [-L|-P]
```

**Issue 7 required-option candidate:** `-L; -P`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_MESSAGES; NLSPATH; PWD`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:pwd`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:pwd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [pwd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pwd.html).

## `read`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
read [-r] var...
```

**Issue 7 required-option candidate:** `-r`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `var`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `IFS; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PS2`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:read`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:read:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [read](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/read.html).

## `renice`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
renice [-g|-p|-u] -n increment ID...
```

**Issue 7 required-option candidate:** `-g; -n; -p; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<increment>`.

**Operands:** `ID`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/renice`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/renice`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:renice:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [renice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html).

## `rm`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
rm [-iRr] file...
rm -f [-iRr] [file...]
```

**Issue 7 required-option candidate:** `-f; -i; -R; -r`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/rm`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/rm`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:rm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [rm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rm.html).

## `rmdir`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
rmdir [-p] dir...
```

**Issue 7 required-option candidate:** `-p`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `dir`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/rmdir`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/rmdir`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:rmdir:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [rmdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rmdir.html).

## `sed`

**Evidence state:** `unverified`.

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

**Operands:** `file; script`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/sed`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/sed`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:sed:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html).

## `sh`

**Evidence state:** `unverified`.

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

**Operands:** `-; argument; command_file; command_name; command_string`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `ENV; FCEDIT; HISTFILE; HISTSIZE; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; MAIL; MAILCHECK; MAILPATH; NLSPATH; PATH; PWD`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_entrypoint`).

**Implementation:** `shell:sh`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:sh:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [sh](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sh.html).

## `sleep`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
sleep time
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `time`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/sleep`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/sleep`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:sleep:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [sleep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sleep.html).

## `sort`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
sort [-m] [-o output] [-bdfinru] [-t char] [-k keydef]... [file...]
sort [-c|-C] [-bdfinru] [-t char] [-k keydef] [file]
```

**Issue 7 required-option candidate:** `-c; -C; -m; -o; -u; -d; -f; -i; -n; -r; -b; -t; -k`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-o=<output>; -t=<char>; -k=<keydef>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/sort`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/sort`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:sort:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [sort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sort.html).

## `split`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
split [-l line_count] [-a suffix_length] [file [name]]
split -b n[k|m] [-a suffix_length] [file [name]]
```

**Issue 7 required-option candidate:** `-a; -b; -l`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-a=<suffix_length>; -b=<n>; -l=<line_count>`.

**Operands:** `file; name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/split`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/split`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:split:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [split](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/split.html).

## `strings`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
strings [-a] [-t format] [-n number] [file...]
```

**Issue 7 required-option candidate:** `-a; -n; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-n=<number>; -t=<format>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/strings`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/strings`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:strings:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [strings](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strings.html).

## `strip`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#strip`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:strip:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [strip](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strip.html).

## `stty`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
stty [-a|-g]
stty operand...
```

**Issue 7 required-option candidate:** `-a; -g`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `parenb (-parenb); parodd (-parodd); cs5 cs6 cs7 cs8; number; ispeed number; ospeed number; hupcl (-hupcl); hup (-hup); cstopb (-cstopb); cread (-cread); clocal (-clocal); ignbrk (-ignbrk); brkint (-brkint); ignpar (-ignpar); parmrk (-parmrk); inpck (-inpck); istrip (-istrip); inlcr (-inlcr); igncr (-igncr); icrnl (-icrnl); ixon (-ixon); ixany (-ixany); ixoff (-ixoff); opost (-opost); onlcr (-onlcr); ocrnl (-ocrnl); onocr (-onocr); onlret (-onlret); ofill (-ofill); ofdel (-ofdel); cr0 cr1 cr2 cr3; nl0 nl1; tab0 tab1 tab2 tab3; tabs (-tabs); bs0 bs1; ff0 ff1; vt0 vt1; isig (-isig); icanon (-icanon); iexten (-iexten); echo (-echo); echoe (-echoe); echok (-echok); echonl (-echonl); noflsh (-noflsh); tostop (-tostop); <control>-character string; min number; time number; saved settings; evenp or parity; oddp; -parity, -evenp, or -oddp; raw (-raw or cooked); nl (-nl); ek; sane`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`custom`).

**Implementation:** `cmds/stty`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/stty`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:stty:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [stty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html).

## `tabs`

**Evidence state:** `unverified`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
tabs [-T type] n[[sep[+]n]...]
[xsi] tabs [-n|-a|-a2|-c|-c2|-c3|-f|-p|-s|-u] [-T type]
```

**Issue 7 required-option candidate:** `-T`.

**Issue 7 conditional-option candidate:** `xsi:-<n>,-a,-a2,-c,-c2,-c3,-f,-p,-s,-u`.

**Issue 7 option-argument candidate:** `-T=<type>`.

**Operands:** `n[[sep[+]n]...]`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tabs`.

**Conservative source-token audit:** token gaps: options=-<n>, -a2, -c2, -c3; argument-form gaps=none; source `cmds/tabs`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tabs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tabs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tabs.html).

## `tail`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tail [-f] [-c number|-n number] [file]
```

**Issue 7 required-option candidate:** `-c; -f; -n`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-c=<number>; -n=<number>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tail`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tail`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tail:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tail](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tail.html).

## `talk`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#talk`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:talk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [talk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/talk.html).

## `tee`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tee [-ai] [file...]
```

**Issue 7 required-option candidate:** `-a; -i`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tee`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tee`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tee:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tee](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tee.html).

## `test`

**Evidence state:** `unverified`.

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

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** Not used.

**Standard error:** Used only for diagnostic messages.

**Effects:** `status_only`.

**Exit status:** 0 if expression evaluates true; 1 if it evaluates false; greater than 1 on error or invalid expression.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:test`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:test:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [test](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/test.html).

## `time`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
time [-p] utility [argument...]
```

**Issue 7 required-option candidate:** `-p`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `utility; argument`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH; PATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_keyword`).

**Implementation:** `shell:time`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:time:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [time](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/time.html).

## `touch`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
touch [-acm] [-r ref_file|-t time|-d date_time] file...
```

**Issue 7 required-option candidate:** `-a; -c; -d; -m; -r; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-d=<date_time>; -r=<ref_file>; -t=<time>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/touch`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/touch`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:touch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [touch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/touch.html).

## `tput`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tput [-T type] operand...
```

**Issue 7 required-option candidate:** `-T`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-T=<type>`.

**Operands:** `clear; init; reset`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tput`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tput`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tput:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tput](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tput.html).

## `tr`

**Evidence state:** `unverified`.

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

**Operands:** `string1, string2`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tr`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tr`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tr.html).

## `true`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
true
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. No operands are specified.

**Special tokens:** UNVERIFIED

**Standard input:** Not used.

**Environment:** `none`.

**Standard output:** Not used.

**Standard error:** Not used.

**Effects:** `status_only`.

**Exit status:** Always 0.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:true`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:true:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [true](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/true.html).

## `tsort`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tsort [file]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tsort`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tsort`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tsort:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tsort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tsort.html).

## `tty`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
tty
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/tty`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/tty`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:tty:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [tty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tty.html).

## `umask`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
umask [-S] [mask]
```

**Issue 7 required-option candidate:** `-S`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `mask`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:umask`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:umask:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [umask](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/umask.html).

## `unalias`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
unalias alias-name...
unalias -a
```

**Issue 7 required-option candidate:** `-a`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `alias-name`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:unalias`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:unalias:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [unalias](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unalias.html).

## `uname`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uname [-amnrsv]
```

**Issue 7 required-option candidate:** `-a; -m; -n; -r; -s; -v`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `none`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uname`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uname`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:uname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [uname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uname.html).

## `unexpand`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
unexpand [-a|-t tablist] [file...]
```

**Issue 7 required-option candidate:** `-a; -t`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-t=<tablist>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/unexpand`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/unexpand`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:unexpand:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [unexpand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unexpand.html).

## `uniq`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uniq [-c|-d|-u] [-f fields] [-s char] [input_file [output_file]]
```

**Issue 7 required-option candidate:** `-c; -d; -f; -s; -u`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-f=<fields>; -s=<chars>`.

**Operands:** `input_file; output_file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uniq`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uniq`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:uniq:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [uniq](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html).

## `uudecode`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uudecode [-o outfile] [file]
```

**Issue 7 required-option candidate:** `-o`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `-o=<outfile>`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uudecode`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uudecode`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:uudecode:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [uudecode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uudecode.html).

## `uuencode`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
uuencode [-m] [file] decode_pathname
```

**Issue 7 required-option candidate:** `-m`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `decode_pathname; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/uuencode`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/uuencode`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:uuencode:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [uuencode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uuencode.html).

## `vi`

**Evidence state:** `unverified`.

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

**Environment:** `COLUMNS; EXINIT; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LINES; NLSPATH; PATH; SHELL; TERM`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `external_provider`.

**Effective owner:** `external_provider` (`external`).

**Implementation:** `pkg/posixprovider/manifest.tsv#vi`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:vi:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [vi](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/vi.html).

## `wait`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
wait [pid...]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `pid`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `shell_only`.

**Effective owner:** `shell` (`shell_builtin`).

**Implementation:** `shell:wait`.

**Conservative source-token audit:** not applicable to a Go-selected parser; source `-`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:wait:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [wait](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wait.html).

## `wc`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
wc [-c|-m] [-lw] [file...]
```

**Issue 7 required-option candidate:** `-c; -l; -m; -w`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/wc`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/wc`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:wc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [wc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wc.html).

## `who`

**Evidence state:** `unverified`.

**Applicability:** `base; xsi`.

**Issue 7 synopsis candidate:**

```text
who -q [file]
who am i
who am I
[xsi] who [-mTu] [-abdHlprt] [file]
[xsi] who [-mu] -s [-bHlprt] [file]
```

**Issue 7 required-option candidate:** `-q`.

**Issue 7 conditional-option candidate:** `xsi:-a,-b,-d,-H,-l,-m,-p,-r,-s,-t,-T,-u`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `am i, am I; file`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/who`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/who`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:who:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [who](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/who.html).

## `write`

**Evidence state:** `unverified`.

**Applicability:** `base`.

**Issue 7 synopsis candidate:**

```text
write user_name [terminal]
```

**Issue 7 required-option candidate:** `none`.

**Issue 7 conditional-option candidate:** `none`.

**Issue 7 option-argument candidate:** `none`.

**Operands:** `user_name; terminal`. UNVERIFIED

**Special tokens:** UNVERIFIED

**Standard input:** UNVERIFIED

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Standard output:** UNVERIFIED

**Standard error:** UNVERIFIED

**Effects:** `UNVERIFIED`.

**Exit status:** UNVERIFIED

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`flagset`).

**Implementation:** `cmds/write`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/write`. This audit is not proof of behavior.

**Evidence lanes:** Go=`-`; shell=`-`; provider=`-`; clauses=`XCU:write:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

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

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Standard output:** Not used by xargs itself; the invoked utility can write to its inherited standard output.

**Standard error:** Used for diagnostics, every generated command line under -t, and both the generated command line and prompt under -p.

**Effects:** `invokes_utility`.

**Exit status:** 0 if all utility invocations return 0; 1-125 if a command line cannot be assembled, an invocation returns non-zero, or another error occurs; 126 if utility is found but cannot be invoked; 127 if utility cannot be found.

**Compatibility scope:** POSIX Issue 7 only; GNU compatibility is out of scope.

**Availability:** `go`.

**Effective owner:** `go` (`manual`).

**Implementation:** `cmds/xargs`.

**Conservative source-token audit:** tokens found for all declared options and argument forms; behavioral evidence still required; source `cmds/xargs`. This audit is not proof of behavior.

**Evidence lanes:** Go=`cmds/xargs/xargs_test.go;cmds/xargs/xargs_resolve_test.go`; shell=`-`; provider=`-`; clauses=`XCU:xargs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`.

**Issue 7 source:** [xargs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/xargs.html).
