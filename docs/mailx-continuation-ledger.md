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
- Header display and a bounded interactive subset: print/type, headers/from,
  next/previous, delete/undelete, save/write, quit/exit, current-message and
  numeric selection.
- Refusal of symlink and non-regular mailbox targets.

## Remaining POSIX lanes

- Complete message-state transitions, MBOX movement, default save behavior,
  command abbreviation and message-list grammars.
- Startup files, aliases, internal variables, conditional/source commands,
  replies/followups, composition escapes, paging/editing, and signal/dead-letter
  behavior.
- Full locale, terminal prompt, output-failure, identity/authorization, and
  multiprocess lock-recovery coverage.

The interface evidence remains **partial** until those lanes and the targeted
Profile D replay are complete. Local-only transport is a deliberate product
boundary, not a claim that SMTP belongs to POSIX certification.
