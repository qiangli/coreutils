# Profile D `mailx` and `patch` Repair — 2026-08-28

Primary contracts:
[POSIX.1 Issue 7 `mailx`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/mailx.html)
and [POSIX.1 Issue 7 `patch`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/patch.html).
Both applets are pure-Go and are the sole shipped owners of their names: there
is no external-provider definition, build recipe, cache entry, or fallback for
`mail`, `mailx`, or `patch`, and none was introduced here. `mailx` remains
local-file-only by design — SMTP, remote routing, and an external MTA stay out
of scope, and a host-qualified address still fails before any mailbox changes.

The audit re-read the option, operand, interactive-command, diagnostic, exit
status, and signal clauses of both interfaces against the implementation and
fixed the deltas below. Nothing in the harness, the provider pins, or the
evidence manifests was touched.

## `patch` deltas

1. **A context line that reads like an ed command no longer hijacks the input
   format.** Auto-detection accepted an address-less `a`, `c`, `d`, or `i`,
   which is an ordinary line of context in the other three input forms and
   never something `diff -e` emits. A unified diff over a file containing a
   bare `a` line was therefore taken for an ed script: nothing was patched,
   `File to patch:` was written, and the command exited 2. Detection now
   requires an addressed command and stops at the first line that announces a
   copied-context, unified, or normal listing, so a real difference listing
   always wins over a resemblance inside its own data.
   `TestContextLineLikeEdCommandStaysAUnifiedDiff`,
   `TestNormalDiffWithEdLikeDataStaysANormalDiff`,
   `TestAddressedEdScriptStillAutoDetects`,
   `TestExtractEdScriptRequiresAddressAndYieldsToDiffListings`.

2. **`-p num` counts a leading `<slash>` run as exactly one component.**
   Issue 7 is explicit: "If the pathname in the patch file is absolute, any
   leading <slash> characters shall be considered the first component (that
   is, `-p 1` shall remove the leading <slash> characters)." Splitting on
   `/` made every extra leading slash its own empty component, so `-p 1`
   against `//curds/whey/f.txt` produced `/curds/whey/f.txt` and found no
   file. An interior run is likewise one separator, and the default
   (no `-p`) selection is now the final component rather than a slash count.
   `TestStripTreatsLeadingSlashRunAsOneComponent`,
   `TestStripSkipsAdjacentInteriorSlashes`,
   `TestStripComponentsPOSIXExamples` (the standard's own `-p 0`/`-p 1`/
   `-p 4`/default worked example).

3. **The informational message names the file that was patched.** It printed
   the raw header pathname — reporting `patching file /curds/whey/src/f.txt`
   while modifying `./f.txt` — and always ended in a stray `<blank>`. It now
   reports the stripped name actually operated on, with the dry-run marker
   attached without doubling blanks. `TestProgressNamesTheStrippedTargetExactly`.

Unchanged and re-confirmed by inspection: informational and diagnostic output
stays on standard error with standard output unused; exit status is 0, 1 for
rejects, and 2 for errors; `-b`/`-o`/`-r`/`-d`/`-D`/`-l`/`-N`/`-R` behavior and
the copied-context reject format are as the continuation ledger records.

## `mailx` deltas

1. **A hyphenated login is an address selector, not a malformed range.** Any
   msglist word containing `-` was parsed as `n-m` and rejected outright, so
   `from mary-ann` — an address exactly as it appears in the header summary,
   which Issue 7 requires to be matchable — failed with `invalid message
   range`. Only a fully numeric pair is the range form now; everything else
   falls through to address matching.
   `TestPOSIXAddressSelectorAcceptsHyphenatedLogin`.

2. **`retain` and `discard` are independent lists.** Each command deleted the
   other's entry for the same header-field, so `retain subject` followed by
   `discard subject` suppressed Subject and displayed every other field —
   the exact inverse of "If both retain and discard commands are given,
   discard commands shall be ignored" and of the retain command overriding
   all discard/ignore commands. The lists no longer withdraw from each other;
   a non-empty retained list still wins whole.
   `TestPOSIXRetainOverridesDiscardOfTheSameField`.

3. **`set crt` with a null value keeps pagination enabled.** Issue 7 makes the
   value implementation-defined when `crt` is set to null, and a bare
   `set crt` is the ordinary way to ask for pagination at screen height. The
   null value parsed as zero and disabled paging entirely. It now falls back
   to the screenful size (`screen`, default 20); an explicit numeric value and
   `nocrt` are unchanged.
   `TestPOSIXNullCrtKeepsPaginationEnabled`,
   `TestPOSIXPaginationDecisionRequiresTerminalAndCrt`.

4. **`~i` expands the `sign`/`Sign` escapes.** `~a` and `~A` are defined as
   `~i sign` and `~i Sign`, and those two variables recognize `\t` and `\n`,
   so the two spellings produced different text. They now agree.
   `TestPOSIXInsertVariableMatchesSignEscape`.

## Profile D disposition of the remaining lanes

The pagination and signal lanes that the mailx ledger carried as unresolved
were unresolved for want of a controlling terminal, not for want of behavior.
The decision halves are now proven in process — the pagination predicate
(terminal standard output plus a message longer than `crt`) by
`TestPOSIXPaginationDecisionRequiresTerminalAndCrt`, and the two-interrupt
abort, `ignore`/`@` handling and dead-letter write by the existing
`TestPOSIXCompositionInterruptAndIgnore`. What genuinely cannot be established
without a terminal is listed below rather than claimed.

For `patch`, the filename prompt reads from the controlling terminal
(`/dev/tty`); its stream, text, and answer handling are covered in process
through the injected reader (`TestMissingHeaderTargetPromptsForFilename`,
`TestFilenamePromptIsWrittenToStderr`, `TestDefaultReversalPromptsAndAppliesReverse`),
so only the terminal-driver leg remains.

## Honest residuals

* The actual pipe of message output through `PAGER`, and terminal-driver
  signal timing, still need a PTY replay; an in-process test can prove the
  decision and the handler, not the tty.
* `patch` writes its prompts to standard error. Issue 7 is self-contradictory
  here — Filename Determination says standard output, while the STDOUT section
  says "Not used" and LC_MESSAGES speaks of "prompts written to standard
  error". GNU `patch` uses standard output. The choice is left as it is,
  recorded as a deliberate, documented divergence rather than silently
  changed, because two clauses back it.
* Multiprocess mailbox lock recovery and `-u user` authorization remain
  integration lanes on the certification hosts.
* `-l` matches a run of <blank> characters against a run of <blank>
  characters, exactly as specified; GNU's looser whitespace handling is a
  compatibility objective, not a certification requirement.
