---
id: ee78ee9771d5
kind: task
title: 'ctagsfifo private-output guard: os.SameFile cannot see an inode-reuse substitution'
seq: 18
status: done
priority: p1
created: 2026-09-01T14:02:08.277627Z
assignee: claude-opus5.5
sprint: 110
sprint_id: 26c12bac-7289-517f-ad46-01e2c117cad7
sprint_title: Revalidate Go 1.27 GNU and POSIX conformance
closed: 2026-09-30T11:22:09.724122Z
closed_by: claude-opus5.5
---

CI-blocking. Reproduced on ubuntu-latest in GitHub Actions runs 33513814323 and
33515325794; PASSES 5/5 in a local golang:1.26 Linux container and on darwin, so
it is a CI-environment-sensitive failure, not a pure logic bug.

Observed:
  ctagsfifo_unix_test.go:344: Run()=1 stderr="ctags: <tmp>/tags: context deadline exceeded"
The test expects Run() to reject the substitution with "private output changed".

Leading hypothesis (NOT yet confirmed on the failing host). The test does
os.Remove(p.output) then os.WriteFile(p.output, ...). openPrivateOutput in
fifo_unix.go accepts the reopened file when os.SameFile(original, current) holds
-- that is device + inode only. On ext4/tmpfs the freed inode is routinely
handed straight back to the next create, so the REPLACEMENT can present the same
(dev, ino) as the file it replaced. The guard then passes, copyToFIFO proceeds to
openFIFO on a reader-less FIFO, and the 2s fifoOpenTimeout fires -- which is the
message CI reports.

Why it matters beyond CI: this is an anti-substitution guard. If the hypothesis
holds, the guard can be defeated by an inode-reuse race, and the only reason the
run still fails closed is the unrelated FIFO timeout.

Suggested direction: widen the identity to something a reused inode cannot forge
-- (dev, ino, ctime) via unix.Fstat on the RETAINED descriptor, or hold the
original fd open across the provider run and compare it with the reopened path.
Confirm the mechanism first: log st_dev/st_ino/st_ctim of original vs current on
a Linux runner before changing the guard.

Verify with: go test ./cmds/posixproviders/internal/ctagsfifo -run TestPrivateOutput -count=5

## Review 2026-09-30 (steward)

- Status: still open - `fifo_unix.go` and `fifo_other.go` still compare with `os.SameFile` (dev+ino only), and `linux ... ctagsfifo TestPrivateOutputIdentityChangeIsRejected` is still baselined in `test/known-failures.txt` at coreutils dc805500.
- Outdated: nothing material; container reference `golang:1.26` should now be go1.27.1.
- Next step: actionable today, independent of the licensed host. Confirm the inode-reuse mechanism on a Linux runner (log dev/ino/ctime of original vs current), then hold the original fd across the provider run or compare (dev, ino, ctime) via Fstat on the retained descriptor; delete the known-failures line.
- Acceptance: `go test ./cmds/posixproviders/internal/ctagsfifo -run TestPrivateOutput -count=50` green on Linux CI and darwin; baseline entry removed; substitution rejected with "private output changed", not by FIFO timeout.
- Depends on: nothing.

## Conductor brief 2026-09-30 (Sprint #110 heat)

Toolchain: go1.27.1. You may be on darwin: the failure is LINUX-ONLY ({PATH_MAX} 4096 vs 1024), so do not
declare victory from a darwin run. If podman works in your environment, the Linux check is
  /Users/qiangli/.bashy/sprint/110/gate/linux-go-test.sh -count=1 <pkg> [-run X]
(golang:1.27, ordinary user, pinned ../sh). Otherwise reason from the Linux regime and make the
regression reproducible on darwin too where you can (a package-level seam, never a sleep).
Scope: only the files named below plus test/known-failures.txt (delete the story's baseline line in the SAME commit).
Commit with trailers Sprint: #110 / Story: #<seq> / Story-ID: <id> from this file's front matter.

Gate (graded outside your booth, both must pass):
  linux-go-test.sh -count=50 ./cmds/posixproviders/internal/ctagsfifo -run TestPrivateOutput ;
  darwin go test -count=50 same ; a NEW deterministic test TestPrivateOutputInodeReuseIsRejected
  passes (it must simulate a replacement that presents the SAME dev+ino, e.g. via a stat/identity seam,
  and assert rejection with "private output changed", not a FIFO timeout) ; the ctagsfifo
  TestPrivateOutputIdentityChangeIsRejected line and its comment block gone from test/known-failures.txt.
Fix direction per the review: hold the original fd across the provider run, or compare (dev, ino, ctime)
via Fstat on the retained descriptor -- in fifo_unix.go AND fifo_other.go.
Scope: cmds/posixproviders/internal/ctagsfifo/**.
