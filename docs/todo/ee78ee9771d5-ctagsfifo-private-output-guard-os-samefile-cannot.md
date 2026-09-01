---
id: ee78ee9771d5
kind: task
title: 'ctagsfifo private-output guard: os.SameFile cannot see an inode-reuse substitution'
seq: 18
status: todo
priority: p1
created: 2026-09-01T14:02:08.277627Z
sprint: 100
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
