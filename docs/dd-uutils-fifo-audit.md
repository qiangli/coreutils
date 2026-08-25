# dd FIFO Conformance Audit

Scope: public uutils `tests/by-util/test_dd.rs` at
`a7551d77574266075f085d7db9add85e15dec7d6`. All six tests that create a FIFO
are listed below. The audit compares their command lines with coreutils
`55960c0940ea918ef060a9d0f21eda6ef88f3fff`.

| Test (source line) | `dd` operands | Audit result |
|---|---|---|
| `test_random_73k_test_lazy_fullblock` (1057) | `ibs=521 obs=1031 iflag=fullblock if=fifo status=noxfer` | Accepted and covered by a bounded irregular-writer regression. |
| `test_sync_delayed_reader` (1461) | `ibs=16 obs=32 conv=sync if=fifo status=noxfer` | Observed landmine: `conv=sync` was rejected before FIFO rendezvous. This change implements it and adds the exact bounded delayed-writer shape. |
| `test_seek_output_fifo` (1520) | `count=0 seek=1 of=fifo status=noxfer` | Accepted and covered by the bounded output-offset regression. |
| `test_skip_input_fifo` (1542) | `count=0 skip=1 if=fifo status=noxfer` | Accepted and covered by the bounded input-offset regression. |
| `test_reading_partial_blocks_from_fifo` (1665) | `ibs=3 obs=3 if=<fifo>` | Accepted; a bounded two-write regression now pins partial-record reblocking. |
| `test_reading_partial_blocks_from_fifo_unbuffered` (1710) | `bs=3 ibs=1 obs=1 if=<fifo>` | Rendezvous was safe, but `bs` did not override later `ibs`/`obs`. Order-independent precedence and a bounded regression are added. |

No other FIFO or named-pipe construction occurs in the pinned file. Only the
observed `conv=sync` case is quarantined. Every regression uses a subprocess
deadline, nonblocking writer-open/write loops, and kill/wait cleanup so a
future early parser exit cannot strand the test process.

## SIGINT and platform boundary

`dd` makes descriptor reads and writes nonblocking only while it owns the copy,
waits for readiness together with a SIGINT self-pipe, and restores the original
flags of stdin/stdout descriptors borrowed from `RunContext`. On SIGINT it emits
the current record status once, returns 130 to embedded callers, and asks the
standalone multicall boundary to terminate with SIGINT so the process wait
status is `WIFSIGNALED` rather than a normal exit code.

Overlapping embedded invocations that borrow the same descriptor share a
reference-counted nonblocking lease. The original mode is restored through a
private duplicate only after the final borrower exits, so an earlier completion
cannot strand a remaining read or write and descriptor-number reuse cannot
redirect the restore operation.

Linux named-FIFO input uses an `O_NONBLOCK` read descriptor and a poll state
machine. Linux retains `POLLHUP` when a writer opens and closes before the first
read; `TestDdLinuxFIFOImmediateWriterOpenCloseBeforeFirstReadStress` exercises
that exact transition 500 times without sleeps. The unlink/rename plus SIGINT
regression also checks that no descriptor or goroutine survives the command.

Darwin/XNU does not expose that already-finished writer transition after a
nonblocking read open. A cancellable blocking `open(2)` cannot be released
safely after the FIFO pathname is unlinked or renamed, so pathname-based FIFO
input is deliberately refused rather than approximated. It exits 1 immediately
with the deterministic diagnostic:

```text
dd: failed to open 'NAME': interruptible named FIFO input is not supported on darwin
```

Regular files, anonymous streams, and FIFO output remain supported on Darwin.
