# POSIX-required command interfaces for Profiles C/D

Generated from the canonical machine-readable data in
`docs/posix-required-commands.tsv` by `scripts/posix_manifest.py`.
The 116 sections below preserve requirement applicability and keep
mandatory base interfaces separate from XSI, software-development,
other optional, and GNU-only material.

### Availability axis

| Implementation available | Count |
| --- | ---: |
| Go same-name applet | 86 |
| Shell-only name | 14 |
| Pinned external provider | 16 |

### Effective Profile C/D selection axis

| Selected implementation | Count |
| --- | ---: |
| Go | 78 |
| Shell | 22 |
| Pinned external provider | 16 |

Availability and effective selection are independent: eight available Go
applets are intentionally shadowed by shell interfaces.

## `alias`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
alias [alias-name[=string]...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** alias-name; alias-name=string. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: alias-name;alias-name=string.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:alias`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:alias`; clauses `XCU:alias:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [alias](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/alias.html).

## `ar`

**Requirement / applicability:** base; xsi; development.

**Normative POSIX synopsis:**

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

**Mandatory base options:** `-a; -b; -c; -i; -m; -r; -u; -v`.

**Conditional / optional options:** `development:-d; xsi:-C,-p,-q,-s,-t,-T,-x`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** archive; file; posname. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: archive;file;posname.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TMPDIR; TZ`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#ar`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#ar`; clauses `XCU:ar:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ar](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ar.html).

## `at`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
at [-m] [-f file] [-q queuename] -t time_arg
at [-m] [-f file] [-q queuename] timespec...
at -r at_job_id...
at -l -q queuename
at -l [at_job_id...]
```

**Mandatory base options:** `-f; -l; -m; -q; -r; -t`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<file>; -q=<queuename>; -t=<time_arg>`.

**Operands / arity / order:** at_job_id; timespec; time; midnight; noon; now; date; today; tomorrow; increment. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: at_job_id;timespec;time;midnight;noon;now;date;today;tomorrow;increment.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; LC_TIME; SHELL; TZ`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/at`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/at`; clauses `XCU:at:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [at](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/at.html).

## `awk`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
awk [-F sepstring] [-v assignment]... program [argument...]
awk [-F sepstring] -f progfile [-f progfile]... [-v assignment]... [argument...]
```

**Mandatory base options:** `-F; -f; -v`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-F=<sepstring>; -f=<progfile>; -v=<assignment>`.

**Operands / arity / order:** program; argument; file; assignment. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: program;argument;file;assignment.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH; PATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`custom`).

**Implementation source:** `cmds/awk`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/awk`; clauses `XCU:awk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [awk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/awk.html).

## `basename`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
basename string [suffix]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** string; suffix. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: string;suffix.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/basename`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/basename`; clauses `XCU:basename:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [basename](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/basename.html).

## `batch`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
batch
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; SHELL; TZ`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/batch`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/batch`; clauses `XCU:batch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [batch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/batch.html).

## `bc`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
bc [-l] [file...]
```

**Mandatory base options:** `-l`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#bc`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#bc`; clauses `XCU:bc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [bc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bc.html).

## `bg`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] bg [job_id...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** job_id. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: job_id.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:bg`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:bg`; clauses `XCU:bg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [bg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/bg.html).

## `cat`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
cat [-u] [file...]
```

**Mandatory base options:** `-u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/cat`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/cat`; clauses `XCU:cat:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [cat](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cat.html).

## `cd`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
cd [-L|-P] [directory]
cd -
```

**Mandatory base options:** `-L; -P`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** directory; -. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: directory;-.

**Special `-` / `--` / standard input:** -:select the previous working directory; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `CDPATH; HOME; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; OLDPWD; PWD`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:cd`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:cd`; clauses `XCU:cd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [cd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cd.html).

## `chgrp`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
chgrp [-h] group file...
chgrp -R [-H|-L|-P] group file...
```

**Mandatory base options:** `-h; -H; -L; -P; -R`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** group; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: group;file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/chgrp`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/chgrp`; clauses `XCU:chgrp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [chgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chgrp.html).

## `chmod`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
chmod [-R] mode file...
```

**Mandatory base options:** `-R`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** mode; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: mode;file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/chmod`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/chmod`; clauses `XCU:chmod:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [chmod](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chmod.html).

## `chown`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
chown [-h] owner[:group] file...
chown -R [-H|-L|-P] owner[:group] file...
```

**Mandatory base options:** `-h; -H; -L; -P; -R`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** owner[:group]; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: owner[:group];file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/chown`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/chown`; clauses `XCU:chown:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [chown](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/chown.html).

## `cksum`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
cksum [file...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/cksum`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/cksum`; clauses `XCU:cksum:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [cksum](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cksum.html).

## `cmp`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
cmp [-l|-s] file1 file2
```

**Mandatory base options:** `-l; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file1; file2. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file1;file2.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 files identical; 1 files differ; greater than 1 on error.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/cmp`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/cmp`; clauses `XCU:cmp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [cmp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cmp.html).

## `comm`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
comm [-123] file1 file2
```

**Mandatory base options:** `-1; -2; -3`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file1; file2. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file1;file2.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/comm`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/comm`; clauses `XCU:comm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [comm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/comm.html).

## `command`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
command [-p] command_name [argument...]
command [-p][-v|-V] command_name
```

**Mandatory base options:** `-p; -v; -V`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** argument; command_name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: argument;command_name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:command`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:command`; clauses `XCU:command:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [command](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/command.html).

## `cp`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
cp [-Pfip] source_file target_file
cp [-Pfip] source_file... target
cp -R [-H|-L|-P] [-fip] source_file... target
```

