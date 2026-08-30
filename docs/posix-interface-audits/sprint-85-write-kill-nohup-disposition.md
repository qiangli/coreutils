# Sprint 85 disposition — write:22 (write-kill-nohup probe)

Identity: `write:22` — VSC-PCTS2016 `write` test set, assertion 22, as
observed under the sprint 85 write-kill-nohup probe contract
(`vsc-pcts-harness-kit` → `SPRINT-85-WRITE-KILL-NOHUP-DISPOSITION`,
`s85-write-kill-nohup` probe). The suite itself is licensed and was not
consulted; this document records only what the public probe contract carries
plus evidence produced inside this repository.

## Owner hypothesis

The identity sits on the **terminal/login-session/mesg authority boundary**:
whether `write` may reach a recipient is decided by the chain

1. the recipient account exists,
2. the login-accounting database carries a `USER_PROCESS` record for it
   (a `DEAD_PROCESS` record is not a login),
3. the session the record names is alive and owns the terminal,
4. the device exists,
5. the device's group-write bit — the bit `mesg(1)` owns — permits messages,
   narrowed by an explicit terminal operand, with the superuser exempt.

`write` (not a GNU coreutils utility; util-linux/BSD lineage upstream) is the
only set in scope here. The `kill` and `nohup` identities named by the same
sprint disposition are out of this reducer's scope and are not touched.

## Evidence state — why no attribution exists

The retained GNU control run **did not execute** `write:22`. An identity the
control arm never executed produces no control transcript, so there is no
matched A/D pair to compare: the bashy-arm observation cannot be attributed to
the product, to the harness, or to arm provisioning. This is an evidence
vacuum, not a tie.

## Reducer

`cmds/write/s85_write_reducer_test.go` is the hermetic, suite-free reducer:

- synthetic recipients and session records only — the fixture encodes a
  synthetic utmp database and fake device files, injected through the package
  seams; the first test asserts the hermeticity contract itself (no host login
  database, no `/dev`, every "terminal" a regular file), so the reducer can
  never write to a real user TTY;
- replays the authority ladder end to end through the public `run()` entry —
  account/no-record, unknown account, `DEAD_PROCESS` record, dead session,
  `mesg n` on the only line, operand naming a denied line, `mesg y` delivery,
  multi-line selection with the stdout notice, operand narrowing suppressing
  the notice, superuser exemption, and the no-controlling-terminal banner;
- asserts **byte-exact** transcripts on every sink (recipient terminal,
  sender's controlling terminal, stdout, stderr, exit status), because a
  byte-exact transcript is the artifact a matched control run must reproduce
  for attribution;
- all rungs are green. **No one-sided product red is causally proven**, so no
  product or session-support patch was made.

## What the green reducer does and does not establish

Green establishes: under the public POSIX.1 Issue 7 write(1) specification
and the documented deterministic selection rule, every authority-boundary
behavior the reducer encodes conforms. It does **not** establish that the
suite's assertion 22 exercises that boundary exactly, nor that our diagnostic
wording matches what assertion 22 compares — the licensed expectation text
cannot be read, and no control transcript exists to diff against.

## REVIEW RESULT: UNRESOLVED

Ownership of `write:22` is not assigned. Do not mark this identity
product-owned or harness-owned on the strength of the reducer alone.

## Exact prerequisite for matched A/D evidence

1. A retained **GNU control arm that executes** `write:22`: the arm runner
   must provision the write tset's login-session fixture — a second user
   account with a live session on a real pty, `mesg y` and `mesg n` variants —
   in the control container, not only the SUT container, and the identity
   must run to completion (not be skipped or capped) there.
2. Per-identity capture from **both** arms: argv, env, stdin, stdout, stderr,
   exit status, and the recipient-terminal byte stream, in the harness kit's
   machine-readable result shape.
3. A byte-level diff of the two transcripts at the authority boundary
   (selection choice, notice text, banner shape, diagnostic text, exit code).
   Divergence attributable to the product — and only that — reopens this
   disposition with a causal red; everything else stays a provisioning or
   expectation mismatch.

Until both arms execute the identity, any further bashy-arm observation of
`write:22` is noise: record it, do not triage against it.
