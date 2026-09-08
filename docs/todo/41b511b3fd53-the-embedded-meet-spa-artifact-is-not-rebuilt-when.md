---
id: 41b511b3fd53
kind: task
title: The embedded meet SPA artifact is not rebuilt when its source changes, so bashy apps served a stale UI
seq: 102
status: done
priority: p1
created: 2026-09-07T22:37:26.648058Z
assignee: sprint139-manager
sprint: 139
closed: 2026-09-08T10:15:58.145015Z
---

FOUND 2026-09-07 by corbel while rebuilding bashy for this host, not by a test -- which is the point.

MEASURED. pkg/meet/artifact/ is a TRACKED BUILD PRODUCT: the bashy build promotes pkg/meet/web/dist into it and the binary embeds it. Its last refresh commit is 89bf6091 'meet: refresh embedded progress UI'. The SPA SOURCE changed twice AFTER that, both in sprint #135: 33b66c2d 'remove the send hold, cancel and recall' and 76471c47 'offer a draft as a suggestion instead of pre-filling the box'.

So both of those shipped features were absent from every embedded build between #135 and now. bashy apps served the pre-135 meet UI while the sprint that built them was closed green. Rebuilding regenerated the bundle (new content hashes), which is how it surfaced.

WHY NOTHING CAUGHT IT. The e2e suite builds the SPA itself in beforeAll and serves THAT, so it tests the source and never the embedded artifact -- the one thing users actually get. A green meet e2e run says nothing about what bashy apps serves. That is the same shape as the evidence invariant: the check that would have failed does not exist.

WANTED. Either the artifact stops being tracked and is always built (preferred -- a checked-in build product that can silently disagree with its source is the defect), or a gate asserts the tracked artifact matches a fresh build of its source and fails when it does not. A doc note telling people to remember is not a fix.

GATE. A check that rebuilds the SPA and compares against the tracked artifact, wired into the test target, demonstrated failing on the pre-fix tree (it will, today, on any checkout between 89bf6091 and this commit).

Implemented and independently accepted 2026-09-08 in Bashy fleet commits b7f9c23 + 15bde0a: build-meet-spa.sh check builds locked SPA output and compares bytes/file set against the tracked artifact without promotion; missing tools fail closed. make test and dag test enforce regression + freshness before Go tests; Linux CI has a dedicated provisioned meet-spa-fresh job. Owner real-SPA probe accepted fresh output, rejected the historical 89bf6091 index unchanged, and reproduced the old helper returning success while repairing that same stale artifact. Owner combined Go vet and full make test passed (6.629s and101.059s), including existing build safeguards; final Linux-safe empty-PATH fixture and explicit DAG fail-fast were reviewed. Logs: /tmp/sprint139-delivery/evidence/artifact-owner and artifact-integration-gate. Published dependency pin, final CI and installed revision are recorded by the sprint delivery handoff; no broad conformance claim.