**Mandatory base options:** `-f; -H; -i; -L; -P; -p; -R`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** source_file; target_file; target. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: source_file;target_file;target.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/cp`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/cp`; clauses `XCU:cp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [cp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cp.html).

## `crontab`

**Requirement / applicability:** base; optional.

**Normative POSIX synopsis:**

```text
crontab [file]
[optional] crontab [-e|-l|-r]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `optional:-e,-l,-r`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `EDITOR; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/crontab`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/crontab`; clauses `XCU:crontab:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [crontab](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/crontab.html).

## `csplit`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
csplit [-ks] [-f prefix] [-n number] file arg...
```

**Mandatory base options:** `-f; -k; -n; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<prefix>; -n=<number>`.

**Operands / arity / order:** file; /rexp/[offset]; %rexp%[offset]; line_no; {num}. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file;/rexp/[offset];%rexp%[offset];line_no;{num}.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/csplit`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/csplit`; clauses `XCU:csplit:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [csplit](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/csplit.html).

## `ctags`

**Requirement / applicability:** base; development.

**Normative POSIX synopsis:**

```text
ctags -x pathname...
[development] ctags [-a] [-f tagsfile] pathname...
```

**Mandatory base options:** `-x`.

**Conditional / optional options:** `development:-a,-f`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<tagsfile>`.

**Operands / arity / order:** file.c; file.h; file.f. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.c;file.h;file.f.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#ctags`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#ctags`; clauses `XCU:ctags:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ctags](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ctags.html).

## `cut`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
cut -b list [-n] [file...]
cut -c list [file...]
cut -f list [-d delim] [-s] [file...]
```

**Mandatory base options:** `-b; -c; -d; -f; -n; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-b=<list>; -c=<list>; -d=<delim>; -f=<list>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/cut`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/cut`; clauses `XCU:cut:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [cut](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/cut.html).

## `date`

**Requirement / applicability:** base; xsi.

**Normative POSIX synopsis:**

```text
date [-u] [+format]
[xsi] date [-u] mmddhhmm[[cc]yy]
```

**Mandatory base options:** `-u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** +format; mmddhhmm[[cc]yy]. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: +format;mmddhhmm[[cc]yy].

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/date`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/date`; clauses `XCU:date:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [date](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/date.html).

## `dd`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
dd [operand...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** if=file; of=file; ibs=expr; obs=expr; bs=expr; cbs=expr; skip=n; seek=n; count=n; conv=value[,value ...]; ascii; ebcdic; ibm; block; unblock; lcase; ucase; swab; noerror; notrunc; sync. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: if=file;of=file;ibs=expr;obs=expr;bs=expr;cbs=expr;skip=n;seek=n;count=n;conv=value[,value ...];ascii;ebcdic;ibm;block;unblock;lcase;ucase;swab;noerror;notrunc;sync.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`custom`).

**Implementation source:** `cmds/dd`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/dd`; clauses `XCU:dd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [dd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dd.html).

## `df`

**Requirement / applicability:** xsi.

**Normative POSIX synopsis:**

```text
[xsi] df [-k] [-P|-t] [file...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `xsi:-k,-P,-t`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/df`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/df`; clauses `XCU:df:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [df](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/df.html).

## `diff`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
diff [-c|-e|-f|-u|-C n|-U n] [-br] file1 file2
```

**Mandatory base options:** `-b; -c; -C; -e; -f; -r; -u; -U`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-C=<n>; -U=<n>`.

**Operands / arity / order:** file1, file2. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file1, file2.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 no differences; 1 differences found; greater than 1 on error.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/diff`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/diff`; clauses `XCU:diff:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [diff](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/diff.html).

## `dirname`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
dirname string
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** string. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: string.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/dirname`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/dirname`; clauses `XCU:dirname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [dirname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/dirname.html).

## `du`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
du [-a|-s] [-kx] [-H|-L] [file...]
```

**Mandatory base options:** `-a; -H; -k; -L; -s; -x`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/du`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/du`; clauses `XCU:du:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [du](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/du.html).

## `echo`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
echo [string...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** string; \a; \b; \c; \f; \n; \r; \t; \v; \\; \0num. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: string;\a;\b;\c;\f;\n;\r;\t;\v;\\;\0num.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:echo`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:echo`; clauses `XCU:echo:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [echo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/echo.html).

## `ed`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
ed [-p string] [-s] [file]
```

**Mandatory base options:** `-p; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-p=<string>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#ed`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#ed`; clauses `XCU:ed:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ed.html).

## `env`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
env [-i] [name=value]... [utility [argument...]]
```

**Mandatory base options:** `-i`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** name=value; utility; argument. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: name=value;utility;argument.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `process_or_shell_state;stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/env`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/env`; clauses `XCU:env:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [env](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/env.html).

## `ex`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] ex [-rR] [-s|-v] [-c command] [-t tagstring] [-w size] [file...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `optional:-c,-r,-R,-s,-t,-v,-w`.

**GNU-only material:** `none`.

**Option arguments:** `-c=<command>; -t=<tagstring>; -w=<size>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `COLUMNS; EXINIT; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LINES; NLSPATH; PATH; SHELL; TERM`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#ex`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#ex`; clauses `XCU:ex:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ex](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ex.html).

## `expand`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
expand [-t tablist] [file...]
```

**Mandatory base options:** `-t`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-t=<tablist>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/expand`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/expand`; clauses `XCU:expand:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [expand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expand.html).

## `expr`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
expr operand...
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** operand. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: operand.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 non-null/non-zero result; 1 null/zero result; greater than 1 on error.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`custom`).

**Implementation source:** `cmds/expr`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/expr`; clauses `XCU:expr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [expr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/expr.html).

