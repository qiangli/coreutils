---
id: 0d3a822e3c82
kind: task
title: 'browser: uncapturable tty logging, a status verb that denies a working mode, and refusals that name no remedy'
seq: 9
status: done
priority: p2
created: 2026-08-30T18:03:28.188342Z
closed: 2026-08-30T22:48:23.73455Z
---

SPEC: coreutils/pkg/browser. SIBLING: item 00e830c6a352 (the success-that-did-not-happen cluster). RELATED: SPRINT #87 defect class C — "a refusal that names no remedy is indistinguishable from a reachability failure". These are the same class on the browser surface.

OBSERVED (2026-08-30, same session as 00e830c6a352):

1. THE HUB LOG LINE ESCAPES BOTH stdout AND stderr.
   `browser --mode live --json tabs list >/tmp/o.txt 2>/tmp/e.txt`
     terminal still shows: 2026/08/30 10:57:35 INFO live: hub already owned by another
                           process; using client role port=58082
     /tmp/e.txt -> EMPTY.  /tmp/o.txt -> clean JSON.
   It is being written to the controlling terminal, bypassing both file descriptors. No caller
   can capture, suppress or redirect it. It fires on EVERY invocation, and "hub already owned,
   using client role" is the NORMAL case, not news.
   Remedy: nothing writes to /dev/tty; demote to debug; `--json` implies quiet (stdout carries
   exactly one JSON document). Test: both streams redirected -> terminal receives zero bytes.

2. `status` REPORTS LIVE MODE UNSUPPORTED WHILE LIVE MODE WORKS.
   `browser --mode live --json status`
     -> {"message":"mode \"live\" is not supported","mode":"live",
         "probe_url":"http://localhost:9222","reachable":false}
   `browser --mode live --json tabs list`   -> {"success":true, ...}  # 51 tabs, works
   COST: on that message I concluded live mode was unavailable in this build and fell back to
   driving a separate headless Chrome through a different tool. Live mode had been working the
   whole time. This is precisely class C -- a refusal that misroutes the caller to a weaker
   channel. Note also `probe_url` is reported in live mode, where it is meaningless.
   Remedy: status implements live (hub port, extension connected y/n, active tab, tab count);
   mode-irrelevant fields omitted rather than defaulted; no subcommand claims a mode is
   unsupported when sibling subcommands support it.

3. INVALID ACTION ERRORS DO NOT NAME THE VALID SET.
   `browser --mode live --json tabs 4` -> {"success":false,"error":"unknown tab action: 4"}
   Discovery took eight guesses: list/switch/new/close work; activate/select/focus/current are
   all rejected with the same opaque line. The error knows the action is invalid, so it knows
   the valid set. Remedy: `unknown tab action "4"; expected one of: list, switch, new, close`.

4. NO PER-SUBCOMMAND HELP.
   `diff <(browser tabs --help) <(browser --help)` -> IDENTICAL
   Fourteen subcommands share one help page that documents none of them. This is the root cause
   of (3) and of item 00e830c6a352 finding 4 both costing time: `tabs`'s vocabulary and
   `screenshot`'s output contract are undocumented, so both had to be found by experiment.

5. `--json` RETURNS DISPLAY STRINGS, NOT DATA.
   `browser --mode live --json tabs list`
     -> {"success":true,"content":"[1] DHNT.io\n    https://...\n[2] Board - bashy\n    ..."}
   Bracket numbering, newlines and four-space indent are presentation. Under --json this should
   be an array of {id,title,url,active}. A caller has to re-parse the pretty-printer's output to
   recover structure the tool already had. Same problem in `extract`'s "elements" field.

6. TABS ARE ADDRESSABLE ONLY BY INDEX.
   The live browser held 51 tabs. Indices shift whenever a tab opens or closes, so an index
   captured at the start of a script is stale by the end -- a race no caller can win.
   Remedy: `tabs switch --url <substring>` / `--title <substring>`; ambiguous matches fail with
   the candidate list rather than picking one.

7. STRICT CSP DISABLES `eval` ENTIRELY, WITH NO ALTERNATIVE.
   `browser --mode live --json eval "window.dispatchEvent(new Event('toggle-activity-panel'))"`
     -> {"success":false,"error":"Evaluating a string as JavaScript violates the following
         Content Security Policy directive because 'unsafe-eval' is not an allowed source of
         script: script-src 'self' 'unsafe-inline' ..."}
   Any app omitting unsafe-eval -- increasingly the default -- loses eval. The workaround (find
   a DOM element that happens to trigger the same handler) is not always available.
   Remedy: a subcommand that dispatches a named event to window/document/selector, implemented
   via the extension's isolated world or CDP so page CSP does not apply; and have eval's error
   name that remedy instead of only quoting the policy.

8. `extract` REPORTS NO VIEWPORT CONTEXT.
   When a panel did not appear, the two candidate causes were a responsive breakpoint and a
   stale screenshot. With eval blocked by (7) there was no way to read window.innerWidth, so
   neither could be tested; several minutes went to the wrong hypothesis before item
   00e830c6a352 finding 1 turned out to be the cause. One integer in the envelope settles it.
   Remedy: envelope carries viewport {width,height,dpr}; elements carry visibility + bbox;
   optional --include-hidden to separate absent from not-displayed.

DOCS: docs/browser.md is 105 lines and describes the three modes well, but states none of the
above. Add a capability matrix (rows = subcommands, columns = solo/probe/live, cells =
supported/partial/unsupported), the per-subcommand output contract (stream, encoding, envelope),
and one worked live-mode example covering hub, tab selection, click and capture.

NON-SCOPE: the singleton-identity/comms work on SPRINT #87; solo and probe mode behaviour.
