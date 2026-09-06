---
id: f29efccb687f
kind: task
title: 'Remote connection survival: ssh, web terminal and mesh links across suspend, network change and reboot'
seq: 84
status: todo
priority: p2
created: 2026-09-06T20:36:13.846842Z
sprint: 130
---

GOAL
A remote connection — the in-process SSH client, the web-console terminal,
and the mesh/relay link underneath both — survives the operator's laptop
suspending, changing network, or rebooting. The durable thing is the REMOTE
DAEMON, never the link to it: a link is disposable and must be rebuilt, not
nursed. Companion to a15d8f2ee490 (local daemon survival); that one keeps the
server alive, this one keeps the SESSION reachable across a dead link.

This is also where the strongest honest claim lives. In the local case
(a15d8f2e) nothing computes while the lid is shut — agents freeze and thaw.
On the remote path the far host STAYED AWAKE, so agents there genuinely keep
working the whole time the lid is closed, and reconnecting shows work that
continued without you. That claim is only true once the items below hold.

WHAT WAS MEASURED (2026-09-06, this tree — not inferred)

(a) An established SSH session has NO liveness probe and NO reconnect.
    outpost/internal/agent/sshclient:
      - client.go:89-90,109-110  HandshakeTimeout, default 30s
      - transport.go:74-75,90-91 DialTimeout, default 30s
    Both cover ESTABLISHMENT only. Once the session is up, the interactive
    path blocks in sess.Wait() (client.go:499) and is unblocked only by ctx
    cancel (:485-491). After a suspend the socket is half-open: the far side
    never learns, and the client blocks until some TCP timeout it does not
    control. Retry exists (transport.go:143-166) but only for the 401/403
    elevation gate — that is AUTH recovery, not liveness recovery.

    There are zero first-party hits for ServerAliveInterval,
    ServerAliveCountMax or ControlPersist across outpost/, coreutils/ and
    bashy/. That is not an oversight to fix by writing an ssh_config: this
    client speaks SSH in-process via golang.org/x/crypto/ssh over a
    WebSocket, so there IS no ssh_config to set. The ~60s "convert a
    half-open socket into a clean EOF" deadline has to be implemented in Go
    — a periodic SendRequest("keepalive@openssh.com") with a reply deadline
    — and an application-level heartbeat above it that declares the endpoint
    dead FASTER than the transport does, so the UI reacts before the socket.

    Separately: the ssh-proxy path shells out to the operator's own
    /usr/bin/ssh with their own ~/.ssh/config. If they have ControlPersist
    set, a stale multiplex master after a suspend can stall the first
    reconnect well past any backoff schedule — and because we manage no
    config there, we cannot tune it. Detect and bypass, do not silently
    inherit.

(b) The daemon side ALREADY does this correctly. Copy it; do not invent.
    outpost/internal/agent/tunnel.go:107-111 — reconnectInitialBackoff 2s,
    reconnectMaxBackoff 30s, supervised loop with jitter (:254-292). Its
    comment (:112-120) records exactly why the wrapper exists: the frp
    library's own reconnect gives up silently on a yamux session shutdown,
    leaving the process alive with no tunnel.
    So outpost->cloudbox recovers from a suspend and operator->host does
    not. THE ASYMMETRY IS THE DEFECT. Related trap already known: frp's
    Service.Run never returns — not on ctx cancel, not on Close — so a
    supervisor must never assume it will.

(c) The web-console PTY is DESTROYED by a dropped socket.
    coreutils/pkg/webterm/webterm.go — the PTY is created inside the
    WebSocket handler (start(opts, cols, rows), :152) and torn down by
    `defer sess.Close()` (:158). No session registry, no session id, no
    reattach.
    Consequence: closing the lid on the VIEWING laptop kills the shell on
    the SERVING host, and everything running in it. This is the strongest
    form of the gap — not "you cannot reattach" but "there is nothing left
    to reattach to", and it directly contradicts the survival claim
    a15d8f2e is making for the daemon that hosts it.
    The handler already distinguishes a clean close frame from a 1006 drop
    (:144-149) — the signal needed to tell "shell exited" from "network
    died" is present and simply is not acted on.

