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
   - The existing long-list layout is `<mode> <nlink> <owner> <group> <size> <mtime> <name>`.
   - Link count uses carried `SCHILY.nlink` or cpio `c_nlink` metadata when present. A tar hard-link member without carried metadata uses the interoperable fallback 2; other members fall back to 1.
   - A symbolic-link size is the byte length of its effective displayed target, including `-s` and interactive target renames, rather than the normally-zero tar header body size.
   - For symbolic links (`tar.TypeSymlink`): outputs `<ls -l listing> -> <target>`.
   - For hard links (`tar.TypeLink`): outputs `<ls -l listing> == <target>`.

2. **Name Substitutions & Interactivity**:
   - Selection, substitution, and all interactive decisions are precomputed before output. Both target name (`linkTarget`) and member name (`name`) therefore reflect `-s` and `-i` transformations even when a link precedes its target.
   - A skipped target is not added to the rename map. An interactive EOF/error fails before partial listing output.

3. **Custom `listopt` Interactions**:
   - Custom `-o listopt=format` with `-v` applies the explicit user format string (e.g., `%F`, `%L`, `%(path)s`, `%(linkpath)s`, `%(linkname)s`). Effective path/linkpath values are synchronized into copied PAX records so extended headers cannot override transformed values; archive size semantics remain unchanged for explicit list formats.

4. **Write Error Handling**:
   - Output write failures in the ordinary, hard-link, and custom-listopt branches report `"pax: write error: ..."` to standard error and exit with status 1.

## Explicit residual

The default verbose timestamp remains the pre-existing fixed `%b %e %H:%M`
rendering. It does not yet switch to the year form for sufficiently old or
future timestamps as an `ls -l` implementation does. This issue does not claim
closure of that separate long-list residual.
