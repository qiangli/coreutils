# Pure-Go `mailx`, `pax`, and `make` with agentic extensions

Status: implementation roadmap. No command is considered shipped or a Bashy
certification provider until it satisfies the repository release and native
evidence gates.

## Why these three

The 2026-08-12 Bashy-plus-system diagnostic exposed three unusually large
areas:

| owner | GNU A blockers | Bashy B blockers | interpretation |
| --- | ---: | ---: | --- |
| `mailx` | 432 | 432 | provider absent; every purpose uninitiated |
| `pax` | 199 | 199 | shared provider/host behavior, not Bashy-only |
| `make` | 12 | 39 | 27 additional Bashy recipe-shell outcomes |

These numbers are planning evidence, not promised retirements. Implementing a
provider makes tests execute; it does not make all of them pass. `make` is
especially important: replacing GNU make in profile B would hide the shell
differential. Bashy's shell behavior must first be repaired against GNU make,
and the pure-Go make provider must be evaluated independently in profiles C/D.

The product roles are broader than filling provider gaps:

| command | compatibility role | Bashy agentic role |
| --- | --- | --- |
| `mailx` | POSIX mail user agent | primary message/communication interface for agents |
| `pax` | POSIX portable archive exchange | packaging, compression, encryption, signing and provenance envelope |
| `make` | POSIX dependency build tool | explicit agentic-mode alias/front door for Bashy's `dag` executor |

Compatibility mode and agentic mode are separate contracts. Invoking these
commands in POSIX or ordinary shell mode never silently enables an agentic
extension. Bashy may select the agentic surface only when agentic mode is
explicitly enabled for that invocation or session; the selected mode must be
observable in plans, traces and structured results.

## Non-negotiable architecture

- Library first, command adapter second, multicall registration last.
- Pure Go and cross-platform; no cgo.
- Traditional command names preserve documented POSIX/upstream behavior.
- Unsupported behavior fails explicitly; no compatible-looking approximation.
- Agentic behavior uses distinct, documented extension flags or Bashy verbs.
- Structured results never replace traditional stdout/stderr by default.
- Credentials and message bodies follow Bashy's secret/redaction policy; no
  plaintext secret is copied into telemetry, command history, or evidence.
- Every mutating operation supports a plan/dry-run representation before an
  agent receives authority to execute it.
- Conformance and agentic tests are separate: an extension cannot make an
  upstream compatibility test pass by changing the compatibility surface.

## Phase 1 — `mailx`: portable message and mailbox provider

### Compatibility core

Implement the POSIX command around explicit interfaces:

- message parser/writer with byte-preserving headers and bodies;
- local mailbox store with locking and atomic rewrite;
- recipient/address parsing and aliases;
- interactive command interpreter and non-interactive send mode;
- transport interface for local delivery and SMTP;
- stable exit status and diagnostics; and
- invocation-owned environment, clock, hostname, terminal and credential
  dependencies for deterministic tests.

The first certification slice should use a hermetic local spool and transport.
Network SMTP/IMAP support is a product feature, not a prerequisite for testing
local POSIX semantics. Platform-specific mailbox locations must be explicit
configuration, never guessed silently.

### Agentic surface

Treat `mailx` as Bashy's primary agent-facing communication interface. Provide
typed operations through Bashy for compose, send, receive, list, inspect,
reply, forward, search, watch and archive. The same interface should support
human mail, agent-to-agent messages and durable system notifications without
pretending that all transports are Internet email.

Each operation returns a structured envelope with message ID, conversation or
correlation ID, sender identity, recipients, transport decision, redacted
preview, attachments, delivery state, effect class and evidence pointer.
Sending requires explicit authority; reading, watching and searching remain
separately grantable. Delivery adapters may include local spool, SMTP/IMAP and
Bashy's own durable message transport, but transport selection must be explicit
and inspectable.

Integrate Bashy's encrypted-marker facility for SMTP/IMAP credentials. Message
bodies are not secrets by default, but callers may mark any field sensitive so
it is redacted from logs and traces while remaining available to the transport.

### Gates

- parser and mailbox fuzzing;
- concurrent mailbox locking and crash recovery;
- fake transport tests with zero network access;
- Linux/macOS/Windows builds;
- public compatibility corpus plus native `mailx` affected-target replay; and
- proof that secrets and marked bodies are absent from diagnostics/telemetry.

## Phase 2 — `pax`: safe portable archive provider

### Compatibility core

Build on Go's archive primitives where their semantics are exact, with explicit
code for missing POSIX behavior:

