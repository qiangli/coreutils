# Feasibility Study: A Pure-Go POSIX `ed` and a Separate Agentic Patch/Edit Surface

Status: **Draft for review**
Scope: Implementation feasibility, prior-art survey, and licensing analysis for adding
(1) a strict POSIX `ed` command and (2) a distinct, agent-safe patch/edit surface to this
repository. Every external claim is tagged by evidence class:

- **[Sourced]** — verified against a primary source (official repo, `pkg.go.dev`, or the
  POSIX standard) during this study. The source URL is cited inline.
- **[Inferred]** — reasoned from architecture/behavior, not directly cited; flagged so a
  reviewer can challenge it.

The repository has no prior feasibility draft under this filename; an earlier *rejected*
draft exists elsewhere and its specific defects are enumerated and corrected in §9.

---

## 1. Executive summary and recommendation

**Recommendation: proceed, but split the work into two independent deliverables.**

1. **POSIX `ed`** (`cmds/ed`) — a clean-room, pure-Go implementation of the POSIX.1-2024 `ed`
   utility. There is **no** production-grade, permissively-licensed, POSIX-faithful pure-Go
   `ed` to adopt (§5). However, the single hardest subsystem — a POSIX BRE engine **with
   backreference matching**, which Go's RE2-based `regexp` cannot provide — **already exists
   in this repository** as `pkg/bre` (§4.1). This removes the technical obstacle that sinks
   most "Go ed" attempts.

2. **The agentic patch/edit surface** — a *separate* API and CLI designed for programmatic
   file mutation by LLM agents: deterministic unified-diff application with content-hash
   preconditions, transactional/atomic writes, dry-run, rollback, audit provenance, and
   idempotent re-application. This is **not** `ed` and **not** POSIX `patch`; it must be
   designed on its own terms (§6, §8) and **must not be conflated with** POSIX `ed`, the
   classic `patch` program, or git's patch format (§2).

The two deliverables are complementary: the POSIX CLI preserves exact upstream semantics and
conformance; the agentic surface layers the safety and determinism properties an autonomous
agent actually needs. They may share a line-buffer primitive and `pkg/bre`, but they have
different contracts (§2).

**Go/no-go factors**

| Factor | Finding |
|---|---|
| Licensing blocker | None **[Sourced]**. All viable external libraries are MIT/BSD/Apache-2.0; the one promising ed clone with no license is excluded (§5). No GPL source will be touched. |
| Technical blocker (BRE backrefs) | **Already solved in-repo** by `pkg/bre` **[Sourced]** (§4.1). |
| Effort | POSIX `ed`: ~8–12 person-weeks; agentic surface: ~3–5 person-weeks (§11, ranges). |
| Conformance gate | The licensed `671-TP` gate is an *acceptance target* but its content is **licensed** — coverage must be reproduced from public standards, not copied from the artifacts (§12). |

---

## 2. Scope: four things that are *not* the same thing

A frequent source of error (see §9) is conflating these surfaces. They have different
semantics, contracts, and failure modes:

| Surface | What it is | Contract / failure mode |
|---|---|---|
| **POSIX `ed`** | Interactive, line-oriented, single-buffer text editor with its own command grammar (`a`,`c`,`d`,`s`,…) operating on a private copy that is written to disk only on `w`. **[Sourced]** pubs.opengroup.org/.../ed.html | Exact upstream command/output semantics; `?` diagnostics; errors to spec. |
| **POSIX `patch`** | Declarative: applies a *diff hunk* to a target file using context lines, fuzz, and offset search; writes the result. | Hunk-offset/fuzz application; `.rej` on failure. |
| **git/unified diff** | A *diff text format* (unified or git-binary), produced by `diff -u` / `git diff`. It is data, not behavior. | Format spec; can be parsed, generated, or applied by separate code. |
| **`ed` *script*** | A stream of `ed` commands (`2,5d\ns/a/b/\nw`) executed by an `ed` interpreter. **Not** a unified diff, though GNU `diff -e` emits one. | Interpreter semantics; fails on any bad command. |

