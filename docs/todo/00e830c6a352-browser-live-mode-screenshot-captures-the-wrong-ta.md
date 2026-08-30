---
id: 00e830c6a352
kind: task
title: 'browser live mode: screenshot captures the wrong tab and click-by-index silently no-ops, both reporting success'
seq: 8
status: todo
priority: p1
created: 2026-08-30T18:02:59.062638Z
---

SPEC: coreutils/pkg/browser (live/, solo/, probe/, wire/). RELATED: SPRINT #87 (epic bashy-yoke) defect class B/C — an acknowledgement that overstates what happened. This item is the NON-COMMS instance and is NOT gate-blocked: browser is shipped code, not Yoke code.

OBSERVED (2026-08-30, ~40 min driving a local React dashboard on localhost:5478 through `browser --mode live`; bashy 5.3.0(1)-bashy-dev 968e6c0, Chrome + MV3 extension, hub pid 76883 on 127.0.0.1:58082, 51 tabs open):

1. screenshot CAPTURES THE FOREGROUND TAB, NOT THE DRIVEN TAB.
   `browser --mode live --json navigate "http://localhost:5478/chat/triage-demo"`
     -> {"success":true,"title":"Kiro Crew","url":"http://localhost:5478/chat/triage-demo"}
   `browser --mode live --json extract`   -> correct DOM for that page
   `browser --mode live screenshot`       -> a PNG of a DIFFERENT tab ("Board - bashy", 127.0.0.1:8639)
   navigate/extract/click resolve one tab; screenshot resolves another. No error, no warning.

2. screenshot ALSO RETURNS A STALE FRAME (separable from 1).
   After two clicks that `extract` confirmed had landed (aria-label flipped to "Close panel";
   sidebar toggle flipped to "Show sessions sidebar"), three consecutive screenshots still
   showed the pre-click state. DOM said done, image said not-done.

3. click BY ELEMENT INDEX IS A SILENT NO-OP.
   `browser --mode live --json extract`
     -> [36] <div role="button" aria-label="Open activity panel">Open activity panel</div>
   `browser --mode live --json click 36`
     -> {"success":true}                      # nothing happened; aria-label unchanged
   `browser --mode live --json click '[aria-label="Open activity panel"]'`
     -> {"success":true,"content":"clicked"}  # panel opens
   Note the envelopes differ: the index form omits "content". That is likely the tell that no
   element was resolved -- in which case it should have been success:false.

4. screenshot SILENTLY DISCARDS A PATH ARGUMENT AND EXITS 0.
   `browser --mode live screenshot /tmp/shot-test.png >/tmp/o 2>/tmp/e`
     -> exit=0, file created: NO, stdout: 223117 bytes of base64, stderr: empty
   Contradicts the documented house rule ("anything else fails with a clear error (exit 2),
   never a silent guess"). 223KB of base64 on stdout is also hostile to an agent caller: it
   lands in a context window unless every call is redirected defensively.

WHY THIS IS THE EXPENSIVE CLASS: each of these returns a well-formed success. (1) and (2) hand
back a real PNG of the wrong thing, so they fail plausibly rather than loudly. In this session
they produced two successive WRONG root causes -- first "the app did not load", then "a
responsive breakpoint is hiding the panel" -- and cost roughly 25 of the 40 minutes. A wrong
answer that looks right is worse than an error.

REMEDIES:
- All live-mode subcommands resolve the target tab from ONE shared source of truth.
- screenshot waits for a paint after the last mutating command, or exposes an explicit settle flag.
- `screenshot --output PATH` writes a PNG and prints only the path; `--tab <id>`; `--full-page`.
- Base64 to stdout only when stdout is not a TTY, or behind an explicit flag.
- Unrecognised positional args exit 2 naming the argument.
- A click resolving no element returns success:false with a reason; both forms share one envelope shape.
- Indices from `extract` resolve in `click`, or the index form is removed and documented as removed.

TESTS THAT WOULD HAVE CAUGHT IT:
- Two tabs open, navigate the background one, assert the captured image is the navigated page.
- Click a toggle, screenshot, assert captured pixels reflect the post-click DOM.
- Assert click-by-index and click-by-selector return the same envelope shape for the same element.

NON-SCOPE: solo/probe mode behaviour (not exercised); the MV3 extension's permission model.
