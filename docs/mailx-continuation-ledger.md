# `mail` / `mailx` continuation ledger

The pure-Go `mailx` applet, also registered as `mail`, is the Profile C/D
runtime owner. It delivers only to local mbox files. SMTP, IMAP, remote address
routing, and an external MTA are deliberately out of scope; remote-qualified
addresses fail before any mailbox is changed. `mail` and `mailx` have no
external-provider definition, build recipe, cache entry, or fallback.

## Implemented lanes

- Local multi-recipient send mode with subject, local sender, Cc/Bcc, header
  recipients, and `-F` local recording.
- Default, `-u`, and `-f [file]` mailbox selection; `-e`, `-H`, and `-N`.
- Strict mbox parsing, symmetric `From` quoting, locked append, and
  transactional delete-on-quit that preserves concurrently appended mail.
- The complete Issue 7 receive command vocabulary, minimum abbreviations,
  message selectors and disposition state machine.
- Startup files, aliases, variables, conditionals, replies/followups,
  composition escapes, paging/editing, signals, and dead-letter handling.
- Invocation-wide output/error tracking plus checked local-file writes.
- Refusal of symlink and non-regular mailbox targets.

## 2026-08-28 Profile D repair

- A msglist word containing a `<hyphen>` is only the `n-m` range form when
  both sides are numeric; a hyphenated login is matched as an address, as
  Issue 7 requires of any address shown in a header summary.
- `retain` and `discard` keep independent lists, so a discard can no longer
  withdraw a retained header-field and a non-empty retained list still wins
  whole.
- `crt` set to null keeps pagination enabled at the screenful size, the
  implementation-defined value Issue 7 permits.
- `~i` expands the `sign`/`Sign` escapes so it agrees with `~a`/`~A`.

See [`docs/posix-interface-audits/profile-d-mailx-patch-repair-2026-08-28.md`](posix-interface-audits/profile-d-mailx-patch-repair-2026-08-28.md).

## Remaining certification lanes

- Profile D PTY replay for the actual pipe through `PAGER` and for
  terminal-driver signal timing. The decisions those lanes drive are proven in
  process (`TestPOSIXPaginationDecisionRequiresTerminalAndCrt`,
  `TestPOSIXCompositionInterruptAndIgnore`); the terminal itself is not.
- Integration evidence for identity/authorization and multiprocess lock
  recovery on the configured certification hosts.

The interface evidence remains **partial** until those integration lanes are
complete. Local-only transport is a deliberate product boundary, not a claim
that SMTP belongs to POSIX certification.