> **Design principle.** The POSIX `ed` CLI honors upstream semantics verbatim. The agentic
> patch/edit surface (§6) is a *new* tool that happens to consume unified-diff payloads; it
> does **not** become POSIX `patch`, and it does **not** expose `ed` commands as an API. An
> `ed`-script executor can be offered later as an *extension* if needed, but it is a third,
> distinct surface.

---

## 3. Authoritative POSIX `ed` requirements

**[Sourced]** IEEE Std 1003.1-2024 (POSIX.1-2024, Issue 8), `ed` utility page:
https://pubs.opengroup.org/onlinepubs/9799919799/utilities/ed.html

Required behaviors any conforming implementation must provide (high level; not a copy of
licensed test material):

- **Buffer model.** Operates on an in-memory copy of the file. Disk is mutated *only* by the
  `w`/`W` commands; reads (`e`,`E`,`r`) replace/append the buffer. A "modified" flag tracks
  unsaved state and gates `q` (prompting) vs `Q`.
- **Regular expressions.** BRE (XBD §9.3), **including `\(...\)` grouping and `\1`…`\9`
  backreferences used within the matching RE**. The null/empty RE reuses the last RE.
  **[Sourced]** — this is the crux of §4.1/§7.
- **Text-file definition.** A *text file* is a sequence of lines each terminated by `\n`;
  behavior on files containing NUL bytes or a final line without a trailing newline is
  specified (warning + newline added). **[Inferred]** from the standard's line/encoding
  description and the well-known "no newline" interop behavior.
- **Locale.** `LC_ALL=C`-style deterministic behavior is the repository's baseline agent
  contract; collation classes (`[:alpha:]` etc.) resolve under the C locale. **[Inferred]**
  from the repo-wide contract in `README.md`/`CLAUDE.md`.
- **Signals and EOF.** Interrupt handling and end-of-input behavior are specified; an
  interrupt returns to command mode (discarding a partial text-input line) and prints `?`.
- **Diagnostics.** Errors produce `?` on the output; `h`/`H` optionally print a message.
  Exit codes: `0` success, `1` an error occurred during editing, `2` usage/invalid invocation.
- **File semantics.** `e`/`E` edit a named file, `f` sets/prints the remembered filename, `r`
  reads a file into the buffer, `w`/`W` write (whole / append). The `e !cmd` and `r !cmd`
  forms **read from a shell command** (see §3.1).
- **Command set.** Includes `a c d e E f g G h H i j k l m n p P q Q r s t u v V w W x y z = !`
  and the line-addressing grammar (`.`, `$`, numbers, `+`/`-`, `/re/`, `?re?`, `;`, marks).
  Notable hard cases: `g`/`G`/`v`/`V` global mark-then-execute semantics; `s` with `%`, `g`,
  count, and confirmation flags; `u` (single-level) undo; `l` (list) backslash-escapes.

### 3.1 The `!` shell escape — a direct conflict with the no-shell-out rule

The POSIX `ed` `!` command (and the `e !cmd` / `r !cmd` read-from-shell forms) **execute a
shell command**. **[Sourced]** to the standard. This collides head-on with the repository
rule that no tool spawns a program to implement its own behavior (the one documented exception
covers wrappers like `env`/`timeout`/`xargs` whose purpose *is* running the operand).

**Resolution (inference).** Keep the two surfaces separate:
- The **POSIX `ed` CLI** may implement `!` as an explicitly documented, off-by-default
  extension gated behind a flag/env (e.g. `ED_ALLOW_SHELL=1`), failing loudly (`ed: '!'
  disabled`) otherwise — consistent with the repo's "unsupported fails loudly" contract.
- The **agentic patch/edit surface must never expose `!`** or any command-execution primitive.

This must be decided by maintainers; it is flagged as an open design decision, not silently
approximated.

---

## 4. In-repo prior art (the decisive correction)

The rejected draft treated the BRE/backreference problem as an *open* limitation requiring an
*"external BRE package."* That is wrong: **the problem is already solved in this tree.**

### 4.1 `pkg/bre` — POSIX BRE with bounded-backtracking backreference support **[Sourced]**

`pkg/bre/bre.go` (verified by reading the source):

