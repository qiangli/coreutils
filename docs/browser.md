# `browser` — CDP-backed browser automation

`bashy browser` (also `coreutils browser`) drives Chrome/Chromium over the
DevTools protocol. One command, two dials: a **mode** (how it gets a browser)
and a **subcommand** (the action). Add `--json` for structured envelopes — the
agent-facing form, and the default under `$BASHY_AGENTIC`.

Engine: `coreutils/pkg/browser` (`probe` / `solo` / `live` clients + the
`wire` action/result protocol); command: `coreutils/cmds/browser`.

## Modes (`--mode`)

| mode | how it gets a browser | setup | best for |
|---|---|---|---|
| **`solo`** | launches a **private headless Chrome** (`--headed` to show it), runs one action, exits | none | one-shot scrapes / automation — the zero-setup default recommendation for agents |
| **`probe`** *(coded default)* | attaches to a **Chrome you already started** with `--remote-debugging-port=9222` (override with `--probe-url`) | start Chrome yourself | a persistent session you control across many calls |
| **`live`** | drives your **real, logged-in Chrome** via an MV3 extension + a local WebSocket hub | `browser hub` + `browser setup` | pages exactly as *you're* logged in (cookies/SSO intact) |

**Session model:** `probe` and `live` attach to a *persistent* browser, so
multi-step flows (`navigate` → `click` → `extract`) keep state across separate
`bashy browser …` calls. **`solo` is one-shot** — each invocation is a fresh
Chrome, so a successful `navigate <url>` returns the loaded page itself
(`title`/`url`/`content`); a standalone `extract`/`eval` in solo mode would only
see `about:blank`. Scrape in solo mode with a single `navigate` (or an `eval`
whose script is self-contained).

> Note: the **coded default is `probe`**, which errors ("no browser reachable")
> until you start a Chrome with `--remote-debugging-port=9222`. For hands-off
> agent use, pass `--mode solo` (self-contained) or start a probe Chrome first.

## Subcommands

| action | does |
|---|---|
| `status` | is a browser reachable in the current mode (`{reachable, mode, probe_url}`) |
| `navigate <url>` | load a page (solo: this *is* the scrape — returns title/url/content) |
| `extract [scope]` | extract page text/content (optionally scoped to a CSS selector) |
| `eval '<js>'` | run JavaScript in the page, return the result |
| `click <sel>` · `type <sel> <text>` | interact with elements |
| `wait-for-selector <sel>` | block until an element appears |
| `screenshot [path]` | capture the page (PNG) |
| `cookies-get [name [domain]]` | read cookies |
| `scroll [dir amount]` · `keyboard-press <key>` · `back` · `tabs` | page/tab control |
| `fetch <url>` | fetch a URL **through the browser** (JS-executed) — see below |
| `hub` · `setup` | live-mode plumbing (start the hub / connect the extension) |
| `login` | automated login flow (`--success-url`, `--token-selector`, `--cookie`, `--timeout`, `--dry-run`) |

### `browser fetch` ≠ `bashy fetch`

`bashy fetch <url>` is a **plain HTTP/REST client** (no browser) — use it for
APIs and static pages. `browser fetch <url>` renders through Chrome (runs JS) —
use it (or `navigate`+`extract`) for JS-heavy pages.

## Result envelopes (`--json`)

Every action returns a `wire.Result`: `success` (bool), and on success one of
`title`/`url`/`content`/`elements`/`data`/`path`/`image` depending on the
action; on failure, `error`. Pipe to `jq`. Example — a solo scrape:

```sh
$ bashy browser --mode solo --json navigate \
    'data:text/html,<title>Demo</title><h1>Hello from headless Chrome</h1>'
{"success":true,"title":"Demo","url":"data:…","content":"Hello from headless Chrome"}
```

## Examples

**Solo (no setup — one-shot):**
```sh
bashy browser --mode solo --json navigate https://news.ycombinator.com
bashy browser --mode solo --json eval 'document.querySelector("h1").innerText'
bashy browser --mode solo --headed navigate https://example.com   # watch it run
```

**Probe (attach to a Chrome you started — persistent):**
```sh
"$CHROME" --remote-debugging-port=9222 &        # your Chrome/Chromium path
bashy browser status                             # reachable: true
bashy browser navigate https://example.com
bashy browser eval 'document.title'
bashy browser screenshot shot.png
```

**Live (your real logged-in Chrome):**
```sh
bashy browser hub          # start the local WebSocket hub (keep running)
bashy browser setup        # install/connect the MV3 extension (one-time)
bashy browser --mode live navigate https://your-app.example
bashy browser --mode live --json extract
```

**Automated login (capture a token/cookie):**
```sh
bashy browser login --success-url /dashboard --cookie session --timeout 3m
bashy browser login --token-selector '#api-token' --dry-run   # preview what it polls
```

## Notes

- Unknown flags/subcommands fail with a clear exit-2 error, never a silent guess.
- `solo` needs a Chrome/Chromium on the system (`--chrome-path` to point at one;
  `--user-data-dir` for a persistent profile).
- Headless Chrome has its own network stack — if `navigate <live-url>` times out
  with `context deadline exceeded` while `bashy fetch <url>` succeeds, the
  browser's network path is being blocked (e.g. a restricted sandbox), not bashy.
