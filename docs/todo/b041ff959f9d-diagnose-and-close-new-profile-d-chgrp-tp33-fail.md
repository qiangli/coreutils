---
id: b041ff959f9d
kind: bug
title: Diagnose and close new Profile D chgrp TP33 FAIL
seq: 184
status: done
priority: p0
created: 2026-10-03T05:54:58.115122Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:04.587838Z
closed_by: codex-gpt6-sol
---

Fresh exact one-file static Profile D arm profile-d-s355-base-full-20261003 reached chgrp assertion #33 raw FAIL at set7 after prior integrated D chgrp:33 PASS. Preserve raw journal and partial-arm stop provenance; inspect GA45 invalid nonnumeric group operand, group lookup, command exit/stderr, host group database, and old pinned controls. Prove product or fixture cause before code. Repair if product defect, run focused exact-pin chgrp and early full rerun with POSIX mode on; keep frozen ledger unchanged.

## Sprint 355 acceptance evidence 2026-10-04

The chgrp:33 investigation is recorded at 33258a31; the clean full6 candidate reports raw PASS for chgrp:33, with no new chgrp failure. The earlier partial-arm raw FAIL remains historical. Historical raw journals and any pending formal certification decisions are unchanged.
