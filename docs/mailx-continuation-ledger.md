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

## Remaining certification lanes

- Profile D PTY replay for terminal pagination and signal timing.
- Integration evidence for identity/authorization and multiprocess lock
  recovery on the configured certification hosts.

The interface evidence remains **partial** until those integration lanes are
complete. Local-only transport is a deliberate product boundary, not a claim
that SMTP belongs to POSIX certification.