> *"Package bre matches POSIX Basic Regular Expressions (plus the common GNU extensions),
> shared by the pure-Go tools that default to BRE (grep, sed). Patterns without
> back-references are translated to Go RE2 syntax; patterns with `\1`..`\9` or word-edge
> anchors use a bounded backtracking matcher."*

Supported constructs include `\( \)`, `\|` (GNU alternation), `\{m,n\}`, `\+`/`\?`, bracket
expressions with `[:class:]`, and the GNU `\w \W \s \S \b \B` classes. Matching is
leftmost-longest (POSIX semantics) after `(*Regexp).Longest`.

**Why this matters.** Go's standard `regexp` is RE2-based and **cannot match** a backreference
pattern such as `\(a\)\1` at all — RE2 omits backreferences by design to guarantee linear
time. POSIX BRE *requires* them (§3). `pkg/bre` already bridges exactly this gap, and is
already the engine behind `grep` and `sed`. `ed` should use `pkg/bre` directly. **The BRE
backreference gap is not an open problem.** (This is the single most important correction to
the prior draft; see §9.)

### 4.2 `cmds/diff` — Myers engine **[Sourced]**

`cmds/diff/diff.go` and `cmds/diff/myers.go` provide a line-level Myers diff with FIFO-based
streaming (`diff_fifo_unix_test.go`). This is a reusable building block for *generating*
unified diffs for the agentic surface's dry-run/preview output.

### 4.3 `cmds/sed` — address/range and BRE integration **[Sourced]**

`cmds/sed` already implements line addressing, address ranges, and `s///` semantics over
`pkg/bre`. Its address-parser and substitution-replacement logic are directly applicable to
`ed`'s `s` command and address grammar.

### 4.4 Command registration **[Sourced]**

`cmds/all/all.go` blank-imports each command package to register the multicall set; a new
`cmds/ed` package follows the same pattern.

### 4.5 What does *not* exist yet **[Sourced]**

There is no `cmds/ed` and no `cmds/patch`. Confirmed by directory listing.

---

## 5. Prior-art survey: Go `ed` implementations

Five candidates investigated; four confirmed to exist, one relevant negative.

| Project | License | State (as verified) | Verdict |
|---|---|---|---|
| `thimc/ed` | MIT **[Sourced]** github.com/thimc/ed | Most POSIX-complete of the set; aims at bug-for-bug fidelity. **[Inferred]** Caveats: built on Go `regexp` (RE2) ⇒ **no backreference matching** (fails `\(a\)\1`), so it is *not* POSIX-BRE-faithful as-is; distributed as a CLI binary, not a library; active refactor on a `new` branch ⇒ unstable to pin. | **Reference / study only.** Useful as a command-semantics oracle; not adoptable due to RE2 + instability + not-a-library. |
| `mtimkovich/em` | BSD-2-Clause **[Sourced]** github.com/mtimkovich/em | "A fan remake of ed." Small, clean, real (two core files). | **Study reference** for buffer/command-structure ideas; too thin to adopt. |
| `LihatRadu/newed` | **No license** **[Sourced]** (empty README; repo present) | "Modern improved ed"; structured packages but ships a committed `.exe`; zero community adoption. | **Exclude.** No license ⇒ cannot adopt or adapt. |
| `maksimKorzh/ed` | **No license** (LICENSE link 404) **[Sourced]** | Educational, based on Kernighan's tutorial; tiny command subset. | **Exclude.** No license; not POSIX-complete. |
| `ichiban/ed`, `onefile/ed` | — | 404 / do not exist **[Sourced]**. | N/A. |
| `u-root` (`cmds/core`) | BSD-3-Clause | **No `ed` command exists** in the flagship Go userland **[Sourced]** (directory listing). | **Useful negative:** the ecosystem's reference busybox has no permissively-licensed ed to inherit. |

**Survey inference.** There is no production-grade, permissively-licensed, POSIX-faithful
pure-Go `ed` suitable for adoption. Clean-room implementation is the only viable path (§8),
and `pkg/bre` makes it substantially cheaper than the ecosystem norm.

---

## 6. Agentic requirements for the patch/edit surface

