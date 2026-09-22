# Windows FIFOs: marker file + named pipe (format v1)

**Status:** v1, frozen. Writer: `cmds/mkfifo` (`marker.go` is the in-repo
format implementation). Reader/opener: the shell's Windows open path — a
separate lane implemented **from this document** (the shell cannot import
this module). Any change to the bytes or the semantics below is a wire break
and needs a version bump in the magic line.

## Problem

A Windows named pipe lives in the machine-local `\\.\pipe\` namespace, not
on a filesystem, so a path like `C:\Temp\a.pipe` cannot itself *be* a pipe.
`mkfifo` therefore does what Cygwin's `fhandler_fifo` does (design copied,
format **not** Cygwin-interoperable): it writes a small **marker file** at
the FIFO's path, and every open of that path recognises the marker and
rendezvouses on the named pipe it names instead.

## The marker file

A regular file containing exactly two LF-terminated ASCII lines, with the
`FILE_ATTRIBUTE_SYSTEM` attribute set:

```
!<bashyfifo>\n
bashy-fifo-<32 lowercase hex>\n
```

* Line 1 — magic: the 13 bytes `!<bashyfifo>` + LF. A future format changes
  this line; v1 readers reject anything else.
* Line 2 — the **pipe leaf**: `bashy-fifo-` + 32 lowercase-hex chars
  (128 random bits from a CSPRNG, chosen at mkfifo time), + LF.
* Nothing follows line 2. A v1 marker is exactly 57 bytes (13 + 43 + 1).
  CRLF anywhere is a mismatch.

Grammar (ABNF, case-sensitive literals):

```
marker = %s"!<bashyfifo>" LF leaf LF
leaf   = %s"bashy-fifo-" 32(%x30-39 / %x61-66)
```

## Detection — what an opener MUST check

1. The path resolves to a **regular file** of size ≤ 128 bytes.
2. `FILE_ATTRIBUTE_SYSTEM` is set (Cygwin's convention for special files;
   magic bytes alone must not turn a user's ordinary file into a FIFO).
3. The full content matches the grammar above **exactly**.

All three or it is not a FIFO — treat the file as the regular file it is,
never "almost a FIFO". Leaf strictness is a **security boundary**: an opener
that accepted a looser line 2 could be pointed by a crafted marker at an
arbitrary pipe (`\\.\pipe\lsass`, a squatted service pipe, …). Reject; never
sanitise.

## Pipe name

```
native path = \\.\pipe\ + leaf        e.g. \\.\pipe\bashy-fifo-9f8c…
```

Where a path must survive shell word re-parsing, the equivalent
forward-slash spelling `//./pipe/<leaf>` may be used (same convention as
sh's Windows process substitution). Named pipes are machine-local: a marker
on a network share does not rendezvous across machines.

## Open semantics — the opener contract

Model (Cygwin's): **readers own pipe instances (servers); writers are
clients.** `mkfifo` creates **no** pipe — an instance's lifetime is its
creating process, and the FIFO must outlive `mkfifo`; the marker is the only
durable artifact.

Every instance is created with:

```
CreateNamedPipe(\\.\pipe\<leaf>,
    PIPE_ACCESS_INBOUND,
    PIPE_TYPE_BYTE | PIPE_WAIT | PIPE_REJECT_REMOTE_CLIENTS,
    PIPE_UNLIMITED_INSTANCES, 65536, 65536, 0, default security)
```

* **open for read (`O_RDONLY`)** — create an instance as above, then block
  in `ConnectNamedPipe` until a writer connects (this is POSIX's
  open-blocks-until-a-writer). Reading then proceeds on the instance;
  `ERROR_BROKEN_PIPE` / `ERROR_PIPE_NOT_CONNECTED` on read is **EOF** (the
  writer closed). v1: EOF ends the stream — one writer connection per open.
  An opener MAY `DisconnectNamedPipe` + re-`ConnectNamedPipe` to accept a
  further writer, but nothing in the suite requires it.
* **open for write (`O_WRONLY`)** — `CreateFile(GENERIC_WRITE)` on the
  native path. `ERROR_FILE_NOT_FOUND` means no reader is listening yet:
  sleep ~10 ms and retry (this is POSIX's open-blocks-until-a-reader);
  honour the shell's cancellation while looping. `ERROR_PIPE_BUSY` means
  readers exist but every instance is taken: `WaitNamedPipe(name, 50)` and
  retry.
* **open read-write (`O_RDWR`, bash's `exec 9<> a.pipe`)** — create a
  reader instance, then immediately self-connect one `GENERIC_WRITE` client
  to the same name; the fd is the **pair** (reads from the server handle,
  writes to the client handle). Never blocks, and bytes written come back on
  read — the Linux `O_RDWR`-FIFO behaviour the test suite relies on. If
  another reader instance happens to be listening, the self-client may land
  on it instead; v1 accepts this (POSIX likewise lets any reader consume any
  writer's bytes).
* **`O_NONBLOCK`** (SHOULD; nothing in the suite needs it yet) — reader:
  create the instance with `FILE_FLAG_OVERLAPPED`, leave the connect
  pending, return success. Writer: single `CreateFile` try; no listening
  instance ⇒ fail with `ENXIO`.

Error mapping: `ERROR_ACCESS_DENIED` from `CreateNamedPipe` (the leaf exists
as another user's pipe) ⇒ `EACCES`. Failure to read the marker itself maps
as any file open error.

## unlink, rename, copy, recreate

* **unlink** is `DeleteFile` on the marker. Already-connected ends are
  unaffected; an opener blocked waiting for a peer stays blocked (both match
  POSIX). `rm` needs nothing special beyond the readonly handling it already
  has.
* **rename** keeps the leaf, so it keeps the FIFO — as on POSIX.
* A byte **copy** of the marker aliases the same pipe name. POSIX `cp` reads
  *through* a FIFO rather than cloning it, so nothing sensible does this;
  noted, not defended against.
* **recreate** after unlink draws a fresh random leaf: a new, distinct FIFO.

## stat and permissions

`test -p`, `ls -l`, `find -type p` SHOULD report type `p` for a path that
passes detection (separate lanes; not part of this one). Permission bits
follow the platform's recorded-ACL mode scheme (`tool/mode.go`): `mkfifo`
itself records nothing — like `mkdir` and `touch`, it leaves `chmod` as that
scheme's one writer, and `-m` is applied by mkfifo's ordinary chmod step.
One documented divergence: an opener must *read* the marker even for a
write-only open, so a FIFO whose mode denies read to a class (e.g. 0200)
denies that class every open — stricter than POSIX.

## Security notes

* Leaf grammar strictness (above) — the opener MUST reject, never repair.
* `PIPE_REJECT_REMOTE_CLIENTS`, always.
* Instances carry the default DACL (creator full control): same-user
  rendezvous works; another local user who can read the marker can create
  the first instance and receive a writer's bytes — the same trust model as
  a world-readable FIFO in a shared directory. `FIRST_PIPE_INSTANCE` is
  deliberately not set: multiple concurrent readers are legal.

## Documented deviations from POSIX FIFO semantics (v1)

* No single shared kernel buffer: each writer↔reader connection is its own
  conduit. Concurrent writers may rendezvous with different readers, and
  `PIPE_BUF` atomicity is per-connection.
* EOF is per-connection (the connected writer closing), not
  last-writer-closes.

## Interop

Not Cygwin's on-disk format (Cygwin derives `\\.\pipe\cygfifo…` from device
and inode numbers of its own system-attribute file). A Cygwin FIFO is a
plain file to bashy and vice versa — intentionally, so neither side
half-works against the other's protocol.