## `false`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
false
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `none`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: Always greater than zero.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:false`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:false`; clauses `XCU:false:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [false](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/false.html).

## `fc`

**Requirement / applicability:** base; optional.

**Normative POSIX synopsis:**

```text
fc -l [-nr] [first [last]]
fc -s [old=new] [first]
[optional] fc [-r] [-e editor] [first [last]]
```

**Mandatory base options:** `-l; -n; -r; -s`.

**Conditional / optional options:** `optional:-e`.

**GNU-only material:** `none`.

**Option arguments:** `-e=<editor>`.

**Operands / arity / order:** first, last; [+]number; -number; string; old=new. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: first, last;[+]number;-number;string;old=new.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `FCEDIT; HISTFILE; HISTSIZE; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:fc`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:fc`; clauses `XCU:fc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [fc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fc.html).

## `fg`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] fg [job_id]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** job_id. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: job_id.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:fg`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:fg`; clauses `XCU:fg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [fg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fg.html).

## `file`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
file [-dh] [-M file] [-m file] file...
file -i [-h] file...
```

**Mandatory base options:** `-d; -h; -i; -M; -m`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-M=<file>; -m=<file>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/file`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/file`; clauses `XCU:file:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [file](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/file.html).

## `find`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
find [-H|-L] path... [operand_expression...]
```

**Mandatory base options:** `-H; -L`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** +n; n; -n; -name pattern; -path pattern; -nouser; -nogroup; -xdev; -prune; -perm [-]mode; -perm [-]onum; -type c; -links n; -user uname; -group gname; -size n[c]; -atime n; -ctime n; -mtime n; -exec utility_name [argument ...] ; ; -exec utility_name [argument ...] {} +; -ok utility_name [argument ...] ; ; -print; -newer file; -depth; ( expression ); ! expression; expression [-a] expression; expression -o expression. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: +n;n;-n;-name pattern;-path pattern;-nouser;-nogroup;-xdev;-prune;-perm [-]mode;-perm [-]onum;-type c;-links n;-user uname;-group gname;-size n[c];-atime n;-ctime n;-mtime n;-exec utility_name [argument ...] ;;-exec utility_name [argument ...] {} +;-ok utility_name [argument ...] ;;-print;-newer file;-depth;( expression );! expression;expression [-a] expression;expression -o expression.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`custom`).

**Implementation source:** `cmds/find`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/find`; clauses `XCU:find:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [find](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/find.html).

## `fold`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
fold [-bs] [-w width] [file...]
```

**Mandatory base options:** `-b; -s; -w`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-w=<width>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/fold`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/fold`; clauses `XCU:fold:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [fold](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/fold.html).

## `getconf`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
getconf [-v specification] system_var
getconf [-v specification] path_var pathname
```

**Mandatory base options:** `-v`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-v=<specification>`.

**Operands / arity / order:** path_var; pathname; system_var. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: path_var;pathname;system_var.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/getconf`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/getconf`; clauses `XCU:getconf:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [getconf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getconf.html).

