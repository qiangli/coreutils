# talk — POSIX Issue 7 conformance checklist

Normative source: [The Open Group Base Specifications Issue 7, 2018 edition,
`talk`](https://pubs.opengroup.org/onlinepubs/9699919799/utilities/talk.html).
The POSIX scope is local-host communication; historical `talkd` and remote-host
addressing are deliberately outside this implementation.

| Clause | Implementation | Executable evidence |
|---|---|---|
| Synopsis, no options, `address [terminal]` | One required and one optional operand; host-qualified addresses fail before notification. Universal help/version are extensions. | `TestRemoteRejectedBeforeSessionOrNotification`, `TestSyntaxAndTerminalRequirements` |
| Recipient and terminal selection | Resolves a real local account and `who` session; optional terminal selects the exact session; `mesg` and TTY ownership fail closed. | `TestDefaultAlwaysRequiresWhoAndMesg`, `TestTerminalSelectionAndNotificationFailureFailClosed`, `TestRecipientSelectionSkipsStaleTTYWhenAnotherIsUsable` |
| Invitation | Writes the required three-part invitation to an eligible recipient TTY. | `TestRunUsesOSIdentityNotEnvironmentAndNotifiesTTY` |
| STDIN/STDOUT terminals | Both streams must be terminals. Operational and syntax diagnostics use stdout; stderr is unused. | `TestSyntaxAndTerminalRequirements` |
| Two-way simultaneous character processing | Non-canonical/no-echo local input is polled while authenticated peer datagrams are polled; unrelated termios mappings and `iexten` are preserved, and local and remote transcripts render in independently labelled screen regions. | `TestTalkPTYUsesCharacterModeAndRestoresTerminal`, `TestTalkCharacterEventsKeepIndependentRegionsInSync` |
| Alert | An alert event emits the recipient terminal's `bel` capability. | `TestTalkAlertRefreshAndConfiguredTerminationCharacters` |
| Control-L | Refreshes both regions on the sender display and is not transmitted. | `TestTalkAlertRefreshAndConfiguredTerminationCharacters` |
| Erase and kill | Uses the invoking terminal's configured `VERASE`/`VKILL`; updates the current line at each endpoint. | `TestTalkCharacterEventsKeepIndependentRegionsInSync`, `TestTalkPTYUsesCharacterModeAndRestoresTerminal` |
| Interrupt and EOF | Uses configured `VINTR`/`VEOF`; local termination is status 0 and closes the peer endpoint. SIGINT also returns 0. | `TestTalkAlertRefreshAndConfiguredTerminationCharacters`, `TestConverseCancellationWithBlockedPipeReturnsAndClosesReader` |
| One-sided termination | Peer is notified in its peer region; ordinary input is then ignored and only local interrupt/EOF/SIGINT exits. | `TestTalkPeerCloseAllowsOnlyLocalExit`, `TestLocalUnixTransportIsPrivateAuthenticatedEphemeralAndConverges` |
| LC_CTYPE | C/POSIX bytes and UTF-8 characters classified as print/space are carried; other bytes become safe printable sequences; split UTF-8 reads are reassembled. | `TestPrintableInputAndUTF8Locales`, `TestTalkUTF8SplitAcrossTerminalReads` |
| TERM and terminal deficiency | Uses the shared pure-Go terminfo database for `clear`, `cup`, `el`, and `bel`; unknown, incapable, undersized, or inaccessible terminals fail before recipient notification. | `TestTalkTerminalCapabilityGateFailsClosed` |
| LANG, LC_ALL, LC_MESSAGES, NLSPATH | Category precedence is invocation-local through `pkg/locale`. The repository ships deterministic C/POSIX English talk messages and no translated talk catalogs; NLSPATH therefore selects no applet-owned catalog. | `pkg/locale/locale_test.go` |
| Transport/effects | Local-only encrypted authenticated AF_UNIX event transport; no transcript; endpoints removed on exit; malformed or legacy event frames fail closed. | `TestLocalUnixTransportIsPrivateAuthenticatedEphemeralAndConverges`, `TestAuthenticatedTransportDiscardsInjectedDatagramAndContinues`, `TestTalkRejectsMalformedOrLegacyPeerEvents` |
| Exit and terminal restoration | Normal EOF, interrupt, SIGINT, and context cancellation return 0; operational/capability/input/output failures return non-zero; the exact original terminal state is restored. | `TestTalkPTYUsesCharacterModeAndRestoresTerminal`, `TestConverseCancellationWithBlockedPipeReturnsAndClosesReader` |

## Honest residuals

- The implementation provides only the POSIX local-host address form. Remote
  BSD address forms and `talkd` are not POSIX requirements and are rejected.
- The repository has no translated `talk` message catalogs. C/POSIX English is
  deterministic for every locale; adding catalogs would be an optional
  internationalization enhancement, not a terminal-semantics workaround.
- Terminals without cursor addressing and clear-to-end-of-line capabilities
  are rejected instead of receiving the reduced-interaction mode POSIX permits.
- Runtime raw-mode support is implemented on Linux and BSD-family systems.
  Other build targets compile and fail closed with a terminal-mode diagnostic.
