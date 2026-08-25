# file Issue 746 POSIX.1-2008 Issue 7 Audit

Scope: The Open Group POSIX.1-2008 Issue 7, 2016 Edition `file` utility. This audit covers the Bashy Go-owned implementation in `cmds/file` only, not GNU 9.11 compatibility.

## Evidence Map

| Clause / behavior | Source evidence | Test evidence |
| --- | --- | --- |
| Required synopsis and operands | `cmds/file/file.go:33`, `cmds/file/file.go:82-100` register required `-d`, `-h`, `-i`, `-M`, and `-m`, and reject missing operands or `-i` with magic/default options. | `cmds/file/issue7_test.go:76`, `cmds/file/file_test.go:245`, `cmds/file/issue7_test.go:88` |
| `-d`, `-m`, `-M` option arguments, attached/separate forms, repetition, interleaving, and ordering | `cmds/file/file.go:50-77` records source options in parse order; `cmds/file/magic.go:71-107` constructs ordered plans; `cmds/file/magic.go:142-165` evaluates them in order with deferred context defaults. | `cmds/file/file_test.go:202`, `cmds/file/file_test.go:213`, `cmds/file/issue7_test.go:103` |
| `-h` versus default link following | `cmds/file/file.go:85-87` registers default dereference extension and POSIX `-h`; `cmds/file/file.go:151-166` uses `Lstat` then follows unless `-h` is active, while dangling links are identified as links. | `cmds/file/file_test.go:144`, `cmds/file/file_test.go:181`, `cmds/file/file_test.go:259` |
| `-i` minimal identification | `cmds/file/file.go:87`, `cmds/file/file.go:99-100`, `cmds/file/file.go:140-142`, `cmds/file/file.go:180-181` implement regular-file-only classification and disallow magic/default options. | `cmds/file/file_test.go:245`, `cmds/file/file_test.go:259`, `cmds/file/file_test.go:290` |
| `--` parsing only where supported | Shared parser support is used through `tool.Parse` at `cmds/file/file.go:92`; no command-specific long-option emulation is added. | `cmds/file/issue7_test.go:136`, `cmds/file/file_test.go:790` |
| Stdin operand `-` | `cmds/file/file.go:139-149` treats `-` as standard input for content classification and as regular file under `-i`, without reading stdin in `-i`. | `cmds/file/issue7_test.go:59`, `cmds/file/file_test.go:101`, `cmds/file/file_test.go:259`, `cmds/file/file_test.go:290` |
| Default position and context tests | `cmds/file/magic.go:24-30`, `cmds/file/magic.go:83-87`, `cmds/file/magic.go:110-135`, `cmds/file/magic.go:142-165`, `cmds/file/file.go:388-550` implement empty, known binary signatures, program/text/data classification, bounded reads, and UTF-8 text conditional on locale. | `cmds/file/file_test.go:101`, `cmds/file/file_test.go:118`, `cmds/file/file_test.go:298`, `cmds/file/file_test.go:331`, `cmds/file/file_test.go:382`, `cmds/file/file_test.go:580`, `cmds/file/file_test.go:771` |
| Custom magic grammar: ordering, continuations, masks/operators, string and numeric types, substitutions | `cmds/file/magic.go:168-188` loads magic files; `cmds/file/magic.go:200-249` parses lines; `cmds/file/magic.go:298-351` implements string, byte/short/long aliases, signed/unsigned widths, and masks; `cmds/file/magic.go:449-520` applies primary/continuation tests and operators; `cmds/file/magic.go:557-888` renders printf-like messages and conversions. | `cmds/file/file_test.go:490`, `cmds/file/file_test.go:521`, `cmds/file/file_test.go:535`, `cmds/file/file_test.go:603`, `cmds/file/file_test.go:626`, `cmds/file/file_test.go:649`, `cmds/file/file_test.go:703`, `cmds/file/file_test.go:725`, `cmds/file/file_test.go:747` |
| Special-file and stat classification | `cmds/file/file.go:151-179` classifies directories, FIFOs, sockets, devices, and other non-regular special files before content reads; device wording is supplied by `cmds/file/file_unix.go:13` and `cmds/file/file_other.go:7`. | `cmds/file/issue7_test.go:26`, `cmds/file/file_test.go:144`, `cmds/file/file_unix_test.go:14` |
| Locale and environment behavior | `cmds/file/file.go:521-525` resolves `LC_CTYPE` through the invocation `RunContext` and affects UTF-8 text recognition without process-global locale mutation. | `cmds/file/file_test.go:580` |
| Exact stdout shape and operand ordering | `cmds/file/file.go:108-134` emits exactly one line per operand in operand order using the required `<operand>: <type>\n` shape. | `cmds/file/issue7_test.go:26`, `cmds/file/file_test.go:790` |
| Inaccessible or undetermined operands | `cmds/file/file.go:112-123` turns stat/open/read/close failures into `cannot open` stdout classifications and preserves status 0; magic rendering errors remain diagnostics with status 1. | `cmds/file/issue7_test.go:26`, `cmds/file/issue7_test.go:88`, `cmds/file/file_test.go:428`, `cmds/file/file_test.go:478`, `cmds/file/file_test.go:747`, `cmds/file/file_test.go:790` |
| Diagnostics and aggregate exit status | `cmds/file/file.go:96-106`, `cmds/file/file.go:112-123`, `cmds/file/file.go:131-136` return usage status for syntax, status 1 for magic/load/output failures, status 0 for inaccessible operands, and continue across per-operand nonfatal classifications. | `cmds/file/issue7_test.go:26`, `cmds/file/issue7_test.go:76`, `cmds/file/file_test.go:454`, `cmds/file/file_test.go:556`, `cmds/file/file_test.go:747`, `cmds/file/file_test.go:790` |
| Output errors and short writes | `cmds/file/file.go:131-133` checks every stdout write; `cmds/file/file.go:380-386` converts short writes to `io.ErrShortWrite`. | `cmds/file/file_test.go:454` |

## Changes Made

- No mandatory behavior defect was confirmed. The initially proposed `-b`
  inaccessible-output change was rejected during manager review: Issue 7 does
  not specify a `-b` option, and the existing extension already includes the
  inaccessible operand in its `cannot open` type text.
- Added command-local executable tests for separate required and attached
  extension forms of `-m`/`-M`, repeated and interspersed source-order
  precedence, and supported `--` parsing.

## Residuals

- The `-` operand is implementation-defined by POSIX. Bashy intentionally treats it as standard input for content classification and as a regular file under `-i`; this is pinned by tests but not claimed as the only conforming behavior.
- `-b` and `-L` are non-conflicting Bashy extensions and do not form part of
  the Issue 7 conformance claim.
- Type strings beyond required categories are implementation-defined. The audit verifies mandatory classes and Bashy's deterministic wording, not GNU/libmagic parity.
- Device major/minor formatting is platform-specific. Unix coverage uses `/dev/null`; non-Unix builds fall back to generic device wording.
- Locale coverage is limited to `LC_CTYPE` behavior visible through `RunContext`; message localization (`LC_MESSAGES`, `NLSPATH`) is not implemented by this Go command.
- Magic grammar support is scoped to POSIX-required portable forms plus documented non-conflicting extensions in the implementation. Unsupported magic syntax fails with diagnostics instead of silent approximation.
