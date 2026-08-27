# POSIX mailx completion ledger

Normative reference: POSIX.1 Issue 7, 2016 Edition, `mailx`. The implementation
is deliberately a local-mail delivery system: an address is a local account and
delivery is an mbox transaction. SMTP, talk-to-sendmail, and network routing are
outside this applet. POSIX leaves non-login address forms and the underlying
delivery mechanism unspecified.

## Interface

- Send synopsis and required `-s` are implemented.
- UP receive synopses and `-e`, `-f`, `-F`, `-H`, `-i`, `-n`, `-N`, and `-u`
  are implemented. `-n` suppresses only the system startup file; the user
  `MAILRC` is still processed.
- Files, standard input, local delivery, diagnostics, mboxrd output, and
  `-e`/normal exit status have behavioral tests.
- `DEAD`, `EDITOR`, `HOME`, `LISTER`, `MAILRC`, `MBOX`, `PAGER`, `SHELL`,
  `TERM`, `TZ`, and `VISUAL` are imported without reading process-global
  environment state. Locale selection remains the repository's documented
  deterministic C-locale behavior.

## Receive-mode state machine

- New, unread, read, deleted, preserved, explicitly saved, and MBOX-directed
  states are distinct.
- The initial current message is first-new, then first-unread, then first.
- `Status: O`/`Status: RO` preserves aging/read state between invocations.
- Quit transactionally updates the mailbox while retaining concurrent appends.
- Read system mail moves to MBOX; `hold`, `keepsave`, `append`, explicit save,
  delete, `mbox`, secondary-mailbox, and `exit` dispositions are separate.
- Message selectors cover number, `+`, `-`, `.`, `^`, `$`, `*`, ranges,
  sender, `/subject`, and `:d/:n/:o/:r/:u`.

## Commands and composition

The POSIX command vocabulary is recognized with its specified minimum
abbreviations: aliases and alternates; directory/folder commands; copy/save/
write; delete/undelete/hold/mbox/touch; headers/from/print/Print/top/size/next;
mail/reply/Reply/followup/Followup; pipe, editor, visual, shell and `!`; startup
conditionals/source/set/unset; help/list/echo/folders; scroll, quit, and exit.

Composition supports `~!`, `~.`, `~:`/`~_`, `~?`, `~A`, `~a`, `~b`, `~c`,
`~d`, `~e`, `~f`, `~F`, `~h`, `~i`, `~m`, `~M`, `~p`, `~q`, `~r`/`~<`,
`~s`, `~t`, `~v`, `~w`, `~x`, and `~|`. Two-interrupt abort, `-i`/`ignore`,
DEAD saving, `dot`, `ignoreeof`, subject/Cc/Bcc prompting, and local reply
delivery are implemented.

## Deliberate boundaries and residual portability work

- Remote and host-qualified delivery is rejected before any mailbox changes.
- Diagnostic text is deterministic English rather than message-catalog
  localized; this is the repository-wide C-locale contract.
- Shell word expansion for filenames covers environment parameters, home
  expansion, and pathname matching. Exotic command-substitution and arithmetic
  expansion inside a filename are not yet claimed.
- Terminal pagination and signal paths are implemented but require the Profile D
  PTY suites for final certification evidence; ordinary unit tests cannot prove
  terminal-driver timing.
- Delivery to several different mailbox files is validated before writing, but
  the files are not a cross-filesystem atomic transaction. POSIX does not define
  crash atomicity for the underlying mail delivery system.

These residuals must remain visible until the corresponding certification tests
pass; this document is a clause ledger, not a substitute for VSC-PCTS evidence.