## `getopts`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
getopts optstring name [arg...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** optstring; name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: optstring;name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; OPTIND`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:getopts`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:getopts`; clauses `XCU:getopts:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [getopts](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/getopts.html).

## `grep`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
grep [-E|-F] [-c|-l|-q] [-insvx] -e pattern_list [-e pattern_list]... [-f pattern_file]... [file...]
grep [-E|-F] [-c|-l|-q] [-insvx] [-e pattern_list]... -f pattern_file [-f pattern_file]... [file...]
grep [-E|-F] [-c|-l|-q] [-insvx] pattern_list [file...]
```

**Mandatory base options:** `-E; -F; -c; -e; -f; -i; -l; -n; -q; -s; -v; -x`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-e=<pattern_list>; -f=<pattern_file>`.

**Operands / arity / order:** pattern_list; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: pattern_list;file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 selected lines found; 1 none found; greater than 1 on error.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/grep`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/grep`; clauses `XCU:grep:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [grep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/grep.html).

## `hash`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
hash [utility...]
hash -r
```

**Mandatory base options:** `-r`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** utility. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: utility.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:hash`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:hash`; clauses `XCU:hash:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [hash](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/hash.html).

## `head`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
head [-n number] [file...]
```

**Mandatory base options:** `-n`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-n=<number>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/head`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/head`; clauses `XCU:head:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [head](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/head.html).

## `iconv`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
iconv [-cs] -f frommap -t tomap [file...]
iconv -f fromcode [-cs] [-t tocode] [file...]
iconv -t tocode [-cs] [-f fromcode] [file...]
iconv -l
```

**Mandatory base options:** `-c; -f; -l; -s; -t`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<fromcodeset>; -t=<tocodeset>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/iconv`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/iconv`; clauses `XCU:iconv:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [iconv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/iconv.html).

## `id`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
id [user]
id -G [-n] [user]
id -g [-nr] [user]
id -u [-nr] [user]
```

**Mandatory base options:** `-G; -g; -n; -r; -u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** user. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: user.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/id`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/id`; clauses `XCU:id:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [id](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/id.html).

## `jobs`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] jobs [-l|-p] [job_id...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `optional:-l,-p`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** job_id. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: job_id.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:jobs`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:jobs`; clauses `XCU:jobs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [jobs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/jobs.html).

## `join`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
join [-a file_number|-v file_number] [-e string] [-o list] [-t char] [-1 field] [-2 field] file1 file2
```

**Mandatory base options:** `-a; -e; -o; -t; -v; -1; -2`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-a=<file_number>; -e=<string>; -o=<list>; -t=<char>; -v=<file_number>; -1=<field>; -2=<field>`.

**Operands / arity / order:** file1, file2. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file1, file2.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/join`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/join`; clauses `XCU:join:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [join](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/join.html).

## `kill`

**Requirement / applicability:** base; xsi.

**Normative POSIX synopsis:**

```text
kill -s signal_name pid...
kill -l [exit_status]
kill [-signal_number] pid...
[xsi] kill [-signal_name] pid...
```

**Mandatory base options:** `-l; -s; -<signal_number>`.

**Conditional / optional options:** `xsi:-<signal_name>`.

**GNU-only material:** `none`.

**Option arguments:** `-s=<signal_name>`.

**Operands / arity / order:** pid; exit_status. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: pid;exit_status.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:kill`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:kill`; clauses `XCU:kill:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [kill](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/kill.html).

## `ln`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
ln [-fs] [-L|-P] source_file target_file
ln [-fs] [-L|-P] source_file... target_dir
```

**Mandatory base options:** `-f; -L; -P; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** source_file; target_file; target_dir. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: source_file;target_file;target_dir.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/ln`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/ln`; clauses `XCU:ln:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ln](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ln.html).

## `locale`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
locale [-a|-m]
locale [-ck] name...
```

**Mandatory base options:** `-a; -c; -k; -m`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/locale`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/locale`; clauses `XCU:locale:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [locale](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/locale.html).

## `localedef`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
localedef [-c] [-f charmap] [-i sourcefile] [-u code_set_name] name
```

**Mandatory base options:** `-c; -f; -i; -u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<charmap>; -i=<inputfile>; -u=<code_set_name>`.

**Operands / arity / order:** name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#localedef`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#localedef`; clauses `XCU:localedef:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [localedef](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/localedef.html).

## `logger`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
logger string...
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** string. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: string.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/logger`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/logger`; clauses `XCU:logger:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [logger](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logger.html).

## `logname`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
logname
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/logname`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/logname`; clauses `XCU:logname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [logname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/logname.html).

## `lp`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
lp [-c] [-d dest] [-n copies] [-msw] [-o option]... [-t title] [file...]
```

**Mandatory base options:** `-c; -d; -m; -n; -o; -s; -t; -w`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-d=<dest>; -n=<copies>; -o=<option>; -t=<title>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; LPDEST; NLSPATH; PRINTER; TZ`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#lp`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#lp`; clauses `XCU:lp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [lp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/lp.html).

## `ls`

**Requirement / applicability:** xsi.

**Normative POSIX synopsis:**

```text
[xsi] ls [-ikqrs] [-glno] [-A|-a] [-C|-m|-x|-1]  [-F|-p] [-H|-L] [-R|-d] [-S|-f|-t] [-c|-u] [file...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `xsi:-A,-C,-F,-H,-L,-R,-S,-a,-c,-d,-f,-g,-i,-k,-l,-m,-n,-o,-p,-q,-r,-s,-t,-u,-x,-1`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `COLUMNS; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/ls`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/ls`; clauses `XCU:ls:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ls](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ls.html).

## `m4`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
m4 [-s] [-D name[=val]]... [-U name]... file...
```

**Mandatory base options:** `-s; -D; -U`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-D=<name[=val]>; -U=<name>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#m4`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#m4`; clauses `XCU:m4:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [m4](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/m4.html).

## `mailx`

**Requirement / applicability:** base; optional.

**Normative POSIX synopsis:**

```text
mailx [-s subject] address...
mailx [-HiNn] [-F] [-u user]
mailx -f [-HiNn] [-F] [file]
[optional] mailx -e
```

**Mandatory base options:** `-f; -F; -H; -i; -n; -N; -s; -u`.

**Conditional / optional options:** `optional:-e`.

**GNU-only material:** `none`.

**Option arguments:** `-s=<subject>; -u=<user>`.

**Operands / arity / order:** address; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: address;file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `DEAD; EDITOR; HOME; LANG; LC_ALL; LC_CTYPE; LC_TIME; LC_MESSAGES; LISTER; MAILRC; MBOX; NLSPATH; PAGER; SHELL; TERM; TZ; VISUAL`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#mailx`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#mailx`; clauses `XCU:mailx:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [mailx](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mailx.html).

## `make`

**Requirement / applicability:** development.

**Normative POSIX synopsis:**

```text
[development] make [-einpqrst] [-f makefile]... [-k|-S] [macro=value...] [target_name...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `development:-e,-f,-i,-k,-n,-p,-q,-r,-S,-s,-t`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<makefile>`.

**Operands / arity / order:** target_name; macro=value. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: target_name;macro=value.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; MAKEFLAGS; NLSPATH; PROJECTDIR`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#make`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#make`; clauses `XCU:make:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [make](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/make.html).

## `man`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
man [-k] name...
```

**Mandatory base options:** `-k`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PAGER`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#man`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#man`; clauses `XCU:man:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [man](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/man.html).

## `mesg`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
mesg [y|n]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** y; n. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: y;n.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/mesg`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/mesg`; clauses `XCU:mesg:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [mesg](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mesg.html).

## `mkdir`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
mkdir [-p] [-m mode] dir...
```

**Mandatory base options:** `-m; -p`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-m=<mode>`.

**Operands / arity / order:** dir. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: dir.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/mkdir`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/mkdir`; clauses `XCU:mkdir:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [mkdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkdir.html).

## `mkfifo`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
mkfifo [-m mode] file...
```

**Mandatory base options:** `-m`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-m=<mode>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/mkfifo`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/mkfifo`; clauses `XCU:mkfifo:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [mkfifo](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mkfifo.html).

## `more`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] more [-ceisu] [-n number] [-p command] [-t tagstring] [file...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `optional:-c,-e,-i,-n,-p,-s,-t,-u`.

**GNU-only material:** `none`.

**Option arguments:** `-n=<number>; -p=<command>; -t=<tagstring>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `COLUMNS; EDITOR; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; LINES; MORE; TERM`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/more`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/more`; clauses `XCU:more:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [more](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/more.html).

## `mv`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
mv [-if] source_file target_file
mv [-if] source_file... target_dir
```

**Mandatory base options:** `-f; -i`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** source_file; target_file; target_dir. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: source_file;target_file;target_dir.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/mv`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/mv`; clauses `XCU:mv:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [mv](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/mv.html).

## `newgrp`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
newgrp [-l] [group]
```

**Mandatory base options:** `-l`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** group. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: group.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/newgrp`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/newgrp`; clauses `XCU:newgrp:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [newgrp](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/newgrp.html).

## `nice`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
nice [-n increment] utility [argument...]
```

**Mandatory base options:** `-n`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-n=<increment>`.

**Operands / arity / order:** utility; argument. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: utility;argument.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/nice`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/nice`; clauses `XCU:nice:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [nice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nice.html).

## `nm`

**Requirement / applicability:** xsi; development.

**Normative POSIX synopsis:**

```text
[development] nm [-APv] [-g|-u] [-t format] file...
[xsi] nm [-APv] [-efox] [-g|-u] [-t format] file...
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `development:-A,-g,-P,-t,-u,-v; xsi:-e,-f,-o,-x`.

**GNU-only material:** `none`.

**Option arguments:** `-t=<format>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#nm`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#nm`; clauses `XCU:nm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [nm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nm.html).

## `nohup`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
nohup utility [argument...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** utility; argument. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: utility;argument.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `HOME; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/nohup`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/nohup`; clauses `XCU:nohup:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [nohup](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/nohup.html).

## `od`

**Requirement / applicability:** base; xsi.

**Normative POSIX synopsis:**

```text
od [-v] [-A address_base] [-j skip] [-N count] [-t type_string]... [file...]
[xsi] od [-bcdosx] [file] [[+]offset[.][b]]
```

**Mandatory base options:** `-A; -j; -N; -t; -v`.

**Conditional / optional options:** `xsi:-b,-c,-d,-o,-s,-x`.

**GNU-only material:** `none`.

**Option arguments:** `-A=<address_base>; -j=<skip>; -N=<count>; -t=<type_string>`.

**Operands / arity / order:** file; [+]offset[.][b]. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file;[+]offset[.][b].

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/od`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/od`; clauses `XCU:od:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [od](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/od.html).

## `paste`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
paste [-s] [-d list] file...
```

**Mandatory base options:** `-d; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-d=<list>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/paste`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/paste`; clauses `XCU:paste:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [paste](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/paste.html).

## `patch`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
patch [-blNR] [-c|-e|-n|-u] [-d dir] [-D define] [-i patchfile] [-o outfile] [-p num] [-r rejectfile] [file]
```

**Mandatory base options:** `-b; -c; -d; -D; -e; -i; -l; -n; -N; -o; -p; -R; -r; -u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-d=<dir>; -D=<define>; -i=<patchfile>; -o=<outfile>; -p=<num>; -r=<rejectfile>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; LC_TIME`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#patch`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#patch`; clauses `XCU:patch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [patch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/patch.html).

## `pathchk`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
pathchk [-p] [-P] pathname...
```

**Mandatory base options:** `-p; -P`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** pathname. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: pathname.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/pathchk`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/pathchk`; clauses `XCU:pathchk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [pathchk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pathchk.html).

## `pax`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
pax [-dv] [-c|-n] [-H|-L] [-o options] [-f archive] [-s replstr]... [pattern...]
pax -r[-c|-n] [-dikuv] [-H|-L] [-f archive] [-o options]... [-p string]... [-s replstr]... [pattern...]
pax -w [-dituvX] [-H|-L] [-b blocksize] [[-a] [-f archive]] [-o options]... [-s replstr]... [-x format] [file...]
pax -r -w [-diklntuvX] [-H|-L] [-o options]... [-p string]... [-s replstr]... [file...] directory
```

**Mandatory base options:** `-r; -w; -a; -b; -c; -d; -f; -H; -i; -k; -l; -L; -n; -o; -p; -s; -t; -u; -v; -x; -X`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-b=<blocksize>; -f=<archive>; -o=<options>; -p=<string>; -s=<replstr>; -x=<format>`.

**Operands / arity / order:** directory; file; pattern. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: directory;file;pattern.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TMPDIR; TZ`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/pax`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/pax`; clauses `XCU:pax:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [pax](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html).

## `pr`

**Requirement / applicability:** base; xsi.

**Normative POSIX synopsis:**

```text
pr [+page] [-column] [-adFmrt] [-e[char][gap]] [-h header] [-i[char][gap]] [-l lines] [-n[char][width]] [-o offset] [-s[char]] [-w width] [-fp] [file...]
```

**Mandatory base options:** `+<page>; -<column>; -a; -d; -e; -f; -F; -h; -i; -m; -n; -o; -p; -r; -s; -t; -w`.

**Conditional / optional options:** `xsi:-l`.

**GNU-only material:** `none`.

**Option arguments:** `-h=<header>; -l=<lines>; -o=<offset>; -w=<width>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/pr`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/pr`; clauses `XCU:pr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [pr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pr.html).

## `printf`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
printf format [argument...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** format; argument. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: format;argument.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:printf`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:printf`; clauses `XCU:printf:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [printf](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/printf.html).

## `ps`

**Requirement / applicability:** xsi.

**Normative POSIX synopsis:**

```text
[xsi] ps [-aA] [-defl] [-g grouplist] [-G grouplist] [-n namelist] [-o format]... [-p proclist] [-t termlist] [-u userlist] [-U userlist]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `xsi:-a,-A,-d,-e,-f,-g,-G,-l,-n,-o,-p,-t,-u,-U`.

**GNU-only material:** `none`.

**Option arguments:** `-g=<grouplist>; -G=<grouplist>; -n=<namelist>; -o=<format>; -p=<proclist>; -t=<termlist>; -u=<userlist>; -U=<userlist>`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `COLUMNS; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/ps`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/ps`; clauses `XCU:ps:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [ps](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/ps.html).

## `pwd`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
pwd [-L|-P]
```

**Mandatory base options:** `-L; -P`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_MESSAGES; NLSPATH; PWD`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:pwd`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:pwd`; clauses `XCU:pwd:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [pwd](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pwd.html).

## `read`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
read [-r] var...
```

**Mandatory base options:** `-r`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** var. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: var.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `IFS; LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; PS2`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:read`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:read`; clauses `XCU:read:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [read](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/read.html).

## `renice`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
renice [-g|-p|-u] -n increment ID...
```

**Mandatory base options:** `-g; -n; -p; -u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-n=<increment>`.

**Operands / arity / order:** ID. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: ID.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/renice`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/renice`; clauses `XCU:renice:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [renice](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/renice.html).

## `rm`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
rm [-iRr] file...
rm -f [-iRr] [file...]
```

**Mandatory base options:** `-f; -i; -R; -r`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/rm`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/rm`; clauses `XCU:rm:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [rm](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rm.html).

## `rmdir`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
rmdir [-p] dir...
```

**Mandatory base options:** `-p`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** dir. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: dir.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/rmdir`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/rmdir`; clauses `XCU:rmdir:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [rmdir](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/rmdir.html).

## `sed`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
sed [-n] script [file...]
sed [-n] -e script [-e script]... [-f script_file]... [file...]
sed [-n] [-e script]... -f script_file [-f script_file]... [file...]
```

**Mandatory base options:** `-e; -f; -n`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-e=<script>; -f=<script_file>`.

**Operands / arity / order:** file; script. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file;script.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`custom`).

**Implementation source:** `cmds/sed`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/sed`; clauses `XCU:sed:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html).

## `sh`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
sh [-abCefhimnuvx] [-o option]... [+abCefhimnuvx] [+o option]... [command_file [argument...]]
sh -c [-abCefhimnuvx] [-o option]... [+abCefhimnuvx] [+o option]... command_string [command_name [argument...]]
sh -s [-abCefhimnuvx] [-o option]... [+abCefhimnuvx] [+o option]... [argument...]
```

**Mandatory base options:** `-a; -b; -C; -e; -f; -h; -i; -m; -n; -u; -v; -x; -o; +a; +b; +C; +e; +f; +h; +i; +m; +n; +u; +v; +x; +o; -c; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-o=<option>; +o=<option>`.

**Operands / arity / order:** -; argument; command_file; command_name; command_string. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: -;argument;command_file;command_name;command_string.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `ENV; FCEDIT; HISTFILE; HISTSIZE; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; MAIL; MAILCHECK; MAILPATH; NLSPATH; PATH; PWD`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:sh`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:sh`; clauses `XCU:sh:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [sh](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sh.html).

## `sleep`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
sleep time
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** time. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: time.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/sleep`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/sleep`; clauses `XCU:sleep:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [sleep](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sleep.html).

## `sort`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
sort [-m] [-o output] [-bdfinru] [-t char] [-k keydef]... [file...]
sort [-c|-C] [-bdfinru] [-t char] [-k keydef] [file]
```

**Mandatory base options:** `-c; -C; -m; -o; -u; -d; -f; -i; -n; -r; -b; -t; -k`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-o=<output>; -t=<char>; -k=<keydef>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/sort`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/sort`; clauses `XCU:sort:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [sort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sort.html).

## `split`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
split [-l line_count] [-a suffix_length] [file [name]]
split -b n[k|m] [-a suffix_length] [file [name]]
```

**Mandatory base options:** `-a; -b; -l`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-a=<suffix_length>; -b=<n>; -l=<line_count>`.

**Operands / arity / order:** file; name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file;name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/split`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/split`; clauses `XCU:split:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [split](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/split.html).

## `strings`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
strings [-a] [-t format] [-n number] [file...]
```

**Mandatory base options:** `-a; -n; -t`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-n=<number>; -t=<format>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/strings`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/strings`; clauses `XCU:strings:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [strings](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strings.html).

## `strip`

**Requirement / applicability:** development.

**Normative POSIX synopsis:**

```text
[development] strip file...
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#strip`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#strip`; clauses `XCU:strip:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [strip](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/strip.html).

## `stty`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
stty [-a|-g]
stty operand...
```

**Mandatory base options:** `-a; -g`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** parenb (-parenb); parodd (-parodd); cs5 cs6 cs7 cs8; number; ispeed number; ospeed number; hupcl (-hupcl); hup (-hup); cstopb (-cstopb); cread (-cread); clocal (-clocal); ignbrk (-ignbrk); brkint (-brkint); ignpar (-ignpar); parmrk (-parmrk); inpck (-inpck); istrip (-istrip); inlcr (-inlcr); igncr (-igncr); icrnl (-icrnl); ixon (-ixon); ixany (-ixany); ixoff (-ixoff); opost (-opost); onlcr (-onlcr); ocrnl (-ocrnl); onocr (-onocr); onlret (-onlret); ofill (-ofill); ofdel (-ofdel); cr0 cr1 cr2 cr3; nl0 nl1; tab0 tab1 tab2 tab3; tabs (-tabs); bs0 bs1; ff0 ff1; vt0 vt1; isig (-isig); icanon (-icanon); iexten (-iexten); echo (-echo); echoe (-echoe); echok (-echok); echonl (-echonl); noflsh (-noflsh); tostop (-tostop); <control>-character string; min number; time number; saved settings; evenp or parity; oddp; -parity, -evenp, or -oddp; raw (-raw or cooked); nl (-nl); ek; sane. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: parenb (-parenb);parodd (-parodd);cs5 cs6 cs7 cs8;number;ispeed number;ospeed number;hupcl (-hupcl);hup (-hup);cstopb (-cstopb);cread (-cread);clocal (-clocal);ignbrk (-ignbrk);brkint (-brkint);ignpar (-ignpar);parmrk (-parmrk);inpck (-inpck);istrip (-istrip);inlcr (-inlcr);igncr (-igncr);icrnl (-icrnl);ixon (-ixon);ixany (-ixany);ixoff (-ixoff);opost (-opost);onlcr (-onlcr);ocrnl (-ocrnl);onocr (-onocr);onlret (-onlret);ofill (-ofill);ofdel (-ofdel);cr0 cr1 cr2 cr3;nl0 nl1;tab0 tab1 tab2 tab3;tabs (-tabs);bs0 bs1;ff0 ff1;vt0 vt1;isig (-isig);icanon (-icanon);iexten (-iexten);echo (-echo);echoe (-echoe);echok (-echok);echonl (-echonl);noflsh (-noflsh);tostop (-tostop);<control>-character string;min number;time number;saved settings;evenp or parity;oddp;-parity, -evenp, or -oddp;raw (-raw or cooked);nl (-nl);ek;sane.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`custom`).

**Implementation source:** `cmds/stty`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/stty`; clauses `XCU:stty:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [stty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/stty.html).

## `tabs`

**Requirement / applicability:** base; xsi.

**Normative POSIX synopsis:**

```text
tabs [-T type] n[[sep[+]n]...]
[xsi] tabs [-n|-a|-a2|-c|-c2|-c3|-f|-p|-s|-u] [-T type]
```

**Mandatory base options:** `-T`.

**Conditional / optional options:** `xsi:-<n>,-a,-a2,-c,-c2,-c3,-f,-p,-s,-u`.

**GNU-only material:** `none`.

**Option arguments:** `-T=<type>`.

**Operands / arity / order:** n[[sep[+]n]...]. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: n[[sep[+]n]...].

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TERM`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tabs`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tabs`; clauses `XCU:tabs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tabs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tabs.html).

## `tail`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
tail [-f] [-c number|-n number] [file]
```

**Mandatory base options:** `-c; -f; -n`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-c=<number>; -n=<number>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand as permitted by the synopsis.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tail`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tail`; clauses `XCU:tail:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tail](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tail.html).

## `talk`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] talk address [terminal]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** address; terminal. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: address;terminal.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TERM`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#talk`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#talk`; clauses `XCU:talk:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [talk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/talk.html).

## `tee`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
tee [-ai] [file...]
```

**Mandatory base options:** `-a; -i`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tee`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tee`; clauses `XCU:tee:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tee](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tee.html).

## `test`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
test [expression]
[ [expression] ]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** -b pathname; -c pathname; -d pathname; -e pathname; -f pathname; -g pathname; -h pathname; -L pathname; -n string; -p pathname; -r pathname; -S pathname; -s pathname; -t file_descriptor; -u pathname; -w pathname; -x pathname; -z string; string; s1 = s2; s1 != s2; n1 -eq n2; n1 -ne n2; n1 -gt n2; n1 -ge n2; n1 -lt n2; n1 -le n2; expression1 -a expression2; expression1 -o expression2; ! expression; ( expression ). Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: -b pathname;-c pathname;-d pathname;-e pathname;-f pathname;-g pathname;-h pathname;-L pathname;-n string;-p pathname;-r pathname;-S pathname;-s pathname;-t file_descriptor;-u pathname;-w pathname;-x pathname;-z string;string;s1 = s2;s1 != s2;n1 -eq n2;n1 -ne n2;n1 -gt n2;n1 -ge n2;n1 -lt n2;n1 -le n2;expression1 -a expression2;expression1 -o expression2;! expression;( expression ).

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 expression true; 1 expression false; greater than 1 on error.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:test`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:test`; clauses `XCU:test:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [test](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/test.html).

## `time`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
time [-p] utility [argument...]
```

**Mandatory base options:** `-p`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** utility; argument. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: utility;argument.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_NUMERIC; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_keyword`).

**Implementation source:** `shell:time`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:time`; clauses `XCU:time:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [time](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/time.html).

## `touch`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
touch [-acm] [-r ref_file|-t time|-d date_time] file...
```

**Mandatory base options:** `-a; -c; -d; -m; -r; -t`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-d=<date_time>; -r=<ref_file>; -t=<time>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TZ`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/touch`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/touch`; clauses `XCU:touch:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [touch](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/touch.html).

## `tput`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
tput [-T type] operand...
```

**Mandatory base options:** `-T`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-T=<type>`.

**Operands / arity / order:** clear; init; reset. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: clear;init;reset.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH; TERM`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tput`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tput`; clauses `XCU:tput:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tput](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tput.html).

## `tr`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
tr [-c|-C] [-s] string1 string2
tr -s [-c|-C] string1
tr -d [-c|-C] string1
tr -ds [-c|-C] string1 string2
```

**Mandatory base options:** `-c; -C; -d; -s`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** string1, string2. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: string1, string2.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tr`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tr`; clauses `XCU:tr:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tr](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tr.html).

