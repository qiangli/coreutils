# The message-board addressing & delivery model

`bashy mb` / `bashy ping` post to one public, append-only board (`pkg/bus`,
`board.go`). A **send** produces a **receipt**, and the one rule the receipt must
obey is Yoke's: *claim only what the store can prove*. A confirmation that reads
like a delivery when nothing was delivered is the same class of defect as an exit
code reporting a success nothing verified.

This document is the canonical mapping the code implements.

## Resolution happens at SEND time

Before a post is written, the target a sender typed is resolved. Precedence,
most specific first (`ResolveSendTarget`):

| kind     | what it matches                                        | address used            |
|----------|--------------------------------------------------------|-------------------------|
| `role`   | a seat on this host (`steward`, `conductor:22`)        | the seat's stable topic |
| `agent`  | a name in the roster (`bashy agents list`)             | the canonical fleet name |
| `reader` | a name with a cursor — it has read the board ≥ once     | itself                  |

A target matching **none** of the three resolves to nothing. The send then
writes NOTHING and fails, naming near misses (`NearMisses`, ranked by edit
distance across roles + roster + readers) plus the broadcast escape hatch. This
is Yoke's requirement that an unresolvable identity "fail with choices instead of
guessing".

> The defect this closes: `bashy ping --as X profile-b "msg"` used to be accepted,
> posted with a literal `to: profile-b`, and reported *"waiting on the board for:
> profile-b"* — a receipt indistinguishable from a real delivery, to a name no
> reader answered.

## The six provable Delivery states

`Delivery.State` (`board.go`) is one of six, from most contact to least. The
classifier is `deliveryState(to, seq, steered, perReader)`.

| state        | proof                                                                      |
|--------------|----------------------------------------------------------------------------|
| `delivered`  | pushed into a live session — `SteerLive` succeeded                          |
| `read`       | the recipient's cursor is **at or past** the post's sequence               |
| `queued`     | appended, and the recipient's cursor is **behind** that sequence           |
| `unverified` | appended, but the recipient has **no cursor at all** — it has never read    |
| `accepted`   | well-formed and the target resolved, with **no single reader cursor** to judge (a role seat, a selector group, a broadcast) |
| `failed`     | the target resolved to no role, agent or reader — **nothing was written**   |

`unverified` is the important one, and the state the old wording erased. A reader
that has never read is **not merely behind**: reporting `queued` there claims more
than the evidence supports. `CursorSeq` draws the line — it returns `ok=false`
for a name with no cursor, which is distinct from a cursor of zero.

`perReader` is false for a role/selector/broadcast (no single cursor to judge),
so those prove at most `accepted`; it is true for an agent or an existing reader,
which have a cursor of their own and so can reach `queued` / `read` / `unverified`.
