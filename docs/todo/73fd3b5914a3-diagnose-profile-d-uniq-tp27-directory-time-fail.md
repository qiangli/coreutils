---
id: 73fd3b5914a3
kind: bug
title: Diagnose Profile D uniq TP27 directory-time FAIL
seq: 185
status: done
priority: p0
created: 2026-10-03T13:27:24.827548Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:04.846016Z
closed_by: codex-gpt6-sol
---

New exact full Profile D diagnostic uniq TP27 GA11 FAIL: utility output check reports directory file times changed after uniq input output; earlier integrated and GNU controls PASS. Determine whether a product write, harness timing, or host effect; preserve raw evidence. Fix only a proved defect, verify locally, and request focused exact replay before next full candidate.

2026-10-03 triage: The full 117-set arm completed with one new `uniq:27` FAIL. The raw journal reports only that directory file times changed. The installed licensed helper compares nanosecond access, change, and modification times on the existing output directory; those source bytes remain in the private evidence store. The current applet reads the input, then calls `os.Create` on the output operand. It has no directory enumeration or timestamp write.

An isolated Linux probe on the held diagnostic host used the exact static one-file Bashy candidate, POSIXLY_CORRECT=1, the same foreign-owner/mode directory shape, and separate GNU controls. The one-file `ls -ld -- uniq_out` helper step and `uniq uniq_in uniq_out` each preserved all three nanosecond timestamps. Both Bashy and GNU `uniq` returned failure for the directory output. The Bashy syscall trace shows `openat` with create/truncate flags returning `EISDIR`, with no timestamp-changing syscall. Private sealed probe manifest SHA-256: `61917f8f3521154278ecff321dcb27a1fc9e15e55db70a3532b1f215401a707f`.

The installed GA11 helper was also invoked outside TCC in isolated scratch with the exact Bashy route, POSIX mode, and the test's foreign UID/GID configuration. Three consecutive fresh-fixture invocations returned success for its directory, regular-file, FIFO, and symlink phases, with empty stderr. An initial invocation also returned success, but its environment setup produced unrelated stderr, so the three clean trials carry the inference. The staged one-file executable SHA-256 was `7f4aab920342fcb0b7471401bb1252b2ececaf75ccb90cd835a4c9dda2be6347`; the private trial manifest SHA-256 is `a17ed8b4c274ebbe91763f827279628ce6e8a4430085b586156e77945cc498d3`. The attempted timestamp wrapper did not capture intermediate fields, so these passes do not identify the timestamp that differed in the full-run failure. The full journal gives TP27 start/end times only (11:17:17–11:17:29 UTC); it does not retain per-phase timestamps.

No applet defect is established. Keep the new raw FAIL visible. Next gate: exact-candidate focused licensed `uniq` replay in a separate results path, with directory timestamps and helper commands traced privately if TP27 fails again. A product patch requires a reproduced product-caused mutation; a passing focused replay would establish intermittency but would not rewrite the full-run result.

## Sprint 355 acceptance evidence 2026-10-04

The controlled GA11/atime investigation is recorded at 1a8eda4b and a1ec5c72; the clean watcher-free full6 candidate reports raw PASS for uniq:27. The prior FAIL remains historical and the no-live-fixture-observation rule is documented. Historical raw journals and any pending formal certification decisions are unchanged.