## `true`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
true
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `none`.

**Output / effects:** Write the specified result to standard output; diagnostics use standard error. Effects classification: `stdout_or_diagnostic`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: Always zero.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:true`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:true`; clauses `XCU:true:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [true](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/true.html).

## `tsort`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
tsort [file]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tsort`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tsort`; clauses `XCU:tsort:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tsort](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tsort.html).

## `tty`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
tty
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/tty`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/tty`; clauses `XCU:tty:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [tty](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/tty.html).

## `umask`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
umask [-S] [mask]
```

**Mandatory base options:** `-S`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** mask. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: mask.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:umask`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:umask`; clauses `XCU:umask:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [umask](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/umask.html).

## `unalias`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
unalias alias-name...
unalias -a
```

**Mandatory base options:** `-a`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** alias-name. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: alias-name.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:unalias`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:unalias`; clauses `XCU:unalias:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [unalias](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unalias.html).

## `uname`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
uname [-amnrsv]
```

**Mandatory base options:** `-a; -m; -n; -r; -s; -v`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** none. No operands are specified; use exactly the option-only or operand-free synopsis.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/uname`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/uname`; clauses `XCU:uname:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [uname](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uname.html).

## `unexpand`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
unexpand [-a|-t tablist] [file...]
```

**Mandatory base options:** `-a; -t`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-t=<tablist>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/unexpand`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/unexpand`; clauses `XCU:unexpand:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [unexpand](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/unexpand.html).

## `uniq`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
uniq [-c|-d|-u] [-f fields] [-s char] [input_file [output_file]]
```

