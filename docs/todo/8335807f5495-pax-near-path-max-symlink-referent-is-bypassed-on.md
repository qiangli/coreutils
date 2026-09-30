---
id: 8335807f5495
kind: task
title: 'pax: near-PATH_MAX symlink referent is bypassed on Linux with a wrong ''not UTF-8'' diagnostic'
seq: 22
status: todo
priority: p1
created: 2026-09-01T15:03:01.56994Z
sprint: 110
sprint_id: 26c12bac-7289-517f-ad46-01e2c117cad7
sprint_title: Revalidate Go 1.27 GNU and POSIX conformance
---

`pax` is a POSIX-required utility in the certification scope. This is a
CONFORMANCE failure that only appears on Linux, and it is currently invisible to
anyone checking on macOS.

    --- FAIL: TestFollowedSymlinkBelowOperandNearPathMaxIsArchived
        source_pathmax_test.go:255: write -L near-PATH_MAX descent =
        (0, "pax: <long p.../referent>: value cannot be encoded as UTF-8; bypassed"),
        want (0, "")

Evidence: ubuntu-latest, GitHub Actions runs 33513814323 and 33515325794;
reproduced in a golang:1.26 Linux container. PASSES on darwin.

Why the platform split matters, and why this is filed separately from
95fcce29a7cc (which is about attributing the eight shared FAIL seats): {PATH_MAX}
is 4096 on Linux and 1024 on darwin, so the "near-limit" source tree the test
constructs lands in a completely different regime on each. The sibling story's
recorded note "the current tree already contains the complete recent pax
correction series ... `go test ./cmds/pax` passes" is therefore true on macOS
and false on Linux. Do not exonerate any pax identity on the strength of a
darwin run.

The diagnostic itself is suspect and is the place to start: the referent path is
pure ASCII 'p' characters, so "value cannot be encoded as UTF-8" is reporting
the wrong cause -- most likely a length/truncation condition in the extended
header path being classified as an encoding condition. A pax that emits a
misleading diagnostic and bypasses a member is a conformance problem in its own
right, independent of whether the archive contents are correct.

Reproduce (as an ORDINARY USER, never root -- root inverts permission-based
tests elsewhere in this suite):
  podman run --rm --user "$(id -u)" -v "$PWD:/w" -w /w -e HOME=/tmp \
    docker.io/library/golang:1.26 go test ./cmds/pax -run NearPathMax -v

Baselined in test/known-failures.txt (linux) so it does not mask a NEW break;
delete that line when this lands.

## Review 2026-09-30 (steward)

- Status: still open - `linux ... cmds/pax TestFollowedSymlinkBelowOperandNearPathMaxIsArchived` is still baselined in `test/known-failures.txt` at coreutils dc805500, and no cmds/pax commit has landed since 2026-09-01.
- Outdated: `golang:1.26` in the repro; use the certification toolchain go1.27.1 (`golang:1.27`), still as an ordinary user.
- Next step: actionable today, independent of the licensed host. Reproduce in a Linux container as a non-root user; trace why a pure-ASCII near-PATH_MAX referent is classified as a UTF-8 encoding failure (likely a length condition in the extended-header path); fix the classification and the bypass; delete the known-failures line.
- Acceptance: the test passes on the Linux CI leg and in the container; known-failures line removed in the same commit; `go test ./cmds/pax` green on darwin and Linux.
- Depends on: nothing. Feeds 95fcce29a7cc (pax cluster) - do this first.

## Conductor brief 2026-09-30 (Sprint #110 heat)

Toolchain: go1.27.1. You may be on darwin: the failure is LINUX-ONLY ({PATH_MAX} 4096 vs 1024), so do not
declare victory from a darwin run. If podman works in your environment, the Linux check is
  /Users/qiangli/.bashy/sprint/110/gate/linux-go-test.sh -count=1 <pkg> [-run X]
(golang:1.27, ordinary user, pinned ../sh). Otherwise reason from the Linux regime and make the
regression reproducible on darwin too where you can (a package-level seam, never a sleep).
Scope: only the files named below plus test/known-failures.txt (delete the story's baseline line in the SAME commit).
Commit with trailers Sprint: #110 / Story: #<seq> / Story-ID: <id> from this file's front matter.

Gate (graded outside your booth, both must pass):
  linux-go-test.sh -count=1 ./cmds/pax ; darwin go test -count=1 ./cmds/pax ; the
  'cmds/pax TestFollowedSymlinkBelowOperandNearPathMaxIsArchived' line gone from test/known-failures.txt ;
  every package still compiles (go test -run '^$' ./...).
Start at the emitter of "value cannot be encoded as UTF-8" in cmds/pax: an ASCII-only referent is
being misclassified -- find the length/limit condition that reaches the encoding branch, report the
real cause (or archive the member, as POSIX requires for a representable name), and never bypass silently.
Scope: cmds/pax/**.