These are properties an *autonomous LLM agent* needs from a file-mutation tool. POSIX `ed`
and POSIX `patch` provide some incidentally; none provide all. The agentic surface is designed
to provide all of them by construction. (Several were *missing or mis-specified* in the prior
draft — see §9.)

1. **Deterministic, locale-free output.** `LC_ALL=C` semantics; no color/terminal/locale
   variance (repo-wide agent contract).
2. **Transactional multi-file edits.** A single edit set either fully applies (all targets)
   or applies to none. Partial application is a failure.
3. **Content-hash preconditions.** Each target carries a precondition (e.g. the current
   content's hash). Apply is refused — without writing — if the precondition does not match,
   enabling safe retries and *idempotence*.
4. **Idempotent re-application.** Re-running the same edit set on already-applied content
   succeeds (no-op) rather than producing drift or double-edit.
5. **Dry-run.** Execute entirely in memory; emit a deterministic unified diff (via
   `cmds/diff`/Myers) of *what would change* without touching disk.
6. **Deterministic diff output.** The diff emitted for preview/audit is stable across runs
   for identical inputs (canonical hunk ordering, deterministic context).
7. **Atomic writes + metadata policy.** Write to a temp file in the same directory and `rename`;
   preserve or explicitly normalize mode/owner/timestamps (defined, not accidental).
8. **Rollback.** On any per-file or global failure, restore prior state; never leave a target
   half-written.
9. **Audit / provenance.** Emit a structured record of what was applied (target, before/after
   hash, applied hunks/edits) for the agent's transcript.
10. **Conflict behavior, defined explicitly.** When a hunk does not apply (context mismatch),
    the tool must fail *loudly* and report which hunk — never silently approximate, per repo
    rule 3.
11. **Path & symlink safety (correct, not `filepath.Clean`).** Containment requires resolving
    each path to an absolute path **within the allowed workspace root**, rejecting or
    resolving symlinks, and guarding against symlink races (per-component checks,
    `O_NOFOLLOW`-style opens). `filepath.Clean` is **not** containment —
    `filepath.Clean("../../etc/passwd")` returns `"../../etc/passwd"` unchanged, and it does
    not resolve symlinks. **[Inferred]** from the Go stdlib semantics of `filepath.Clean`.
12. **Resource limits.** Bounded buffer/hunk sizes and line counts; fail loudly on exceed,
    steering the agent to stream tools (`sed`, `tr`).
13. **Structured Go API and CLI.** An in-process API (`ApplySet`, `Diff`, `DryRun`) usable by
    embedding consumers, plus a CLI wrapper over `tool.RunContext`.
14. **No command execution.** The agentic surface exposes no `!`/shell primitive (contrast
    POSIX `ed`, §3.1).

> These requirements define a *new* tool. It is **not** "ed with safety" and **not** "patch
> with preconditions"; conflating it with either is the core design error to avoid (§2, §9).

---

## 7. The BRE backreference question — fully resolved

The rejected draft (§10.1) framed this as an open risk: *"Go's regexp uses RE2, which does
not support backreferences … POSIX BRE rely on backreferences … use an external BRE package or
document this as a known limitation."* This is the draft's most important technical error.

**Correction (sourced).**

- In `ed`'s `s/RE/replacement/`, there are two distinct backreference mechanisms:
  - **Matching backreference** — `\1`…`\9` used *inside the search RE* to match a repetition
    of an earlier group, e.g. `\(a\)\1`. Go `regexp` (RE2) **cannot do this at all**. **This
    is the real gap.**
  - **Replacement backreference** — `\1` in the replacement text references the Nth captured
    group. This is universal and supported by Go via `regexp.Expand`/template syntax. There
    was never a gap here.
- `pkg/bre` already provides a **bounded backtracking matcher** for patterns containing
  `\1`…`\9` (it routes backreference patterns away from RE2), and is already used by `grep`
  and `sed`. **Therefore `ed` reuses `pkg/bre` and the gap is closed in-repo.**
- The only remaining nuance is **leftmost-longest** POSIX semantics: `pkg/bre` opts into
    `(*Regexp).Longest`, which `ed`'s `s` command must honor (the longest leftmost match is
    replaced). **[Inferred]** to be wired through the same call path sed already uses.

