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
//     exit codes and per-target status, forced to JSON by DHNT_AGENT=1.
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
package dag

// SchemaVersion is stamped into every dag envelope's schema_version field.
// Independent from weave's "loom-v2" — bump when the dag output shape changes.
const SchemaVersion = "dag-v1"