(d) Reconnect would rebuild the WRONG path.
    docs/mesh-transport-selection-gap.md (OPEN, measured 2026-08-28):
    `outpost reach` has rungs lan | cloudbox | offline and NO mesh rung, and
    its lan rung is gated on mDNS, which returns nothing site-wide. So a
    reconnect-after-wake will faithfully re-establish through the relay even
    while the libp2p mesh holds a direct hole-punched QUIC link to that same
    host. On a metered relay that is a data-egress problem, not a latency
    nit (docs/cloudbox-data-plane-block.md). Any reconnect built here must
    consult the transport that is already holding a connection, rather than
    re-deriving locality from multicast.

SCOPE

  S1. SSH liveness in Go: periodic keepalive request with a reply deadline
      on the established session, so a half-open socket becomes a clean EOF
      on a bounded schedule instead of an indefinite block.
  S2. An application-level heartbeat ABOVE the transport that declares an
      endpoint dead faster than S1 does, so the surface reacts first.
  S3. Supervised reconnect with exponential backoff, reusing tunnel.go's
      shape and its hard-won lesson. Each attempt builds a BRAND NEW
      connection and discards the old one — disposable link, durable daemon.
  S4. Freeze, do not lie. While reconnecting, the last known surface stays
      visible but dimmed and frozen, with input and navigation disabled
      until a fresh connection delivers a coherent state. Never let anyone
      type into a stale screen.
  S5. Durable web-console PTY: a session registry keyed by a session id, so
      a dropped WebSocket detaches instead of killing, and a returning
      client reattaches to the running shell. Requires an explicit lifetime
      and reap policy for an orphaned PTY, and a bounded scrollback replay
      on reattach.
  S6. Reconnect consults the mesh: add the missing direct rung so a rebuilt
      link takes the direct path when one is live, per (d).

GATE

  G1. coreutils `go test ./...`; outpost `go test -short ./...`.
  G2. pkg/webconsole/console_dom_test.go stays green (sprint 130 UI rule),
      plus a browser-level assertion of the frozen/dimmed reconnect state.
  G3. REAL SUSPEND, not a simulated socket close. On a live pair of hosts:
        - start a long-running command on the remote host through each
          surface (in-process ssh client, ssh-proxy, web console terminal);
        - suspend the CLIENT laptop for longer than any backoff cap, and
          separately change its network (Wi-Fi to tethered) so the source
          IP and NAT mapping actually change;
        - on wake, confirm each surface reconnects unattended, the remote
          command KEPT RUNNING throughout, and its output since the drop is
          recoverable;
        - reboot the client and confirm reattach to the still-running work;
        - confirm the rebuilt link took the direct rung when the mesh had
          one, by inspecting the reported path and not by assuming it.
      Record transcripts per surface. A surface with no transcript is
      reported UNVERIFIED, never as passing.
  G4. Per-OS: the client side must be exercised on macOS, Linux and Windows.
      Cross-compilation is not evidence.

NON-GOALS

  - mosh, autossh, or tmux-on-the-far-side. The remote daemon is already the
    durable thing; adding a session multiplexer would be a second answer to
    a question we have answered.
  - Keeping any machine awake. Refused in a15d8f2e and still refused.
  - Unbounded scrollback retention for a detached PTY. Bounded replay only;
    the retention depth is a decision this story must record, not assume.
  - Fixing the mesh rung itself beyond what reconnect needs — S6 is
    "consume the signal", the full defect stays with its own doc.

CROSS-REPO NOTE
  Most of S1-S4 and S6 land in outpost (internal/agent/sshclient, tunnel.go,
  cmd/outpost/reach.go); S5 lands in coreutils (pkg/webterm, pkg/webconsole).
  Sprint 130 currently tracks dhnt, coreutils and bashy — NOT outpost. Before
  the first outpost commit, run `bashy sprint track 130 --repo` inside
  outpost, or the commit-provenance guard will not resolve the story
  reference. Filed here in coreutils so the two survival stories stay
  together.

SPRINT NOTE
  Filed at p2 and deliberately NOT linked to a #130 goal item, for the same
  reason as a15d8f2ee490: sprint 130's gate is closed-ended and this is new
  work. It will show under "UNCOVERED by the plan" until an operator links or
  moves it. PRIORITY-FIRST execution should not let it displace the p0/p1
  gate.
