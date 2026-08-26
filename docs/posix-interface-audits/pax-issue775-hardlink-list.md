# pax verbose list-output audit (Sprint 79 issues 775 and 778)

Scope: POSIX Issue 7 (2016 Edition) STDOUT specification for `pax` verbose (`-v`) list mode, including link fields and the inherited `ls -l` timestamp shape.

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

## Implemented and focused-tested behavior

1. **Verbose List-Mode (`-v`) Output**:
   - The existing long-list layout is `<mode> <nlink> <owner> <group> <size> <mtime> <name>`.
   - Link count uses carried `SCHILY.nlink` or cpio `c_nlink` metadata when present. A tar hard-link member without carried metadata uses the interoperable fallback 2; other members fall back to 1.
   - A symbolic-link size is the byte length of its effective displayed target, including `-s` and interactive target renames, rather than the normally-zero tar header body size.
   - For symbolic links (`tar.TypeSymlink`): outputs `<ls -l listing> -> <target>`.
   - For hard links (`tar.TypeLink`): outputs `<ls -l listing> == <target>`.

2. **Name Substitutions & Interactivity**:
   - Selection, substitution, and all interactive decisions are precomputed before output. Both target name (`linkTarget`) and member name (`name`) therefore reflect `-s` and `-i` transformations even when a link precedes its target.
   - Link-target resolution retains archive-member occurrence identity: the latest preceding occurrence is preferred, with the first later occurrence used for a link-before-target archive. A later duplicate raw pathname or a distinct pathname colliding after `-s` cannot overwrite the effective name of the occurrence actually referenced by an earlier hard link.
   - A skipped target supplies no effective interactive rename. An interactive EOF/error fails before partial listing output.

3. **Custom `listopt` Interactions**:
   - Custom `-o listopt=format` with `-v` applies the explicit user format string (e.g., `%F`, `%L`, `%(path)s`, `%(linkpath)s`, `%(linkname)s`). Effective path/linkpath values are synchronized into copied PAX records so extended headers cannot override transformed values; archive size semantics remain unchanged for explicit list formats.

4. **Write Error Handling**:
   - Output write failures in the ordinary, hard-link, and custom-listopt branches report `"pax: write error: ..."` to standard error and exit with status 1.

## Timestamp residual closure (issue 778)

The default verbose timestamp now follows the POSIX `ls -l` shape inherited by
`pax -v`: `%b %e %H:%M` for a modification time strictly within the last six
months, and `%b %e  %Y` at the exact six-month boundary, for older times, and
for every future time. The duration is the same half of 365.2425 days used by
this repository's GNU-compatible `ls`; black-box GNU `ls` 9.11 probes on both
sides of that cutoff select the corresponding time and year forms. An
invocation-local clock is sampled once per listing, so every member is
classified against the same instant and boundary tests do not depend on
wall-clock timing. `TestPAXVerboseTimestampAgeBoundaries` covers now, both sides
of the cutoff, the exact cutoff, and the future case through the verbose list
command surface.
