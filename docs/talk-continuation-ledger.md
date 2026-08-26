# `talk` continuation ledger

The pure-Go `talk` applet implements only same-host conversations. It follows
the public `talk address [terminal]` interface and records the command's BSD
history, but it is a clean Go implementation: no BSD C source was translated.
`talk` has no external-provider definition, build recipe, cache entry, or
fallback.

The applet derives identity from the operating-system account, requires a real
logged-in recipient terminal that permits messages, and writes the invitation
to that terminal. Remote and host-qualified addresses fail before session or
terminal notification work begins.

Conversation text and close notices use encrypted, authenticated datagrams over
ephemeral AF_UNIX sockets. Short-lived public-key endpoint metadata is owned and
validated per participant; injected or wrong-owner endpoints cannot join. All
session files are removed on exit and no transcript is written to `mb` or any
other durable store. The applet never contacts `talkd` or another host.

Implemented coverage includes reciprocal session convergence, stale-terminal
fallback, terminal selection and `mesg` permission checks, endpoint ownership
and datagram authentication, private cleanup, TTY requirements, UTF-8 and
control-character handling, peer close, EOF, cancellation, and SIGINT.
Interface evidence remains **partial** pending the configured Profile D replay
and remaining terminal-format and interaction clauses.
