# Prior art (`priorart/`, gitignored)

Moved verbatim from `CLAUDE.md` on 2026-09-20.

Local clones of permissive open-source reimplementations, for studying
and — where the license allows — adapting. **Conformance is judged
against the original command's official documentation, never against
prior art**: these projects have their own gaps and deviations (u-root
is deliberately flag-partial; aict changes output formats), so anything
copied must be verified against the GNU manual / POSIX and covered by
tests before it counts as supported.

| Clone | Project | Lang | License | Policy |
|---|---|---|---|---|
| `priorart/aict` | aict (agent-oriented coreutils, XML/JSON output) | Go | MIT | copy/adapt |
| `priorart/guonaihong-coreutils` | guonaihong/coreutils | Go | Apache-2.0 | copy/adapt |
| `priorart/u-root` | u-root/u-root (`cmds/core/…`) | Go | BSD-3-Clause | copy/adapt |
| `priorart/coreutils` | microsoft/coreutils | Rust | MIT | reference only |
| `priorart/uutils-coretuils` | uutils/coreutils | Rust | MIT | reference only (best GNU-fidelity reference) |

When adapting code from a copy/adapt clone:

- Keep a provenance header on the file: source repo, path, license.
- Add the source to `THIRD_PARTY_LICENSES.md` (license text included).
  Apache-2.0 sources (guonaihong) additionally require stating changes.
- Strip anything that violates the agent contract while adapting —
  Linux-only assumptions (u-root), non-GNU output modes (aict's
  XML/JSON), locale-dependent behavior.
- aict's XML/JSON output idea is explicitly NOT adopted as default
  behavior — upstream tools don't do it, and rule 3 forbids changing
  output shapes under upstream names. If structured output ever lands,
  it will be an explicitly documented extension flag.

The Rust clones are semantic references: uutils is the most
GNU-faithful reimplementation in existence and is often the fastest way
to resolve "what does GNU actually do here" questions — but code flows
from it only as understanding, never as translation (reference-only by
policy to keep provenance simple, even though its license would allow
more).

