// Copyright (c) 2025 qiangli
// See LICENSE for licensing information

// Package dag is an agent-first task runner: a Makefile replacement whose
// targets are defined as headings in a markdown file (DAG.md) and executed as
// a real dependency DAG. It is built for AI agents first, humans second — the
// inverse of make/task/just.
//
// Why it exists:
//   - Targets are plain markdown an agent already reads and writes fluently
//     (the xc "docs ARE the tasks" idea), so adding a target is appending a
//     heading + a fenced code block. No tabs, no proprietary DSL.
//   - Discovery is structured: `dag --list --json` returns the full target
//     inventory (names, descriptions, requires, sources/generates) so an agent
//     plans against the real graph instead of scraping `make help`.
//   - Results are structured: every run emits a weavecli.Envelope with stable
//     exit codes and per-target status, forced to JSON by BASHY_AGENTIC=1.
//   - Bodies run hermetically through the in-process mvdan.cc/sh/v3 fork with
//     the coreutils userland resolved in-process (shell.Handler()) — no PATH
//     variance, identical on Linux/macOS/Windows.
//
// P1 (this code) is the minimal-but-real DAG engine: parse -> build graph ->
// cycle detection -> topological SERIAL execution -> bash-in-process -> envelope
// output. Later phases layer parallel scheduling + fingerprint skip (P1.5), the
// dhnt contract/effects/attestation model (P2 — each target may declare an
// `Ensure:` postcondition and an `Effects:` cap), and multi-interpreter bodies
// (P3 — go/python/starlark via RegisterInterpreter). The `Ensure:`/`Effects:`
// metadata is already parsed-and-ignored here so a P2 file parses cleanly today.
//
// # Incremental fingerprint cache
//
// dag's up-to-date skip is content-hashed, not mtime-based (make's prerequisite
// model): a touched-but-unchanged source does not force a rebuild. The store is
// one JSON file per DAG document (see [Cache] / [LoadCache]):
//
//   - Cache key (the JSON file name): sha256(absolute DAG-file path) hex-encoded.
//     One document → one cache file, so two checkouts of the same pipeline at
//     different paths never collide.
//   - Location: os.UserCacheDir()/bashy/dag/<key>.json — on Linux that is
//     ~/.cache/bashy/dag/, on macOS ~/Library/Caches/bashy/dag/, on Windows
//     %LocalAppData%\bashy\dag\. Writes are atomic (tmp + rename).
//
// Inside the file, Hashes maps a target name to its last successful
// fingerprint. A target's fingerprint (see [Cache.Fingerprint]) folds, in
// order: its body, then each dependency's already-computed fingerprint (so an
// upstream change invalidates everything downstream), then the content hash of
// every Sources/Inputs path (a file's bytes, or a directory's recursive file
// contents). A target is up-to-date — and is skipped — iff it declares
// Generates, all of those outputs exist on disk, AND its recorded fingerprint
// matches the freshly computed one. A target with no Generates is phony and
// always runs. `--force` (-B) ignores the cache entirely; `--explain` prints,
// per target, whether it would run or is up-to-date and why, running nothing.
//
// # Ensure: postcondition vocabulary
//
// A target's `Ensure:` line is a postcondition the engine evaluates AFTER the
// body exits 0 (P2 contract): a clean exit is necessary but not sufficient — if
// any Ensure check fails the target fails with the precondition exit code, even
// though make would have called it done. A target may carry more than one
// `Ensure:` line; all must pass. The recognized predicate forms are:
//
//   - file-exists <path>   — the path exists (relative to the DAG-file dir).
//     Example: `Ensure: file-exists dist/app` (also `file-exists path=dist/app`).
//   - file-absent <path>   — the path does NOT exist (e.g. a clean target removed
//     it). Example: `Ensure: file-absent dist/stale.tmp`.
//   - http-ok <url>        — an HTTP GET returns 2xx (a readiness probe).
//     Example: `Ensure: http-ok http://localhost:8080/healthz`.
//   - cmd <shell...>       — an explicit shell command; exit 0 = pass.
//     Example: `Ensure: cmd test "$(cat VERSION)" = 1.2.0`.
//   - <bare shell command> — anything not matching the forms above is run as a
//     shell command through the in-process userland; exit 0 = pass.
//     Example: `Ensure: test -s dist/app && ./dist/app --version`.
//
// The `file-exists`/`file-absent`/`http-ok` sugar also accepts the explicit
// `key=value` spelling (`path=`, `url=`). See contract.go for the evaluator.
//
// # Fleet execution
//
// `--fleet` runs targets through a [Pool] of [Worker]s instead of a bare -j
// semaphore. A worker offers one or more execution venues; a [Transport] is how
// a target reaches one. Only the userland venue ships today — same host, in
// process — so `--fleet` on one box is `-j N` by construction: `Pool == nil`
// degrades to LocalPool(Concurrency), and there is no second code path to drift.
//
// The pool is the gate, not an addition to one. It owns placement and per-worker
// slot accounting, which a single global semaphore cannot express ("4 slots
// here, 12 there"). [Pool.Eligible] fails a target fast when no worker could
// ever host it, rather than parking it on a slot that will never qualify.
//
// Chunking is a separate axis and pays off with no fleet at all. Chunk
// membership — which cases are in chunk i, out of how many — is a property of
// the CORPUS, pinned in a committed manifest (see [LoadChunkManifest]) and
// changed only when the corpus changes. Fleet capacity decides how many chunks
// run concurrently, never how many exist or what is in them. Deriving membership
// from capacity would make `suite:shard=7` name a different case set depending
// on who was online, which breaks both selective re-run and the fingerprint
// cache. [BindChunks] hands each shard target its committed case list via
// DAG_CHUNK_MEMBERS; a manifest that reaches no target is an error, because
// silently dropping corpus produces a flattering pass rate.
//
// # dag-v1 schema stability
//
// Every dag envelope stamps schema_version = "dag-v1" (see [SchemaVersion]).
// The compatibility policy is additive-only: new fields may be added to the
// envelope, the list/run/plan results, the attestation, or a target's metadata
// without bumping the version — agents MUST ignore unknown fields rather than
// reject them. schema_version bumps only on a breaking change: removing or
// renaming a field, changing a field's type, or altering the meaning of an
// existing value. New target metadata keys (Matrix/Secrets/Artifacts/When and
// future additions) and new Ensure predicate forms are additive and do not bump
// the version; a reader that does not understand one simply does not act on it.
package dag

// SchemaVersion is stamped into every dag envelope's schema_version field.
// Independent from weave's "loom-v2" — bump when the dag output shape changes.
const SchemaVersion = "dag-v1"
