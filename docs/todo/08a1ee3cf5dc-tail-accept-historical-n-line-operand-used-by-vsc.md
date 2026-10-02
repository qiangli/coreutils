---
id: 08a1ee3cf5dc
kind: bug
title: 'tail: accept historical +N line operand used by VSC patch setup'
seq: 178
status: todo
priority: p0
created: 2026-10-02T00:27:07.258685Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Profile C and D patch TP33 are UNRESOLVED during suite setup, before patch runs. The licensed setup executes tail +3 on a generated diff; the current Go tail treats +3 as a pathname and exits 1 (reproduced suite-free). Profile B GNU tail passes that startup but TP33 fails later for a different reason. Add the historical +N line-count extension in POSIX mode without changing -n +N or file operands after --, test the exact behavior, and run focused patch TP33 replay after D raw evidence is archived. Retain original result codes.

Implementation candidate recognizes a leading `+N` only when no file by
that name exists under the invocation directory. Existing `+N` files and
`-- +N` remain path operands. Focused native tests exercise the VSC spelling
under `POSIXLY_CORRECT=1`, the existing-file case, and the `--` case; the
Linux test binary cross-compiles. Exact licensed replay is still required.
