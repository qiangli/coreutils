# S79 adversarial review — sed/awk POSIX locale equivalence classes

Independent review of the locale-semantics half of `985bcb5` ("posix: close
sed and file profile C deltas"), which added POSIX bracket-expression
**equivalence classes** (`[[=x=]]`) and **collating elements** (`[[.x.]]`) to
the locale byte substrate that `sed` and `awk` use under a non-C `LC_CTYPE`.

Reviewed surface: `pkg/bre/byte_pattern.go`, `pkg/bre/byte_pattern_ere.go`,
`pkg/bre/byte_regexp.go`, `pkg/ctype/ctype_glibc.go`, `pkg/ctype/ctype_stub.go`,
and the `cmds/sed` / `cmds/awk` locale plumbing.

## Resolution update

F2–F5 and F7 below are retained as the adversarial record of the pre-fix
state. They are resolved by the Sprint 79 locale correction:

- `sed` and `awk` now resolve `LC_CTYPE` and `LC_COLLATE` independently.
  Character classes and case conversion use the former; equivalence classes,
  collating symbols, and ranges use the latter.
- The partial German hand table was removed from `pkg/ctype`. On supported
  glibc systems, `pkg/collate` snapshots every valid single-byte equivalence
  class from the installed locale's POSIX regex provider and copies glibc's
  single-byte collation sequence for ranges. There is no shell-out and no
  imported libc/GNU source or table.
- A non-C collation provider must supply complete equivalence, range-weight,
  and collating-element data. Missing or malformed data is an initialization
  error, not an identity fallback.
- Single-byte `[.x.]` elements are checked against provider validity and may be
  range endpoints. Multi-character collating elements remain unsupported and
  fail closed before input is read.

The bounded implementation remains deliberately limited to glibc on
Linux amd64/arm64 and the installed `de_DE.ISO-8859-1` aliases. Other non-C
locales/platforms fail closed. The normative references are POSIX.1-2008 Issue
7, 2016 edition [XBD 9.3.5](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/basedefs/V1_chap09.html#tag_09_03_05),
[sed](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/sed.html),
and [awk](https://pubs.opengroup.org/onlinepubs/9699919799.2016edition/utilities/awk.html).

Authority: POSIX.1 Issue 7 (2016) XBD 9.3.5 (RE Bracket Expression) and the
`sed`/`awk` ENVIRONMENT sections, per `docs/reference-policy.md`.

## Verdict

One confirmed silent-wrong-answer defect (**F1**, fixed in this commit, with
regression tests at all three layers). Two open semantic gaps (**F2**, **F3**)
that are design decisions, not slips — documented here rather than changed
unilaterally, because "fixing" either means asserting a behavior this repo
cannot yet verify against glibc. Three lower-severity observations follow.

The parsing added to `parseLocaleByteBracket` is otherwise sound. It was probed
against `[[.].]]`, `[[=]=]]` (the delimiter-as-content cases), `[[..]]`,
`[[.a.]`, `[[=a=]`, `[a[.b.]]`, `[[.a.][.b.]]`, `[[=a=]-]`, `[^[=a=]]`,
`[[:alpha:][=a=]]`, and `[[.a.]-z]`; the index arithmetic (`i += end + 4`) is
correct against the `for i < len(pattern)` loop, unterminated forms fail
closed, and a collating element used as a range endpoint is refused rather
than silently mis-parsed.

---

## F1 — CONFIRMED, FIXED: `[[=x=]]` matched nothing in every ERE

`pkg/bre/byte_pattern_ere.go:12` hand-copied `bytePatternTables` field by field
and was not updated when `equivalent [256][256]bool` was added to the struct
(`pkg/bre/byte_pattern.go:14`). The BRE compiler's identical hand-copy *was*
updated. The result: in an ERE, every equivalence-class row was all-false, so
`[[=x=]]` compiled cleanly into a character class that matches **no byte at
all** — not even `x`, which is a member of its own class by reflexivity.

No error, no diagnostic, exit 0. This is precisely the "silent approximation"
that CLAUDE.md rule 2 and the agent contract forbid; the failure is worse than
the pre-change behavior, which refused `[[=x=]]` loudly.

Reproduction before the fix:

```
$ LC_ALL=de_DE.iso88591 sed    's/[[=a=]]/X/' <<< bab   ->  bXb     (correct)
$ LC_ALL=de_DE.iso88591 sed -E 's/[[=a=]]/X/' <<< bab   ->  bab     (WRONG, exit 0)
$ LC_ALL=de_DE.iso88591 awk '/[[=a=]]/ {print "yes"}' <<< bab  ->  (nothing)
```

`awk` is the wider blast radius: it has no BRE mode, so `cmds/awk/awk.go:156`
compiles *every* awk regex with `Syntax: ByteRegexpERE`. Under any non-C
`LC_CTYPE`, no awk equivalence class matched anything.

Why the tests missed it: `TestLocaleByteEREFailsClosed` only asserted that the
*rejected* form `[[=ab=]]` errors. The positive cases added by `985bcb5`
(`TestLocaleBytePatternClasses`) went to the BRE compiler only, so no test ever
asserted that an ERE equivalence class matches anything.

### Fix

The root cause is the duplication, not the missing field: two hand-written
copies of a growing struct, one of which will always be forgotten. Both
compilers now call a single `bytePatternTables.snapshot()`
(`pkg/bre/byte_pattern.go`), so a future field cannot reach one grammar and not
the other.

Regression coverage added at each layer, each verified to fail against the
unfixed compiler:

- `pkg/bre/byte_pattern_equivalence_test.go` —
  `TestBytePatternSnapshotCopiesEveryField` (reflect-based; catches the *next*
  dropped field) and `TestLocaleByteEquivalenceIsGrammarIndependent` (BRE/ERE
  parity for `[[=a=]]`/`[[.a.]]`, including reflexivity and non-membership).
- `cmds/sed/ctype_test.go` — `TestSedLocaleEquivalenceClassMatchesInBothGrammars`.
- `cmds/awk/awk_locale_test.go` — `TestAwkLocaleEquivalenceClassMatches`.

---

## F2 — OPEN: equivalence classes are driven by `LC_CTYPE`, not `LC_COLLATE`

POSIX splits these two categories explicitly. XBD 9.3.5 defines collating
elements and equivalence classes in terms of the **current collating
sequence**, and the `sed` ENVIRONMENT section assigns them to `LC_COLLATE`
("the behavior of ranges, equivalence classes, and multi-character collating
elements within regular expressions"), leaving `LC_CTYPE` responsible for
character *class* expressions. `awk` reads identically.

This implementation routes them through `LC_CTYPE` end to end:

- `cmds/sed/sed.go:153` and `cmds/awk/awk.go:90` resolve `locale.CType`, and
  that one name decides whether the locale substrate is used at all.
- `pkg/ctype` requests `LC_CTYPE` only — `lcCtypeMask = 1 << 0`
  (`pkg/ctype/ctype_glibc.go:21`) — so the provider now answering collation
  questions holds no `LC_COLLATE` handle.

Two divergences from POSIX follow, in both directions:

```
LC_CTYPE=de_DE.ISO-8859-1 LC_COLLATE=C            sed 's/[[=a=]]/X/'
  POSIX: LC_COLLATE=C, so [[=a=]] is [a] and byte 0xe4 must not match.
  Here:  0xe4 matches.

LC_CTYPE=C LC_COLLATE=de_DE.ISO-8859-1            sed 's/[[=a=]]/X/'
  POSIX: German equivalence classes apply; 0xe4 must match.
  Here:  LocaleTables is nil, the C engine runs, 0xe4 does not match.
```

The repo already has both halves needed to do this correctly:
`locale.Collate` (`pkg/locale/locale.go:29`) and `pkg/collate`, whose provider
holds a real `LC_COLLATE` handle alongside its `LC_CTYPE` one. The equivalence
data was attached to the `LC_CTYPE` provider instead.

Note this is only observable when the two categories differ — with `LC_ALL` or
a single `LANG` set, the two agree and nothing is visibly wrong. That is what
makes it easy to ship and easy to miss.

**Recommendation:** resolve `locale.Collate` separately in `sed`/`awk` and
source equivalence classes from an `LC_COLLATE`-backed provider, or — if that
is out of scope for the sprint — refuse `[[=x=]]` with a clear error whenever
`LC_COLLATE` differs from `LC_CTYPE`, so the wrong answer is never silent.

---

## F3 — OPEN: the German equivalence table is internally inconsistent

`Provider.Equivalents` (`pkg/ctype/ctype_glibc.go:343`) is a hand-written table
of eight Latin-1 base-letter groups. Every other classification in this package
is *asked of glibc* over `dlopen`/`dlsym` — that is the package's entire safety
story. This one is a constant, it never consults `p.lib` or the opened
`locale_t`, and it is identical for any admitted non-C locale.

That is arguably unavoidable (glibc exposes no per-character equivalence-class
API; only `strxfrm_l` weights come close), but two things follow that are not
unavoidable:

**a. The table is self-inconsistent.** It groups upper and lower case together
for the eight letters that have Latin-1 accented forms, and not for any other
letter:

```
Equivalents('a') = A a À Á Â Ã Ä Å à á â ã ä å
Equivalents('b') = b                              <- 'B' is absent
Equivalents('d') = d                              <- no group at all
```

Whatever rule places `A` and `a` in one class — primary collation weight, the
only candidate — places `B` and `b` in one class too; nothing distinguishes
them. So `sed 's/[[=a=]]/X/'` is case-insensitive while `sed 's/[[=b=]]/X/'` is
case-sensitive, under the same locale, in the same run. This is a defect under
either reading of the spec, and it needs no glibc to see.

**b. The Latin-1 coverage is unverified and looks incomplete.** No test compares
the table to a real `de_DE.ISO-8859-1` locale. Bytes that plausibly share a
primary weight with an ASCII letter but appear in no group include `0xf0`/`0xd0`
(ð/Ð), `0xfe`/`0xde` (þ/Þ), `0xaa` (ª) and `0xba` (º). These are flagged as
*unverified*, not as confirmed misses — resolving them requires reading glibc's
`iso14651_t1` weights, which cannot be done from this workspace.

The doc comment calls the groups "reviewed", but points at no artifact. Under
`docs/reference-policy.md` and the repo's recent evidence-ledger work, a
hand-written conformance table needs a citable source.

**Recommendation:** either extend the table to all letters and back it with a
generated-from-glibc fixture on a Linux runner, or narrow `[[=x=]]` back to a
loud refusal for non-C locales until real collation data exists. The current
middle state answers some equivalence queries correctly and others wrongly,
with no way for a caller to tell which.

---

## F4 — the `ByteEquivalence` fallback is silent, and its comment is misleading

`pkg/bre/byte_regexp.go:158` type-asserts the provider to `ByteEquivalence` and,
on failure, leaves the identity table. The doc comment justifies this as
retaining "the C-locale behavior where every single byte is equivalent only to
itself."

But this path is only ever reached in a **non-C** locale: `sed`/`awk` build
locale tables exclusively when `LC_CTYPE` is neither `C` nor `POSIX`. So the
fallback does not preserve C-locale behavior — it silently substitutes C-locale
equivalence classes into a locale that asked for something else. Today
`*ctype.Provider` implements the interface so nothing is affected, but the
comment documents an invariant the code does not have, and a future provider
that omits `Equivalents` would degrade silently rather than fail loudly.

**Recommendation:** either require `ByteEquivalence` of any provider used for a
non-C locale, or record on the snapshot that equivalence data was unavailable
and error at compile time on `[[=x=]]`.

## F5 — `[[.x.]]` accepts any byte as a collating element

`pkg/bre/byte_pattern.go:299` treats any single byte between `[.` and `.]` as a
valid collating element. POSIX leaves a collating element that does not exist
in the locale undefined, and glibc errors. Low impact for the only admitted
locale (ISO-8859-1 assigns a character to all 256 bytes), but it means the
substrate cannot report a bad collating element in any future locale.

## F6 — `equivalent` is 64 KiB copied on every compile path

`[256][256]bool` is 65 536 bytes, and `bytePatternTables` is passed by value
through `CompileLocaleByteRegexpTables` → `snapshot` → `translateLocaleByte*` →
`parseLocaleByteBracket` (once per bracket expression). Nothing is *retained*
per compiled pattern, so this is transient compile-time cost only, not a leak —
but it grew the struct ~230x for a field consulted only by `[= =]`, which
almost no pattern uses. `SnapshotLocaleByteTables` also makes 256 extra
provider calls per invocation for the same reason.

Not urgent. If it ever shows up, the fix is to make `equivalent` a pointer to a
shared immutable table (it is never mutated after `buildBytePatternTables`), or
a `map[byte][256]bool` populated only for bytes with non-trivial classes.

## F7 — the feature is undocumented

`docs/` contains no mention of equivalence classes or collating elements, and
the `sed` row of `docs/applet-matrix.tsv` was not touched. Per the repo
convention that new capability lands with a doc line, the supported subset
(single-byte collating elements; multi-character elements refused; ranges still
refused) should be stated somewhere a consumer will find it.

---

## Independent verification record

Every claim above was re-checked against the tree rather than taken from the
original review pass. Each item below states the check that was actually run,
so a later reader can reproduce it instead of trusting the verdict.

**F1 — CONFIRMED by mutation, not by inspection.** `985bcb5` is confirmed to
touch `pkg/bre/byte_pattern.go` (adding `equivalent`) and *not*
`pkg/bre/byte_pattern_ere.go` (`git show 985bcb5 --stat`), so the ERE hand-copy
never learned the new field. Reverting `snapshot()` back to that hand-copy makes
all three new regression tests fail, at all three layers:

```
ERE "[[=a=]]" against 0x61 = false, want true   <- not even reflexive
ERE "[[=b=]]" against 0x62 = false, want true
sed:  got=("bab\n","",0) want="bXb\n"           <- exit 0, empty stderr
awk:  got=("","",0)      want="yes\n"
```

The `sed`/`awk` rows are the load-bearing evidence: exit code 0 and an empty
diagnostic alongside a wrong answer is exactly the silent-approximation failure
the agent contract forbids, and it is now pinned in both grammars.

**The `snapshot` guard test was itself mutation-tested.** A guard that cannot
fail is not a guard. `syntheticBytePatternTables` populates `equivalent`
non-trivially (identity diagonal plus `equivalent['a'][0xe9]`), and
`dotAll`/`multi` are set by the test, so the `reflect.DeepEqual` assertion has
real signal: making `snapshot()` zero `equivalent` fails
`TestBytePatternSnapshotCopiesEveryField` as intended.

Known limit of that guard, stated so it is not over-trusted: `DeepEqual` proves
no field was *dropped*, but it cannot see a future *reference-type* field that
`out := t` copies as an alias. Only `classes` has an explicit aliasing
assertion. A new map/slice/pointer field in `bytePatternTables` needs its own
aliasing case added here.

**Fix scope is minimal and behavior-preserving for BRE.** The old BRE hand-copy
and `snapshot()` copy the same fields, so BRE is unchanged; only ERE gains the
previously-dropped table. No compiler writes to `tables.classes` after
snapshotting (reads at `byte_pattern.go:43,47,149,289`,
`byte_pattern_ere.go:13,17,58`), so the map copy is defensive depth against
caller mutation, as its doc comment claims.

**F2 — CONFIRMED at the source.** `cmds/sed/sed.go:153` and `cmds/awk/awk.go:90`
gate the *entire* locale substrate on `locale.Resolve(rc.Env, locale.CType)`,
and `pkg/ctype/ctype_glibc.go:21` requests `lcCtypeMask = 1 << 0` only. Both
divergences described above follow directly from that. `pkg/collate` does hold a
real `LC_COLLATE` handle (`lcCollateMask = 1 << 3`), so the correct data source
exists and is simply not wired.

**F3(a) — CONFIRMED, and it needs no glibc.** The table at
`pkg/ctype/ctype_glibc.go:353` is eight groups, each pairing upper with lower;
every other byte falls through to `return []byte{c}`. So under one locale in one
run, `[[=a=]]` matches `A` and `[[=b=]]` does not match `B`. This is the finding
most worth acting on next: unlike F2 it is decidable from the table alone.
F3(b) is correctly labeled *unverified* rather than confirmed — the Latin-1
coverage question genuinely cannot be settled from this workspace.

**F4 — CONFIRMED.** `buildBytePatternTables` seeds the identity diagonal first
and augments it only `if provider, ok := provider.(ByteEquivalence); ok`
(`pkg/bre/byte_regexp.go:155-168`). A provider without `Equivalents` therefore
yields C-locale equivalence inside a non-C locale, silently. F5 and F7 also
confirmed as written (`[[.x.]]` accepts any byte; no user-facing doc mentions
the feature).

## F8 — the change was not gate-clean as handed over (fixed here)

`scripts/crossvet.sh` failed with `applet matrix was stale`. The cause was this
change's own new tests: `docs/applet-matrix.tsv` counts per-command test
functions, so adding one to `cmds/sed` and one to `cmds/awk` moved sed 61→62 and
awk 20→21. Regenerated with `scripts/applet-matrix.py`; both `.tsv` and `.md`
are included in this commit.

Worth noting for the next agent: the matrix check runs *before* the GOOS legs
and short-circuits them, so a stale matrix masks the cross-OS result entirely —
a green-looking early exit is not a green gate. Read `CROSSVET_EXIT`, not the
last line.

## Gate

- `go vet $(go list ./... | grep -v /external/)` — PASS
- `scripts/crossvet.sh` — PASS (windows, linux, darwin, aix)
- `go test $(go list ./... | grep -v /external/)` — PASS except `pkg/chat`
  (`TestInvokeEmitsNestedTier1SpansWithoutContent`,
  `TestInvokeUsesSeededHeadlessContract`, `TestInvokeCanOverrideCodexSandbox`).
  Verified **pre-existing and unrelated**: the identical three failures
  reproduce on a stashed, clean tree. They are this host's agent-launch sandbox
  guard refusing `codex --dangerously-bypass-approvals-and-sandbox`, not a
  consequence of any change here.