**Bottom line:** BRE backreferences are **not** a blocker and should not be listed as an open
limitation. The draft's framing is corrected.

---

## 8. Implementation approach: adopt / fork / compose / clean-room

For **POSIX `ed`**:

- **Adopt:** Not viable. No permissively-licensed, POSIX-faithful, library-shaped Go `ed`
  exists (§5). The closest (`thimc/ed`) is RE2-based (no backref matching), unstable, and a
  CLI not a library.
- **Fork:** Not viable. Adapting a toy clone to full POSIX would cost more than rewriting,
  and the two usable ones are thin.
- **Compose:** Partial. Reuse `pkg/bre` (regex), `cmds/sed` (address/`s` patterns), and
  `cmds/diff` (diff generation) as *components*, but the `ed` buffer/command state machine
  itself must be written.
- **Clean-room (recommended):** New `cmds/ed` state machine built on `pkg/bre`, following the
  `tool.RunContext` registration pattern. MIT-licensed by virtue of being original code;
  permissive prior art used only as *study references* (§5), never copied.

For the **agentic patch/edit surface**:

- **Compose (recommended).** Build on `go-gitdiff` (§10) for unified-diff *parse + apply*
  with its strict-mode `*Conflict` type, layer on the precondition/dry-run/atomic-write/
  rollback/audit requirements (§6), and emit previews via the in-repo Myers engine
  (`cmds/diff`). This is far cheaper than reimplementing diff application, and go-gitdiff is
  MIT and actively maintained.

---

## 9. Correction log: defects of the rejected prior draft

The rejected draft (located at
`…/workspaces/issue-254/docs/ed-agentic-feasibility.md`, read-only reference) contained
several factual and design errors. This study corrects each:

| # | Draft defect | Correction |
|---|---|---|
| D1 | Implied the BRE backreference gap is *open* and needs an "external BRE package," listed as a known limitation (§10.1). | **False.** `pkg/bre` already provides bounded-backtracking backreference matching and is used by `grep`/`sed` **[Sourced]** (§4.1, §7). |
| D2 | Conflated `ed`, POSIX `patch`, and unified diff into a single "mutate files" concern (§4). | They are distinct surfaces with distinct contracts (§2). The agentic surface is a *new* tool, not "ed+patch". |
| D3 | Proposed `filepath.Clean` as a "boundary" preventing path traversal (§5.1.3). | `filepath.Clean` is **not** containment; it neither stops `../` escape nor resolves symlinks (§6.11). |
| D4 | Proposed enabling the POSIX `!` shell escape via an env var without reconciling it with the no-shell-out rule (§5.1.4). | The conflict is explicit (§3.1); the agentic surface never exposes `!`; the POSIX CLI treats it as an off-by-default extension pending maintainer decision. |
| D5 | Relied on "the standard `regexp` package" for `g/G/v/V/s` (§7 matrix). | Must use `pkg/bre` for POSIX BRE (incl. backref matching) **[Sourced]**. |
| D6 | Understated `go-gitdiff`'s capability (described it vaguely as supporting "precondition hashes"). | `go-gitdiff` **parses and applies** git/unified diffs via `gitdiff.Apply`, with strict-mode `*Conflict`; it does **not** do content-hash preconditioning (that is the agentic layer's job) **[Sourced]** (§10). |
| D7 | Estimated effort in single-day point numbers (§8). | Person-week ranges with phased scope (§11). |
| D8 | Test strategy risked reproducing licensed gate content via a host-`ed` oracle (§9). | Coverage is reproduced from **public POSIX semantics**, not from the licensed `671-TP` artifacts (§12). |
| D9 | No evidence-class labeling. | All claims tagged **[Sourced]** vs **[Inferred]** with citations throughout. |

---

## 10. Prior-art survey: Go diff/patch libraries

Five libraries investigated; categorized by *capability* (generate / parse / apply), which is
the dimension that actually matters for design.