**Mandatory base options:** `-c; -d; -f; -s; -u`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-f=<fields>; -s=<chars>`.

**Operands / arity / order:** input_file; output_file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: input_file;output_file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/uniq`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/uniq`; clauses `XCU:uniq:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [uniq](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uniq.html).

## `uudecode`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
uudecode [-o outfile] [file]
```

**Mandatory base options:** `-o`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `-o=<outfile>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/uudecode`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/uudecode`; clauses `XCU:uudecode:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [uudecode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uudecode.html).

## `uuencode`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
uuencode [-m] [file] decode_pathname
```

**Mandatory base options:** `-m`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** decode_pathname; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: decode_pathname;file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/uuencode`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/uuencode`; clauses `XCU:uuencode:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [uuencode](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/uuencode.html).

## `vi`

**Requirement / applicability:** optional.

**Normative POSIX synopsis:**

```text
[optional] vi [-rR] [-c command] [-t tagstring] [-w size] [file...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `optional:-c,-r,-R,-t,-w`.

**GNU-only material:** `none`.

**Option arguments:** `-c=<command>; -t=<tagstring>; -w=<size>`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `COLUMNS; EXINIT; HOME; LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; LINES; NLSPATH; PATH; SHELL; TERM`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `filesystem`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** pinned external provider.

**Effective Profile C/D owner:** pinned external provider (`external`).

**Implementation source:** `pkg/posixprovider/manifest.tsv#vi`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:pkg/posixprovider/manifest.tsv#vi`; clauses `XCU:vi:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [vi](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/vi.html).

## `wait`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
wait [pid...]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** pid. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: pid.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** shell-only interface.

**Effective Profile C/D owner:** shell (`shell_builtin`).

**Implementation source:** `shell:wait`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:SHELL:wait`; clauses `XCU:wait:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [wait](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wait.html).

## `wc`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
wc [-c|-m] [-lw] [file...]
```

**Mandatory base options:** `-c; -l; -m; -w`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: file.

**Special `-` / `--` / standard input:** -:select standard input in a file-operand position; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input for a '-' file operand and when no file operand is supplied.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/wc`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/wc`; clauses `XCU:wc:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [wc](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/wc.html).

## `who`

**Requirement / applicability:** base; xsi.

**Normative POSIX synopsis:**

```text
who -q [file]
who am i
who am I
[xsi] who [-mTu] [-abdHlprt] [file]
[xsi] who [-mu] -s [-bHlprt] [file]
```

**Mandatory base options:** `-q`.

**Conditional / optional options:** `xsi:-a,-b,-d,-H,-l,-m,-p,-r,-s,-t,-T,-u`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** am i, am I; file. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: am i, am I;file.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; LC_TIME; NLSPATH; TZ`.

**Output / effects:** Write the required result format to standard output. Effects classification: `stdout`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/who`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/who`; clauses `XCU:who:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [who](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/who.html).

## `write`

**Requirement / applicability:** base.

**Normative POSIX synopsis:**

```text
write user_name [terminal]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `none`.

**GNU-only material:** `none`.

**Option arguments:** `none`.

**Operands / arity / order:** user_name; terminal. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: user_name;terminal.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. No command-specific standard-input use; the POSIX STDIN clause remains authoritative.

**Environment:** `LANG; LC_ALL; LC_CTYPE; LC_MESSAGES; NLSPATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `terminal_or_peer_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`flagset`).

**Implementation source:** `cmds/write`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/write`; clauses `XCU:write:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [write](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/write.html).

## `xargs`

**Requirement / applicability:** xsi.

**Normative POSIX synopsis:**

```text
[xsi] xargs [-ptx] [-E eofstr] [-I replstr|-L number|-n number] [-s size] [utility [argument...]]
```

**Mandatory base options:** `none`.

**Conditional / optional options:** `xsi:-E,-I,-L,-n,-p,-s,-t,-x`.

**GNU-only material:** `none`.

**Option arguments:** `-E=<eofstr>; -I=<replstr>; -L=<number>; -n=<number>; -s=<size>`.

**Operands / arity / order:** utility; argument. Use operands in the order and cardinality shown by each applicable synopsis; defined operand forms: utility;argument.

**Special `-` / `--` / standard input:** -:no command-specific operand meaning beyond the displayed synopsis; --:end options where POSIX Utility Syntax Guideline 10 applies. Read standard input when the applicable synopsis supplies no input-file operand.

**Environment:** `LANG; LC_ALL; LC_COLLATE; LC_CTYPE; LC_MESSAGES; NLSPATH; PATH`.

**Output / effects:** Produce the files, state changes, terminal/peer actions, or other effects specified by POSIX. Effects classification: `process_or_shell_state`.

**Diagnostics / status:** Write diagnostics required by POSIX to standard error. Exit status: 0 on successful completion; greater than 0 on error, except where POSIX defines command-specific values.

**Availability:** Go same-name applet.

**Effective Profile C/D owner:** Go (`manual`).

**Implementation source:** `cmds/xargs`.

**Tests / evidence / state:** `scripts/posix_manifest_test.py;repository evidence named by evidence_ids`; `REPO:cmds/xargs`; clauses `XCU:xargs:SYNOPSIS,OPTIONS,OPERANDS,ENVIRONMENT_VARIABLES,STDIN,INPUT_FILES,STDOUT,STDERR,OUTPUT_FILES,EXIT_STATUS,CONSEQUENCES_OF_ERRORS`; state `specified`.

**Official Open Group Issue 7/2016 link:** [xargs](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/xargs.html).
