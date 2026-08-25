# POSIX `more` interface closure (issue 720)

This audit uses the Open Group POSIX.1-2016 Issue 7 `more` utility page as the
normative reference. It supersedes the narrow `more` assessment in batch 6C;
no generated manifest, aggregate count, or shared command matrix is changed by
this issue.

## Closed interface

- Parses the Issue 7 `-c`, `-e`, `-i`, `-n number`, `-p command`, `-s`, `-u`,
  and optional XSI `-t tagstring` forms. A positive `-n` overrides `LINES` and
  terminal geometry; `COLUMNS` supplies display width.
- Honors the required nonterminal rule: only `-s` changes copied content.
- Pages an input incrementally, folds display rows at the selected width, and
  applies the specified CR, width-aware backspace/overstrike, and visibly
  rendered `-u` control-character behavior. Folded rows retain exact source
  byte boundaries for position reports.
- Implements the Issue 7 command grammar, including numeric prefixes,
  line/screen/half-screen movement, searches and repeat/reverse/inversion,
  marks, refresh/re-read, file navigation, tag navigation, position reports,
  help, editor handoff, and all quit spellings.
- Runs `-p` commands for every new or redisplayed file, after `-t` positioning,
  suppresses intermediate screens, and stops that file's command list after an
  informational command failure.
- Uses `MORE`, `LINES`, `COLUMNS`, `EDITOR`, and `TERM`; `TERM=dumb` avoids ANSI
  clearing and highlighting. Editor launch is the sole permitted external
  process and `vi`/`ex` receive the current line with `-c`.
- Resolves `LC_CTYPE` and `LC_COLLATE` with POSIX precedence. C/POSIX uses
  byte-oriented folding, classes, ranges, and ASCII case folding;
  de_DE.ISO-8859-1 uses the reviewed pure-Go/glibc providers; UTF-8 locales use
  glyph-aware folding and Unicode literal/case matching. Locale-sensitive
  UTF-8 bracket expressions fail closed because no reviewed UTF-8 collation
  provider is carried.
- Keeps file bytes on standard output and prompts/diagnostics on the terminal
  channel. Read, write, flush, close, cancellation, and unavailable-terminal
  errors are covered by hermetic seams.

The focused evidence is in `cmds/more/more_test.go`,
`cmds/more/pager_test.go`, `cmds/more/posix_interactive_test.go`, and the
platform-specific controlling-terminal tests.

## Truthful portability boundaries

- `:e` uses the in-process POSIX shell parser and expansion engine for quoting,
  tilde/parameter/arithmetic expansion, field splitting, pathname expansion,
  and command substitution. Command substitutions may use shell builtins; an
  external utility is rejected so this interface cannot evade the pure-Go/no
  process-launch boundary. POSIX leaves multiple resulting pathnames
  unspecified; this implementation diagnoses them.
- Diagnostics are invariant English strings; translated `LC_MESSAGES` and
  `NLSPATH` catalogs are not shipped.
- Historical underline/bold overstrikes are normalized to their displayed
  glyph. The implementation does not recreate terminal-specific visual
  attributes when no portable terminal capability API is available.
- Tag lookup supports the standard numeric and search-pattern ctags addresses
  in the local `tags` file. Implementation-specific ctags extensions are not a
  POSIX claim.

These boundaries do not hide parser or state-machine gaps in the Issue 7
command interface; they identify locale, terminal-capability, and expansion
facilities that cannot be made fully portable in pure Go.