- list, read, write and copy modes;
- ustar and pax extended headers;
- required cpio formats when not supplied by a permissively licensed module;
- selection, rename and substitution rules;
- ownership, modes, times, links and pathname handling;
- deterministic traversal and archive ordering where allowed; and
- fail-closed handling of unsupported devices, ACLs, xattrs and sparse files.

Extraction must use a capability-aware filesystem boundary. Reject absolute
paths, traversal escapes, link escapes and overwrite-policy violations before
mutation. Planning and validation complete before extraction begins whenever
the input permits it.

### Agentic surface

Treat `pax` as Bashy's portable payload-envelope interface. Add archive
inspect, plan, diff, verify, extract and create operations with a
machine-readable manifest, plus explicit compression, encryption, decryption,
signing and signature-verification extensions. Compression and cryptography
are Bashy extensions, not claims about the POSIX `pax` grammar.

The plan records every destination path, overwrite, link, metadata change,
codec, key reference, signature and rejected member. Verification emits
content hashes and provenance suitable for Bashy evidence graphs. Encryption
uses versioned, authenticated envelopes and key references; it never accepts
ad-hoc password-derived formats as a compatibility shortcut. Encrypted text or
payloads use Bashy's registered marker/magic prefix so command adapters,
redactors and agents can recognize them without attempting speculative
decryption. Plaintext keys and decrypted payloads must never enter command
history, telemetry or evidence bundles.

### Gates

- differential format fixtures and round trips;
- adversarial path/link/device corpus;
- bounded-memory streaming and cancellation;
- cross-platform metadata tests with explicit capability skips; and
- native `pax` affected-target replay before replacing an external provider.

## Phase 3 — `make`: POSIX engine plus graph-native execution

### First fix the shell differential

Keep GNU make in profile B while investigating its 27 additional Bashy
outcomes. Use public Makefiles with `SHELL` pinned alternately to GNU Bash 5.3
and Bashy `sh`, covering recipe exit status, signals, redirections, environment,
cwd, umask, command substitution and parallel children. Fix the shell or
harness owner; do not change GNU make or mask the difference with a new
provider.

### Compatibility core

A pure-Go make implementation is a larger independent project:

- POSIX makefile lexer/parser and include handling;
- variables, expansion, conditionally assigned values and command-line vars;
- explicit, suffix and inference rules;
- dependency timestamps, phony/precious/silent/ignore semantics;
- correct update ordering and failure propagation;
- jobserver/parallel execution only after serial semantics are complete; and
- recipe execution through an injected shell interface.

Recipe execution is an allowed process boundary because running commands is
the documented purpose of `make`. In Bashy it should also support an injected
in-process shell runner, but the external-shell path remains required for
compatibility and oracle testing.

### Agentic surface

Reuse Bashy's `dag`, weave and evidence machinery instead of creating a second
orchestrator. In an explicitly enabled agentic session, `make` is an alias or
front door for `dag`: Makefiles are imported as a graph projection of targets,
prerequisites, variables, inferred edges and recipes, and execution is managed
by the existing DAG engine. `bashy make ...` must expose in its plan that this
agentic dispatch occurred.

Outside explicit agentic mode—including POSIX mode, scripts and certification
profiles—`make` remains the compatibility command and must not route to `dag`.
An explicit escape hatch must also let an agentic session request compatibility
execution for oracle testing and reproducible builds. Agentic plan/run can add
effect classification, approval gates, caching, remote workers, trace context
and artifact provenance without changing the imported recipe semantics.

The Makefile is source input; Bashy's durable task graph is the execution and
observability layer. Round-tripping or rewriting a Makefile is a separate,
explicit feature.

### Gates

- public POSIX make corpus and GNU/BSD behavioral differentials;
- generated dependency-graph/property tests;
- serial execution before parallel execution;
- signal/cancellation/recursive-make tests;
- deterministic agentic plan and effect-cap tests; and
- native `make` replay only after the Bashy-shell differential is separately
  understood.

## Delivery order and boundaries

1. `mailx` local compatibility kernel and fake transport.
2. `mailx` agentic typed operations and secret-safe transport adapters.
3. `pax` read/list/manifest and safe extraction.
4. `pax` create/copy modes and complete compatibility surface.
5. GNU-make/Bashy-shell differential repair.
6. Pure-Go serial POSIX make engine.
7. Make graph projection and AgentOS execution extensions.
8. Parallel/remote make only after serial conformance and recovery are stable.

Each phase lands in bounded commits. Registration in `cmds/all`, `cmds/lean`,
the atlas and generated applet matrix occurs only when that command's honest
supported surface and tests are ready. Native diagnostic evidence may guide
development, but only a complete denominator-checked replay can change the
campaign scoreboard.
