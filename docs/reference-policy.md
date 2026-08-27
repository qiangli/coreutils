# Command behavior reference policy

This repository contains more than GNU Coreutils.  The phrase "Bashy
coreutils" describes the assembled portable userland; it does **not** make GNU
Coreutils the upstream project for every command in that userland.  This page
defines which reference controls a command's behavior and how conflicts are
resolved.

## Reference precedence

Use these authorities in order:

1. **Certification baseline — POSIX.1 Issue 7, 2016 Edition.** For a command,
   option, operand, environment variable, exit status, or output requirement
   exercised by the configured VSC-PCTS2016 POSIX08 campaign, the normative
   reference is [The Open Group Base Specifications Issue 7, 2016
   Edition](https://pubs.opengroup.org/onlinepubs/9699919799/). In POSIX mode,
   POSIX behavior wins if an upstream extension conflicts with it.
2. **The command's own official upstream documentation.** This controls
   behavior outside POSIX and behavior POSIX leaves unspecified. The upstream
   project and version must match the command family; they must not be inferred
   from the name of this repository.
3. **The pinned Profile A provider.** The Ubuntu 24.04 Profile A executable is
   a differential-test control and useful empirical evidence. It does not
   override POSIX or the command's official upstream documentation.
4. **VSC-PCTS results.** The suite validates the implementation against the
   certification baseline; it is not the specification. A conflict between a
   test and the standard is handled through a Problem Report, not by silently
   changing command semantics.
5. **Permissively licensed prior art.** Prior-art implementations can inform or
   supply code when their licenses permit it, but they are never behavioral
   authorities. GNU/GPL source is not ported or translated.

## Command-family references

| Command family | POSIX behavior | Extension and non-POSIX behavior |
|---|---|---|
| Commands shipped by GNU Coreutils | POSIX.1 Issue 7, 2016 Edition when standardized | [GNU Coreutils 9.11 manual](https://www.gnu.org/software/coreutils/manual/html_node/index.html) and GNU Coreutils 9.11 runtime |
| `ps` | [POSIX `ps`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/ps.html) | [procps-ng 4.0.4](https://gitlab.com/procps-ng/procps) documentation and runtime, matching Ubuntu 24.04; `ps` is not a GNU Coreutils command |
| Other standardized Go applets, including `awk`, `diff`, `ed`, `grep`, `mailx`, `sed`, and `talk` | POSIX.1 Issue 7, 2016 Edition | The official documentation and explicitly recorded version of the command's actual upstream family only for extensions the applet claims to support; local-only mailx and other incomplete surfaces remain explicitly partial (POSIX talk itself is local-host scoped) |
| Active pinned external providers: `m4`, `man`, `ctags`, `ar`, `nm`, `strip`, `ex`, `vi`, `lp`, and `localedef` | POSIX.1 Issue 7, 2016 Edition | The exact upstream project, version, artifact, and digest in [`pkg/posixprovider/manifest.tsv`](../pkg/posixprovider/manifest.tsv); `make`, `bc`, `ed`, `patch`, `mail`/`mailx`, and `talk` are exclusively pure-Go applets |
| Bashy/AgentOS-only commands | Not applicable unless explicitly documented | The command's repository-owned contract and tests; no POSIX or GNU compatibility is implied |

## Deferred GNU Coreutils 9.11 compatibility objective

Complete behavioral compatibility with **GNU Coreutils 9.11** is deferred to a
future campaign. It is not an active Sprint 79 requirement and is not a
prerequisite for POSIX certification. Existing supported GNU extensions still
use the GNU 9.11 manual/runtime as their authority and must not regress or be
silently approximated.

This target is not a present-tense blanket conformance claim. Documentation
must distinguish among:

- **targeted** — GNU Coreutils 9.11 is the intended reference;
- **compatible subset** — the documented and tested subset matches 9.11, while
  unsupported behavior still fails clearly; and
- **verified compatible** — the complete applicable 9.11 interface has passed
  the repository's focused, differential, and cross-platform gates.

Until a future campaign supplies complete evidence, describe the repository as
“GNU Coreutils 9.11-compatible where implemented,” not as pursuing or providing
a complete GNU Coreutils 9.11 replacement. VSC-PCTS proves the POSIX
certification surface; by itself it cannot establish GNU extension
compatibility. This deferred objective does not apply to commands GNU
Coreutils does not ship.

The [generated applet matrix](applet-matrix.md) identifies the family of each
advertised name. “GNU Coreutils” in that matrix means that the command belongs
to GNU Coreutils; it does not claim complete option coverage. For a non-GNU
family, a change that adds an extension must cite the exact official upstream
and version in its documentation or tests. If no extension reference has been
recorded, only the documented POSIX subset is claimed.

## POSIX mode and conflicts

The certification Profiles C/D run the assembled shell and all commands with
`POSIXLY_CORRECT=1`. Commands must therefore choose their POSIX behavior for
every interface affected by that environment variable. A command that has an
additional command-specific POSIX switch must honor it as well; the global
environment does not excuse omitting an upstream-required switch.

Outside POSIX mode, a command may expose compatible upstream extensions. Such
extensions must preserve their official spelling, defaults, interactions,
output, and exit status. An unsupported option must fail clearly; it must not be
accepted with an approximation.

When two references appear to disagree:

1. reduce the case to a focused argv/environment/stdin fixture;
2. identify whether it is POSIX-defined, implementation-defined, unspecified,
   or an extension;
3. cite the applicable standard clause and exact upstream version;
4. compare the pinned Profile A executable as evidence; and
5. record any intentional deviation next to the implementation and its tests.

## Change evidence

A change that adds or alters command behavior should include:

- the applicable POSIX utility or Base Definitions reference, when any;
- the actual upstream project and version for extensions;
- focused tests covering stdout, stderr, exit status, and filesystem effects;
- a GNU Coreutils 9.11 differential only when the command is part of GNU
  Coreutils; and
- a Profile A differential when behavior depends on the certification image.

Do not describe `ps`, `sed`, `grep`, `awk`, `make`, or another unrelated
utility as “based on GNU Coreutils 9.11.” Use the exact family name. “GNU/system
reference” is acceptable only as a label for the mixed Profile A control, not
as an implementation specification.
