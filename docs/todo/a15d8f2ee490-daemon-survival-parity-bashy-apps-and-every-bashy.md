---
id: a15d8f2ee490
kind: task
title: 'Daemon survival parity: bashy apps and every bashy service across terminal close, sleep and reboot'
seq: 83
status: todo
priority: p2
created: 2026-09-06T20:24:21.245928Z
sprint: 130
---

GOAL
`bashy apps` and every other bashy service (meet, sdlc, schedule, and anything
later adopting pkg/svcd) survive the same three events with the SAME guarantee
on macOS, Linux and Windows:

  1. the terminal that started them goes away (window closed, SSH dropped,
     VS Code / Windows Terminal tab closed, user logged out);
  2. the machine suspends and wakes (lid close);
  3. the machine reboots.

Today (1) is weak and inconsistent, (2) is unowned, and (3) exists only on a
paired outpost host.

WHAT WAS MEASURED (2026-09-06, this tree — not inferred)

(a) Unix detach is process-GROUP only, never a session.
    - pkg/svcd/proc_unix.go:21      -> SysProcAttr{Setpgid: true}
    - pkg/meet/service_unix.go:21   -> SysProcAttr{Setpgid: true}
    - pkg/sdlc/background_unix.go:13-> SysProcAttr{Setpgid: true}
    - pkg/schedule/background_unix.go:13 and job_process_unix.go:15
                                    -> SysProcAttr{Setsid: true}
    So four daemon paths, and they DISAGREE with each other. Setpgid keeps the
    controlling terminal open on the daemon; setsid(2) is the guarantee the
    other two already take. Nobody decided this — it drifted.

(b) Windows has NO detach at all.
    pkg/svcd/proc_other.go:16 -> `func applyBackgroundProcAttrs(cmd *exec.Cmd) {}`
    An empty function. No DETACHED_PROCESS, no CREATE_NEW_PROCESS_GROUP, and
    no job-object escape. Windows Terminal and VS Code place children in a Job
    Object carrying JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE, so a bashy daemon
    started from either is killed the moment the console window closes,
    however "detached" the spawn looked. Windows must do MORE than unix to
    reach the same guarantee: detect that the current job kills on close, and
    when it does, create the daemon outside the caller's job (WMI
    Win32_Process.Create is the known-working route). This is the single
    largest parity hole and it is invisible from a mac dev box.

(c) The lid itself is correctly NOT ours.
    Zero first-party hits for caffeinate, IOPMAssertion,
    SetThreadExecutionState, systemd-inhibit or logind across coreutils/ and
    bashy/ (the only matches are vendored external/podman). bashy never asks
    an OS to stay awake, and it SHOULD NOT start: closing the lid means what
    the operator's power settings say it means (macOS clamshell rules, logind
    HandleLidSwitch, powercfg lid action). Agents do not keep computing
    through a suspend on any platform; they freeze and thaw.
    The claim we may make is the narrow one and it is uniform: NOTHING DIES
    ACROSS THE CYCLE. The daemon, its listener and every PTY come back as they
    were, with no reattach. That is the claim this story must make true —
    "work continues while the lid is shut" is NOT the claim and must not be
    written into any doc or README.

(d) Reboot replay exists only via outpost, and only for a paired host.
    outpost's `Supervised` programs (internal/agent/conf/file.go:595-607) are
    the reboot-durability mechanism: the OS service starts `outpost
    supervisord` at boot and anything it owns returns with it. svcd's own
    header already assumes this supervisor (start idempotent, stop must free
    the port, stale pidfile is success).
    Standalone bashy has NOTHING: zero launchd/systemd/schtasks registration
    anywhere in coreutils/pkg. That contradicts the standing rule that bashy
    is standalone-graceful and cloudbox/outpost only enhance it. After a
    reboot on an unpaired host, `bashy apps` is simply gone, and a stalled
    service is indistinguishable from an idle one.

SCOPE

  S1. One detach contract, one implementation. Fold meet, sdlc and schedule
      onto pkg/svcd (it is already the declared factoring target and only
      pkg/webconsole has adopted it). Resolve Setpgid-vs-Setsid deliberately
      and record WHY in the code, not in a commit message.
  S2. Windows parity: DETACHED_PROCESS + CREATE_NEW_PROCESS_GROUP, plus the
      job-object escape gated on an actual "does my job kill on close?" probe.
      Do not take the WMI path unconditionally.
  S3. Reboot replay without outpost: a bashy-owned per-user autostart
      registration (LaunchAgent / systemd --user / Scheduled Task) that is
      OPT-IN, and that DEFERS to outpost's supervisord when the host is
      paired, so there are never two launchers racing for one port.
  S4. Wake behaviour stated and tested: after suspend/resume the listener is
      still bound, the pidfile still identifies the same process, and status
      reports running. Remote/mesh reconnect is separate work and is NOT in
      this story.
  S5. Document the honest matrix in bashy/docs/bashy-web-console.md and
      docs/agent-interaction-surfaces-design.md: what survives, on which OS,
      by which mechanism, and what the operator's power settings still own.

GATE (a green byte-level unit test is NOT sufficient here)

  G1. coreutils suite: `go test ./...` in coreutils.
  G2. UI: pkg/webconsole/console_dom_test.go stays green (sprint 130 rule for
      any UI-touching change).
  G3. PER-OS RUNTIME PROOF, not cross-compilation. Standing rule: cross-
      compile/CI green is not evidence a daemon survives on that OS. On each
      of macOS, Linux and Windows, on real hardware or a real VM:
        - start `bashy apps`, close the launching terminal (on Windows do it
          from BOTH Windows Terminal and VS Code's integrated terminal), then
          confirm the port still answers and status says running;
        - suspend and resume the machine, then confirm the same;
        - reboot, then confirm the service returns unattended under S3 (and,
          on a paired host, that it returns exactly once).
      Record the three transcripts. A platform with no transcript is reported
      as UNVERIFIED, never as passing.

NON-GOALS

  - Keeping the machine awake. Explicitly refused; see (c).
  - Continuing computation through suspend. Not possible; do not imply it.
  - Session/conversation restore for agent panes after reboot (an agent whose
    harness has no capture path returns with cwd and layout but no
    conversation). That is a separate story if wanted.
  - Remote/mesh reconnect-on-wake.

SPRINT NOTE
  Filed at p2 and deliberately NOT linked to a #130 goal item: sprint 130's
  gate is closed-ended (output reduction, sprint transfer, human-lane p0) and
  this is new work. It will show under "UNCOVERED by the plan" until an
  operator either links it to a goal or moves it. PRIORITY-FIRST execution
  should not let it displace the p0/p1 gate.
