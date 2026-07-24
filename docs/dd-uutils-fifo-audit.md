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
