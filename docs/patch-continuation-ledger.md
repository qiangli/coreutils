# `patch` continuation ledger

The pure-Go `patch` applet is the default multicall owner. The pinned external
provider remains a differential control and can be built or checked explicitly;
it is not part of normal command dispatch.

## Implemented lanes

- Unified, context, and normal diff parsing and application.
- Multi-file unified and context patches, including file creation and deletion.
- Forward and explicit reverse application, offset search, bounded context fuzz,
  already-applied detection, and reject-file output.
- Header path selection and stripping, explicit input/output/reject paths,
  directory selection, backups, empty-file removal, quiet mode, whitespace-aware
  matching, and dry runs.
- Atomic replacement of existing files while preserving their permission bits.

The command and package tests cover parsing, all three textual formats, round
trips, path stripping, reverse application, creates/deletes, rejects, backups,
multi-file patches, missing final newlines, drift/fuzz, and unsupported binary
sections.

## Deliberate gaps

- `-e` ed-script input and `-D` ifdef merging are accepted but fail closed.
- RCS/SCCS retrieval (`-g`) is accepted but fails closed.
- Binary patches and rename-only Git patches are reported, never silently
  treated as applied.
- Reject files are emitted in unified form.
- Fuzz and whitespace matching are intentionally narrower than GNU `patch`.

These gaps keep the interface evidence state at **partial**. Promotion requires
the Profile D VSC replay and explicit evidence for every remaining POSIX lane;
this ledger does not claim certification.
