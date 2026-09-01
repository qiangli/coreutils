---
id: 87b57c7fad8a
kind: task
title: 'S88: resolve shared-B more and od blockers'
seq: 14
status: todo
priority: p0
created: 2026-08-31T23:30:21.747016Z
assignee: s88-more-od
sprint: 88
---

Investigate Profile D shared-B blockers more:130/132/133/134 and od:5/16 from tracked public metadata, public source/history, and short native suite-free reducers. Implement only the smallest POSIX Issue 7-aligned fix with focused native tests; otherwise record the exact redacted result tuple needed. No licensed suite/journal bytes, local containers, pushes, merges, or pin bumps. Done when each identity has a source-backed disposition and any patch passes focused package tests.

## Public-evidence disposition

- `more:130,132,133,134`: do not product-patch. The public-safe Sprint 85
  handoff records all four as shared by both retained GNU controls and not a
  Bashy regression. POSIX Issue 7 requires a non-advancing command at the
  final-file EOF prompt to exit; the current `17be7d54` behavior implements
  that rule. The earlier inverse change `7669ce51` was correctly rejected.
- `od:5,16`: no public tracked artifact maps either number to a clause or
  output category. The public Issue 7 audits report no reproduced `od`
  interface defect and explicitly decline to reconstruct licensed purpose
  counts from absent public evidence. Do not infer a fix from result code or
  adjacency.

The droplet base `c747cabf` and current `main` have identical `cmds/more` tree
`39f11f9c02eb4292ec92f2b7f208d701fb7e40ea` and identical `cmds/od` tree
`fa278a0dfa1e270adc71f4389d7ef0515fbc1766`.

Focused native evidence:

```text
go test ./cmds/more ./cmds/od -count=1                         PASS
go test ./cmds/more -run 'EOF|ExitOnEOF' -count=20            PASS
go test ./cmds/od -run 'POSIX|Offset|Format|Address|Locale' -count=20 PASS
```

## Current replay classification

The sealed current replay completed all 16 Shared-B targets with no caps or
infrastructure failures. The authorized fixed-vocabulary tuples classify all
four `more` purposes as comparison-phase failures. Their paired shapes are
`130/132` (13 lines, four `error`, two `expected`, two `exit`, two `stderr`,
two `command`; `130` additionally has one `locale`) and `133/134` (eight
lines, two `error`, one `expected`, one `exit`, one `stderr`, one `command`;
`133` additionally has one `locale`). This reinforces the shared-control
disposition; it does not identify a `more` state-machine clause.

Both `od` purposes remain phase `unknown`; each has two `option` tokens and
stdout-only category hits (`od:5` four, `od:16` two). Public reduction shows
that this shape could match the two no-address spellings (`-A n` and `-An`)
that pre-launch commit `395c40ab` already repaired. Current
`POSIXLY_CORRECT=1` reducers for both spellings emit the first transformed item
without a leading separator, and the existing 20x address/format suite is
green. Repeating that patch or changing unrelated formatting would therefore
be speculative.

## Sharpened category-only evidence request

For `od:5` and `od:16`, provide fixed-vocabulary counts for `address`,
`radix`, `decimal`, `octal`, `hexadecimal`, `no_address`, `skip`, `count`,
`type`, `character`, `named`, `signed`, `unsigned`, `float`, `duplicate`,
`offset`, `stdin`, `file`, `default`, `order`, `invalid`, and `diagnostic`;
separate stdout/stderr byte, line, and SHA-256 tuples; whether
`POSIXLY_CORRECT` was present; and current/historical GNU-control result
classes. For `more`, the sufficient fixed vocabulary is `eof`, `last_file`,
`next_file`, `prompt`, `locale`, `unsupported`, `comparison`, `timeout`,
`terminal`, `command`, `exit`, `stderr`, and `expected`, plus GNU-control
result classes. No raw output, expected bytes, suite source, journal text,
private path, hostname, or credential is required.
