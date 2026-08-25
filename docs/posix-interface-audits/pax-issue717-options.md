# pax `-o` POSIX Issue 7 audit (Sprint 79 issue 717)

Scope: the normative `pax -o options` language in POSIX Issue 7, 2016 Edition,
including List Mode Format Specifications and extended-header keyword
precedence.  The authoritative reference is the Open Group `pax` utility
page:

<https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/pax.html>

## Implemented and tested

- Option arguments accept leading keyword whitespace, escaped commas, an
  ignored final comma, and repeated options in command-line order.
  `listopt=` consumes the rest of its option argument, and repeated list
  formats concatenate. Malformed assignments fail before archive I/O.
- `delete=` patterns are additive and remove matching physical global and
  per-file records before Go's tar decoder can fold them into header fields.
- `exthdr.name=` and `globexthdr.name=` implement the specified substitutions
  and defaults. Inapplicable write formats and modes fail closed.
- `invalid=binary|bypass|rename|UTF-8|write` control flow, `linkdata`, and
  `times` have mode-specific paths. Locale decisions use only
  `RunContext.Env`; no process
  environment is mutated. In list mode binary/rename/write follow bypass;
  write-mode binary marks untranslatable names, owners, groups, and extended
  strings with `hdrcharset=BINARY`. Interactive rename uses the existing
  `/dev/tty` lane.
- Global `keyword=value`, per-file `keyword:=value`, archive global records,
  archive per-file records, empty per-file deletion, and `delete=` follow the
  required precedence. Ordered name/ID application is deterministic.
- Copy mode uses the same extended-header reader/writer path. Direct `-l`
  remains direct for pathname-only changes, but falls back to material copy
  when an ownership/time/size/link override cannot coexist with a hard link
  without mutating the source inode.
- List formatting implements standard printf string/integer/float
  conversions, portable escapes, and POSIX `T`, `M`, `D`, `F`, and `L`.
  USTAR field names, the required `c_*` cpio field names, extended-header
  keywords (including binary `c_filedata`), repeated-format concatenation,
  `TZ`, and malformed conversions are covered. `%T` supports the complete
  Issue 7 `date` operand conversion set and uses the carried invocation-local
  `LC_TIME` names/encoding. `listopt` supplies the verbose table format when
  `-v` is present; without `-v`, list mode retains the pathname listing.
  Raw USTAR `name`, `prefix`, `magic`, `version`, and `chksum` values are
  retained exactly through list decoding rather than reconstructed from the
  normalized header; their private transport metadata is not a user keyword.
- Input format enforcement distinguishes cpio from tar. A minimal pax stream
  is physically indistinguishable from ustar when no extended record is
  needed, so compatible tar input is accepted rather than falsely rejected.

## Bounded implementation note

The invalid-value implementation provides exact `C`/`POSIX`, UTF-8,
ISO-8859-1, and ISO-8859-15 encodability decisions through `RunContext.Env`;
unknown locale names fail closed for non-ASCII values. Time formatting uses
the same bounded provider and fails before list output for unsupported
`LC_TIME`. It does not claim conversion support for other unadvertised legacy
host charmaps. Raw cpio file data is losslessly base64-carried only through the
private normalization lane and decoded for the normative `c_filedata` field;
it is not advertised as a pax extended-header interface.

The remaining invalid-value blocker is byte transcoding, not encodability
detection: archive UTF-8 text is not yet converted to the selected carried
ISO-8859-1/15 destination bytes, and local ISO bytes are not yet converted to
UTF-8 on write. `invalid=rename` also still needs field-specific replacement
for an invalid link-name value rather than only the member pathname. Until
those are implemented and byte-exactly tested, this audit does not classify
the full `invalid=` interface as closed.

All production archive parsing, rewriting, formatting, and copying is pure Go
and performs no shell-outs.
