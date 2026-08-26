# pax list-mode `-v` hard-link output audit (Sprint 79 issue 775)

Scope: POSIX Issue 7 (2016 Edition) STDOUT specification for `pax` in list mode with verbose (`-v`) output for archive members representing hard links to previously listed members.

The authoritative reference is the Open Group `pax` STDOUT specification:
<https://pubs.opengroup.org/onlinepubs/9699919799/utilities/pax.html#tag_20_92_10>

## POSIX Specification Requirements

POSIX Issue 7 STDOUT section specifies that in list mode when `-v` is specified without `-o listopt=format`:
- For pathnames representing hard links to previous members of the archive:
  `"%s == %s\n", <ls -l listing>, <linkname>`
- For all other pathnames:
  `"%s\n", <ls -l listing>`

where `<ls -l listing>` is the format specified by `ls -l`. For symbolic links, `<ls -l listing>` ends with `pathname -> linkname`. For hard links, `<ls -l listing>` ends with `pathname` followed by ` == linkname`.

When `-o listopt=format` is specified in verbose list mode (`-v`), output format is governed by the `listopt` format specification.

## Implemented and Verified Behavior

1. **Verbose List-Mode (`-v`) Output**:
   - For regular files, directories, fifos, special device files: outputs standard `ls -l` representation (`<mode> <nlink> <owner> <group> <size> <mtime> <name>`).
   - For symbolic links (`tar.TypeSymlink`): outputs `<ls -l listing> -> <target>`.
   - For hard links (`tar.TypeLink`): outputs `<ls -l listing> == <target>`.

2. **Name Substitutions & Interactivity**:
   - Both target name (`linkTarget`) and member name (`name`) reflect `-s` substitution and `-i` interactive rename transformations.
   - Ordering: target-before-hardlink and hardlink-before-target are handled seamlessly.

3. **Custom `listopt` Interactions**:
   - Custom `-o listopt=format` with `-v` applies the explicit user format string (e.g., `%F`, `%L`, `%(linkname)s`).

4. **Write Error Handling**:
   - Output write failures in list mode report `"pax: write error: ..."` to standard error and exit with status 1.
