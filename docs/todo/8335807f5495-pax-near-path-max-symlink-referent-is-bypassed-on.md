---
id: 8335807f5495
kind: task
title: 'pax: near-PATH_MAX symlink referent is bypassed on Linux with a wrong ''not UTF-8'' diagnostic'
seq: 22
status: todo
priority: p1
created: 2026-09-01T15:03:01.56994Z
sprint: 100
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