| Library | License | Capability | Notes **[Sourced]** |
|---|---|---|---|
| `bluekeyes/go-gitdiff` | MIT | **Parse + Apply** | v0.9.0 (2026-07-18). Applies git/unified diffs via `gitdiff.Apply(dst, src, f, opts)`; strict-mode produces `*Conflict`; actively maintained. github.com/bluekeyes/go-gitdiff, pkg.go.dev/github.com/bluekeyes/go-gitdiff. **Best fit** for the agentic surface's diff-application core. |
| `sergi/go-diff` | MIT (Go port) / Apache-2.0 (original Google diff-match-patch) | **Generate (diff-match-patch) + Apply (own patches)** | Port of Google's diff-match-patch; character-level `DiffMain` + `PatchApply`. **Not** unified-diff-native (its "patches" are DMP text, not `diff -u` hunks). Mature, widely depended-upon. Recency not directly re-verified this pass **[Inferred]**. |
| `hexops/gotextdiff` | BSD-3-Clause (golang/tools LICENSE) | **Generate (unified)** | Port of `gopls`'s internal diff package; emits unified diffs from Myers diffs. Generate-only (no apply). |
| `pmezard/go-difflib` | BSD-3-Clause | **Generate (unified/context)** | Port of Python `difflib`. **Explicitly "NO LONGER MAINTAINED"** (archived). Generate-only. |
| `sourcegraph/go-diff` | MIT | **Parse + Print** | Unified-diff parser + printer only; "doesn't actually compute a diff." No apply. |

**Selection inference.** `go-gitdiff` is the clear choice for *applying* unified diffs (parse
+ apply + conflict semantics). For *generating* preview/dry-run diffs, prefer the in-repo
`cmds/diff` (Myers) to avoid a new dependency; `hexops/gotextdiff` is a clean fallback if a
pure-unified emitter is wanted. `pmezard/go-difflib` is excluded (unmaintained).

---

## 11. Phased effort estimate (person-week ranges)

Ranges reflect clean-room work leveraging `pkg/bre`, `cmds/sed`, and `cmds/diff`; they are
planning estimates, not commitments.

### A. POSIX `ed` (~8–12 person-weeks)

| Phase | Scope | Estimate |
|---|---|---|
| A1 — Buffer & addressing | In-memory line buffer; line addressing (`.`,`$`,numbers,`+`/`-`,`/re/`,`?re?`,`;`,marks); commands `p n l = q Q h H P f`. `tool.RunContext` wiring + `cmds/all` registration. | 2–3 pw |
| A2 — File I/O & edit commands | `e E r w W` with safe-path handling; text mutation `a c i d j m t u` (incl. single-level undo). | 1.5–2 pw |
| A3 — Global commands | `g G v V` mark-then-execute semantics; undo interactions. | 1–1.5 pw |
| A4 — Substitute & BRE | `s` with flags (`g`, count, `%`); `pkg/bre` integration; leftmost-longest; null-RE reuse; replacement backrefs via `Expand`. | 1–1.5 pw |
| A5 — POSIX edge cases | signals/EOF; modified-flag tri-state; no-newline warnings; `?` diagnostics; exit codes; `z`; `y`. | 1–1.5 pw |
| A6 — Tests | Public clean-room oracles + table tests + fuzz (§12). | 2–3 pw |

### B. Agentic patch/edit surface (~3–5 person-weeks)

| Phase | Scope | Estimate |
|---|---|---|
| B1 — Core apply | `go-gitdiff` parse+apply; precondition (hash) check; conflict → loud failure. | 1–1.5 pw |
| B2 — Safety | Path/symlink containment; atomic temp+rename write; metadata policy; resource limits. | 1–1.5 pw |
| B3 — Agent UX | Dry-run unified-diff preview (`cmds/diff`); transactional multi-file; rollback; audit record; idempotent re-apply. | 1–1.5 pw |
| B4 — API + CLI + tests | In-process Go API + CLI over `tool.RunContext`; property/fuzz tests. | 0.5–1 pw |

---

## 12. Test strategy: public clean-room coverage (no licensed reproduction)

The `671-TP` gate is a **licensed** conformance corpus used internally as an acceptance
target. Its content is licensed and **must not be copied or paraphrased into the repo**.
Coverage equivalent in intent is derived from **public** POSIX semantics:

1. **Standards-derived oracles.** Generate expected input/output pairs directly from the
   POSIX.1-2024 `ed` description (§3) and XBD §9.3 (BRE). Each test cites the standard clause
   it exercises. This is independent authorship, not reproduction.
2. **Differential oracle (optional, dev-only).** A *developer-local* script may drive a host
   `ed` (macOS/BSD/GNU) to produce expected outputs, then hard-code those pairs into
   `ed_test.go`. The **generated test data** is facts (I/O pairs), not licensed source; the
   script and any host-binary dependency stay out of CI. **[Inferred]** — confirm with
   maintainers that generated I/O pairs are acceptable; do not commit host-binary artifacts.
3. **Table-driven coverage** for every command, flag combination, address form, exit code,
   and diagnostic path; C-locale deterministic outputs.
4. **Fuzzing** the command parser and `pkg/bre` to guarantee no panic on malformed input or
   hostile regex (catastrophic backtracking is already bounded by `pkg/bre`).
5. **`perfbench` end-to-end** run against standard POSIX edge cases per repo convention.

---

## 13. Risks and mitigations

| Risk | Mitigation |
|---|---|
| **`g`/`G`/`v`/`V` + undo interaction bugs** (classic ed hazard) | Dedicated table tests; treat global-command marking as a first-class, tested subsystem (A3). |
| **Leftmost-longest vs leftmost-first mismatch** in `s` | Route through `pkg/bre`'s `Longest` path exactly as `sed` does; cross-test against `sed` results. |
| **`!` shell escape in POSIX mode** vs no-shell-out rule | Off-by-default gated extension; agentic surface never exposes it (§3.1). |
| **Symlink/path-traversal escape** | Real containment (resolve + reject symlinks + per-component checks), not `filepath.Clean` (§6.11). |
| **Memory blowup on huge files** | Hard limits with loud failure steering to stream tools (§6.12). |
| **Catastrophic regex backtracking** | Already bounded by `pkg/bre`'s backtracking matcher; fuzz-verified. |
| **Conformance drift vs the licensed gate** | Standards-derived oracles + maintainer sign-off; never gate on copied licensed content (§12). |

---

## 14. Evidence log

Primary sources consulted during this study (all **[Sourced]**):

- POSIX.1-2024 `ed` — https://pubs.opengroup.org/onlinepubs/9799919799/utilities/ed.html
- `bluekeyes/go-gitdiff` — github.com/bluekeyes/go-gitdiff ; pkg.go.dev/github.com/bluekeyes/go-gitdiff (v0.9.0, 2026-07-18; `Apply`; `*Conflict` strict-mode)
- `sergi/go-diff` — github.com/sergi/go-diff (diff-match-patch port; MIT/Apache-2.0)
- `hexops/gotextdiff` — github.com/hexops/gotextdiff (gopls internal port; BSD-3)
- `pmezard/go-difflib` — github.com/pmezard/go-difflib ("NO LONGER MAINTAINED"; BSD-3)
- `sourcegraph/go-diff` — github.com/sourcegraph/go-diff (parse/print only; MIT)
- `thimc/ed` — github.com/thimc/ed (MIT; RE2-based CLI; unstable `new` branch)
- `mtimkovich/em` — github.com/mtimkovich/em (BSD-2; fan remake)
- `LihatRadu/newed` — github.com/LihatRadu/newed (no license)
- `maksimKorzh/ed` — github.com/maksimKorzh/ed (no license; LICENSE 404)
- `u-root` `cmds/core` — github.com/u-root/u-root (BSD-3; **no ed** in cmds/core)
- In-repo: `pkg/bre/bre.go`, `cmds/diff/{diff.go,myers.go,diff_fifo_unix_test.go}`,
  `cmds/sed/`, `cmds/all/all.go` (directory listing + source read)

**Excluded from adoption (licensing):** `maksimKorzh/ed`, `LihatRadu/newed` (no license).
**Excluded (maintenance):** `pmezard/go-difflib` (archived/unmaintained).
