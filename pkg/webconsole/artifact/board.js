// The steward board page.
//
// A full browser page like the terminal and the message board, and its
// <base href> is the LAUNCHER's, so "api/sprint" and "./" resolve correctly at /
// on loopback and under outpost's /matrix/h/<host>/app/<name>/ prefix alike.
//
// READ-ONLY BY CONSTRUCTION. `board` is the one work verb the atlas marks
// CapReadOnly — it reports across the machine but never starts, merges, or
// kills work — so this page has no action anywhere on it. In particular it must
// never touch a sprint lease: a browser tab left open would otherwise look like
// a working conductor.
const url = (p) => new URL(p, document.baseURI);

try {
  const cfg = JSON.parse(localStorage.getItem("bashy.apps.config") || "{}");
  if (cfg.theme && cfg.theme !== "system") document.documentElement.setAttribute("data-theme", cfg.theme);
} catch (_) {}

const $ = (id) => document.getElementById(id);
const el = (tag, cls, text) => {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text !== undefined) n.textContent = text;
  return n;
};

// state is what a refresh must NOT throw away. The page re-renders every 15s
// with replaceChildren, so anything a reader opened lives here or it collapses
// under them mid-read: `open` is the panels, `stories`/`cont` are a sprint
// card's two disclosures, and `story` is the one story whose body is showing.
const state = { all: false, open: {}, stories: {}, cont: {}, story: {} };

// disclose wires a toggle button to its body and REMEMBERS the answer under
// key, so the next render restores it. Every disclosure on this page goes
// through it — the story list forgot to, which is why an opened story list
// collapsed on the next poll while the story body it was opened for stayed up.
function disclose(btn, body, bag, key) {
  const open = !!bag[key];
  body.hidden = !open;
  btn.classList.toggle("open", open);
  btn.addEventListener("click", () => {
    body.hidden = !body.hidden;
    bag[key] = !body.hidden;
    btn.classList.toggle("open", !body.hidden);
  });
}

// ---- formatting --------------------------------------------------------------

function dur(secs) {
  if (!secs || secs < 0) return "";
  if (secs < 60) return secs + "s";
  if (secs < 3600) return Math.round(secs / 60) + "m";
  if (secs < 86400) return (secs / 3600).toFixed(1) + "h";
  return Math.round(secs / 86400) + "d";
}

// stateClass maps a run/sprint state onto the three things a steward is
// actually sorting for: needs me, running, finished. Anything unrecognised
// stays neutral rather than being guessed into a bucket.
const NEEDS = new Set(["submitted", "review", "failed", "blocked"]);
const LIVE = new Set(["working", "doing", "allocated", "running"]);
function stateClass(s) {
  s = (s || "").toLowerCase();
  if (NEEDS.has(s)) return "needs";
  if (LIVE.has(s)) return "live";
  if (["done", "closed", "cancelled", "canceled", "merged", "abandoned", "killed", "no-op"].includes(s)) return "past";
  return "";
}

// ---- rendering ---------------------------------------------------------------

function stat(label, value, cls) {
  const n = el("div", "bd-stat" + (cls ? " " + cls : ""));
  n.append(el("div", "v", String(value)));
  n.append(el("div", "k", label));
  return n;
}

// The Sprint and Meet apps share the launcher's <base href>, including when
// the launcher itself is reached through an outpost proxy. Build links from
// that base rather than spelling a root-relative /meet/ that would jump out of
// the mounted app on a remote host.
function meetHref(kind, ref) {
  const target = url("meet/");
  target.searchParams.set(kind, ref);
  return target.href;
}

const NEW_SPRINT_DRAFT = "Create a new sprint. Help me define its title, project manager, scope, stories, acceptance criteria, and gate, then use bashy sprint to create and start it.";

function newSprintLink() {
  const a = conversationLink("chat", "1", "Create a new sprint in Chat");
  const target = new URL(a.href);
  target.searchParams.set("draft", NEW_SPRINT_DRAFT);
  a.href = target.href;
  a.classList.add("new-sprint-link");
  a.append(el("span", null, "New sprint"));
  return a;
}

function conversationLink(kind, ref, label) {
  const a = el("a", "conversation-link");
  a.href = meetHref(kind, ref);
  a.title = label;
  a.setAttribute("aria-label", label);
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  svg.setAttribute("aria-hidden", "true");
  svg.setAttribute("fill", "none");
  svg.setAttribute("stroke", "currentColor");
  svg.setAttribute("stroke-width", "1.9");
  svg.setAttribute("stroke-linecap", "round");
  svg.setAttribute("stroke-linejoin", "round");
  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", kind === "dm"
    ? "M21 15a4 4 0 0 1-4 4H8l-5 3V7a4 4 0 0 1 4-4h10a4 4 0 0 1 4 4z"
    : "M4 9h16M4 15h16M10 3 8 21M16 3l-2 18");
  svg.append(path);
  a.append(svg);
  return a;
}

function renderSummary(d) {
  const s = d.summary || {};
  const host = d.resources || {};
  const cells = [
    stat("sprints", d.sprint_total, ""),
    stat("runs", d.run_total, ""),
    stat("needs steward", s.needs_steward, s.needs_steward ? "needs" : ""),
    stat("in flight", s.in_flight, s.in_flight ? "live" : ""),
    stat("todos", s.todos, ""),
    stat("median eta", dur(s.eta_median_seconds) || "—", ""),
  ];
  if (host.cpu) cells.push(stat("cpu", Math.round(host.cpu.usage_percent) + "%", ""));
  if (host.memory) cells.push(stat("memory", host.memory.used_percent + "%", ""));
  if (host.disks && host.disks.length) {
    const worst = host.disks.reduce((n, disk) => Math.max(n, Number(disk.used_percent) || 0), 0);
    cells.push(stat("disk", Math.round(worst) + "%", ""));
  }
  $("bd-summary").replaceChildren(...cells);

  const u = d.utilization;
  const v = $("bd-verdict");
  if (u && u.verdict) {
    v.textContent = u.verdict;
    v.className = u.verdict === "SATURATED" ? "needs" : "";
    v.title = u.reason || "";
  } else {
    v.textContent = "";
    v.title = "";
  }
}

function renderWarnings(d) {
  const host = $("bd-warnings");
  const ws = d.warnings || [];
  host.hidden = ws.length === 0;
  if (!ws.length) return;
  // A source that failed is reported, never swallowed: a board missing a whole
  // source silently looks exactly like a board with nothing in it.
  host.replaceChildren(
    el("strong", null, ws.length + " source warning" + (ws.length === 1 ? "" : "s")),
    ...ws.map((w) => el("div", "w", w)),
  );
}

function card(c) {
  const n = el("article", "bd-card " + stateClass(c.state));
  const head = el("header");
  head.append(el("span", "id", "#" + c.id));
  head.append(el("span", "st", c.state || ""));
  if (c.tool) head.append(el("span", "tool", c.tool + (c.band ? " L" + c.band : "")));
  head.append(el("span", "spacer"));
  if (c.elapsed_seconds) head.append(el("span", "t", dur(c.elapsed_seconds)));
  n.append(head);
  n.append(el("p", "label", c.label || ""));
  const meta = [];
  if (c.scope) meta.push(c.scope);
  if (c.unmerged_commits) meta.push(c.unmerged_commits + " unmerged");
  if (c.salvageable) meta.push("salvageable");
  if (c.stale) meta.push("stale");
  if (meta.length) n.append(el("div", "meta", meta.join(" · ")));
  return n;
}

// Conductors write the continuity brief as blank-line-separated paragraphs led
// by an ALL-CAPS label — STATE:, BLOCKED, NEXT ACTION:, GATES GREEN:. Sprint 84
// is 17.9 kB and nine such sections. As one <pre> that is a wall nobody reads;
// as its sections it is the status report it was written as. A short
// single-paragraph brief (most sprints) has no labels and falls through as one
// plain block, which is right for it.
const SECTION_LABEL = /^([A-Z][A-Z0-9 ,/+&()'-]{2,40}):\s*/;

function continuitySections(text) {
  return String(text || "")
    .split(/\n\s*\n/)
    .map((p) => p.trim())
    .filter(Boolean)
    .map((p) => {
      const m = SECTION_LABEL.exec(p);
      return m ? { label: m[1], body: p.slice(m[0].length) } : { label: "", body: p };
    });
}

// The linked runs are the sprint's ITEMS. They were rendered as a bare count
// ("21 runs"), which says a sprint is busy without saying what it spans — and
// sprint 82's 21 refs cross several repos, which is the useful part.
function runRefsEl(refs) {
  const wrap = el("div", "refs");
  wrap.append(el("span", "k", "runs"));
  const shown = refs.slice(0, 24);
  for (const r of shown) wrap.append(el("span", "ref", (r.repo || "?") + "#" + r.id));
  if (refs.length > shown.length) {
    wrap.append(el("span", "more", "+" + (refs.length - shown.length) + " more"));
  }
  return wrap;
}

// The linked STORIES are what a sprint is FOR, and this page showed none of
// them: the overview payload carried sprints and runs and dropped todos
// entirely, so every card rendered as a title with no work under it. A sprint
// whose stories are invisible reads as an empty sprint.
//
// Rendered in two registers, both reusing the card's existing classes so this
// costs no CSS: an always-visible chip line (what runRefsEl does for runs, so a
// scan sees at once that a card has seven stories, not zero) and a collapsible
// list carrying each story's status, priority and title.
// storyDetail fetches ONE story's full record. Cached per id for the life of
// the page: a body does not change under a reader, and a second click on the
// same story should not re-hit the host.
const storyCache = new Map();
async function storyDetail(id) {
  if (storyCache.has(id)) return storyCache.get(id);
  const p = fetch(url("api/sprint/story/" + encodeURIComponent(id)))
    .then(async (r) => {
      const d = await r.json().catch(() => ({}));
      if (!r.ok) throw new Error(d.error || "HTTP " + r.status);
      return d;
    });
  storyCache.set(id, p);
  // A failed fetch must not poison the cache — the next click should retry
  // rather than replay the error forever.
  p.catch(() => storyCache.delete(id));
  return p;
}

// openStory renders a story's detail INTO an already-visible container. It is
// the one place the page shows a body, so both entry points (a chip and a
// list row) land here and agree.
async function openStory(id, host, sprintID) {
  // Remember it. The page reloads every 15s and re-renders with
  // replaceChildren, so without this a reader's open story vanishes
  // mid-sentence — the panels already solve the same problem with state.open.
  if (sprintID) state.story[sprintID] = id;
  host.hidden = false;
  host.replaceChildren(el("div", "sec-v", "loading #" + id + " …"));
  try {
    const d = await storyDetail(id);
    const rows = [];
    // "unassigned" is todo's placeholder for "nobody", so rendering it as
    // "@unassigned" invents a person. An absent owner shows as nothing.
    const owner = d.assignee && !/^(unassigned|-|none)$/i.test(d.assignee) ? "@" + d.assignee : "";
    const meta = [d.status, d.priority, owner,
      d.sprint ? "sprint #" + d.sprint : "", d.scope]
      .filter(Boolean).join(" · ");
    const head = el("div", "sec");
    head.append(el("div", "sec-k " + stateClass(d.status), "#" + (d.seq || d.id)));
    head.append(el("div", "sec-v", d.title || ""));
    rows.push(head);
    if (meta) {
      const m = el("div", "sec");
      m.append(el("div", "sec-k", "meta"));
      m.append(el("div", "sec-v", meta));
      rows.push(m);
    }
    // The body is written as labelled paragraphs, exactly like a continuity
    // brief, so it is rendered with the same splitter rather than as one wall.
    const secs = continuitySections(d.body);
    if (secs.length) {
      for (const sec of secs) {
        const row = el("div", "sec");
        if (sec.label) row.append(el("div", "sec-k", sec.label));
        row.append(el("div", "sec-v", sec.body));
        rows.push(row);
      }
    } else {
      const row = el("div", "sec");
      row.append(el("div", "sec-v", "(no body)"));
      rows.push(row);
    }
    host.replaceChildren(...rows);
  } catch (e) {
    // Name the failure. A detail pane that silently shows nothing is the same
    // defect class this board is meant to report on.
    const row = el("div", "sec");
    row.append(el("div", "sec-k needs", "error"));
    row.append(el("div", "sec-v", String(e.message || e)));
    host.replaceChildren(row);
  }
}

function storiesEl(stories, detailHost, sprintID) {
  const wrap = el("div", "refs");
  wrap.append(el("span", "k", "stories"));
  const shown = stories.slice(0, 24);
  for (const t of shown) {
    // A button, not a span: a thing that responds to a click has to be
    // reachable by keyboard and announce itself as activatable.
    const chip = el("button", "ref link " + stateClass(t.status), "#" + (t.number || t.id));
    chip.type = "button";
    chip.title = [t.priority, t.status, t.title].filter(Boolean).join(" · ");
    chip.addEventListener("click", () => openStory(t.id, detailHost, sprintID));
    wrap.append(chip);
  }
  if (stories.length > shown.length) {
    wrap.append(el("span", "more", "+" + (stories.length - shown.length) + " more"));
  }
  return wrap;
}

function storyIsClosed(story) {
  return ["done", "closed", "cancelled", "canceled"].includes(String(story.status || "").toLowerCase());
}

function storyStats(sp, stories) {
  const derivedClosed = stories.filter(storyIsClosed).length;
  const total = Number.isInteger(sp.story_total) ? sp.story_total : stories.length;
  const closed = Number.isInteger(sp.story_closed) ? sp.story_closed : derivedClosed;
  const open = Number.isInteger(sp.story_open) ? sp.story_open : Math.max(0, total - closed);
  return { total, closed, open };
}

function storyListEl(sp, stories, detailHost, sprintID) {
  const stats = storyStats(sp, stories);
  const btn = el("button", "more", "stories — " + stats.open + " open · " +
    stats.closed + " closed of " + stats.total);
  btn.type = "button";
  const body = el("div", "continuity");
  const groups = [
    ["open", stories.filter((t) => !storyIsClosed(t))],
    ["closed", stories.filter(storyIsClosed)],
  ];
  for (const [label, items] of groups) {
    if (!items.length) continue;
    body.append(el("div", "story-group", label + " — " + items.length));
    for (const t of items) {
      const row = el("button", "sec link " + (storyIsClosed(t) ? "past" : ""));
      row.type = "button";
      row.append(el("div", "sec-k " + stateClass(t.status),
        "#" + (t.number || t.id) + (t.priority ? " " + t.priority : "")));
      row.append(el("div", "sec-v", t.title || ""));
      row.addEventListener("click", () => openStory(t.id, detailHost, sprintID));
      body.append(row);
    }
  }
  disclose(btn, body, state.stories, sprintID);
  return [btn, body];
}

// A SPRINT card. The sprints are the reason this page exists — the lanes below
// are runs, and a run is the execution of one item, not the time-boxed set a
// conductor drives.
function sprintEl(sp, stories) {
  stories = stories || [];
  const n = el("article", "bd-sprint " + stateClass(sp.column));
  const head = el("header");
  head.append(el("span", "id", "#" + sp.id));
  head.append(el("span", "st", sp.column || ""));
  if (sp.epic) head.append(el("span", "epic", sp.epic));
  head.append(el("span", "spacer"));
  if (sp.gate_state) {
    // The gate is the whole point of a sprint: entity.Sprint.CanConverge is
    // false without one, so its state is never decoration.
    head.append(el("span", "gate " + (sp.gate_state === "complete" ? "past" : "needs"), sp.gate_state));
  }
  // Progress belongs in the HEAD, not only on the disclosure that reveals the
  // list. Whether a sprint has three stories left or thirty is the question a
  // scan of the column is asking, and it should not cost a click per card.
  const headStats = storyStats(sp, stories || []);
  if (headStats.total) {
    const chip = el("span", "stories" + (headStats.open ? "" : " past"),
      headStats.open + " open / " + headStats.closed + " closed");
    chip.title = headStats.closed + " of " + headStats.total + " stories complete";
    head.append(chip);
  }
  n.append(head);

  const title = el("div", "sprint-title");
  title.append(el("p", "label", sp.title || ""));
  if (sp.meet_room_ref) {
    title.append(conversationLink(
      "room",
      sp.meet_room_ref,
      "Open sprint " + sp.id + " Meet room",
    ));
  }
  n.append(title);

  const manager = sp.manager || sp.conductor || sp.lease_holder;
  if (manager) {
    const meta = el("div", "meta manager");
    meta.append(document.createTextNode(
      (sp.lease_stale ? "lease STALE — " : "project manager ") + manager,
    ));
    meta.append(conversationLink("dm", manager, "Chat 1:1 with " + manager));
    n.append(meta);
  }

  const refs = sp.run_refs || [];
  if (refs.length) n.append(runRefsEl(refs));

  if (stories.length) {
    // One detail pane per card, shared by both entry points, so clicking a
    // second story replaces the first rather than stacking panes nobody closes.
    const detail = el("div", "continuity story-detail");
    detail.hidden = true;
    n.append(storiesEl(stories, detail, sp.id));
    n.append(...storyListEl(sp, stories, detail, sp.id));
    n.append(detail);
    // Re-open whatever the reader had open before the last refresh. The body
    // is cached per id, so this costs no request.
    const wasOpen = state.story[sp.id];
    if (wasOpen && stories.some((t) => t.id === wasOpen)) {
      openStory(wasOpen, detail, sp.id);
    }
  }

  const sections = continuitySections(sp.continuity);
  if (sections.length) {
    const labelled = sections.filter((x) => x.label).length;
    const btn = el("button", "more", "continuity — " +
      (labelled ? labelled + " sections" : sections.length + " note" + (sections.length === 1 ? "" : "s")));
    btn.type = "button";
    const body = el("div", "continuity");
    for (const sec of sections) {
      const row = el("div", "sec");
      if (sec.label) row.append(el("div", "sec-k", sec.label));
      row.append(el("div", "sec-v", sec.body));
      body.append(row);
    }
    disclose(btn, body, state.cont, sp.id);
    n.append(btn, body);
  }
  return n;
}

// storiesBySprint indexes the todos onto their card. A story whose sprint is 0
// is UNLINKED, which is a perfectly ordinary item — todo does not require a
// sprint — so it simply does not appear under any card.
function storiesBySprint(todos) {
  const by = new Map();
  for (const t of todos || []) {
    const id = t.sprint_id;
    if (!id) continue;
    if (!by.has(id)) by.set(id, []);
    by.get(id).push(t);
  }
  return by;
}

function renderSprints(d) {
  const host = $("bd-sprints");
  const sps = d.sprints || [];
  const by = storiesBySprint(d.todos);
  const h = el("header");
  h.append(el("span", "t", "Sprints"));
  h.append(el("span", "n", sps.length + (d.all ? "" : " of " + d.sprint_total)));
  h.append(newSprintLink());
  const body = el("div", "cards");
  if (!sps.length) {
    body.append(el("p", "empty", d.all ? "No sprints." : "No sprint is open. Tick “include history” for the finished ones."));
  } else {
    for (const sp of sps) body.append(sprintEl(sp, by.get(sp.id)));
  }
  host.replaceChildren(h, body);
}

function renderLanes(d) {
  const host = $("bd-lanes");
  const lanes = d.lanes || [];
  if (!lanes.length) {
    host.replaceChildren(el("p", "empty", "No runs."));
    return;
  }
  host.replaceChildren(...lanes.map((lane) => {
    const col = el("section", "bd-lane");
    const h = el("header");
    h.append(el("span", "t", lane.title));
    h.append(el("span", "n", String((lane.cards || []).length)));
    col.append(h);
    const body = el("div", "cards");
    for (const c of lane.cards || []) body.append(card(c));
    if (lane.dropped) body.append(el("p", "dropped", "+" + lane.dropped + " more"));
    if (!(lane.cards || []).length) body.append(el("p", "empty", "empty"));
    col.append(body);
    return col;
  }));
}

function table(columns, rows) {
  const t = el("table");
  const thead = el("thead");
  const hr = el("tr");
  for (const c of columns || []) hr.append(el("th", null, c));
  thead.append(hr);
  t.append(thead);
  const tb = el("tbody");
  for (const r of rows || []) {
    const tr = el("tr");
    for (const cell of r) tr.append(el("td", null, cell));
    tb.append(tr);
  }
  t.append(tb);
  const wrap = el("div", "tw");
  wrap.append(t);
  return wrap;
}

async function loadPanelRows(p, body, more) {
  // The full row set is fetched only when a reader opens the panel. The dag
  // panel alone is 8,779 rows / 1.5 MB here; shipping that on every poll is how
  // a read panel turns into a data plane.
  more.disabled = true;
  more.textContent = "loading…";
  try {
    const d = await fetch(url("api/sprint/panel/" + encodeURIComponent(p.id) + "?limit=500")).then((r) => r.json());
    body.replaceChildren(table(d.columns, d.rows));
    if (d.row_total > (d.rows || []).length) {
      body.append(el("p", "dropped", "showing " + d.rows.length + " of " + d.row_total));
    }
    more.remove();
  } catch (_) {
    more.disabled = false;
    more.textContent = "load all — retry";
  }
}

function renderPanels(d) {
  const host = $("bd-panels");
  host.replaceChildren(...(d.panels || []).map((p) => {
    const sec = el("section", "bd-panel");
    const h = el("button", "bd-panel-head");
    h.type = "button";
    h.append(el("span", "t", p.title));
    h.append(el("span", "c", p.collapsed || ""));
    h.append(el("span", "spacer"));
    h.append(el("span", "n", p.row_total ? String(p.row_total) : ""));
    sec.append(h);

    const body = el("div", "bd-panel-body");
    body.hidden = !state.open[p.id];
    if (!p.row_total) {
      body.append(el("p", "empty", "nothing to show"));
    } else {
      body.append(table(p.columns, p.rows));
      if (p.row_total > (p.rows || []).length) {
        const more = el("button", "btn", "load all " + p.row_total);
        more.type = "button";
        more.addEventListener("click", () => loadPanelRows(p, body, more));
        body.append(more);
      }
    }
    h.addEventListener("click", () => {
      body.hidden = !body.hidden;
      state.open[p.id] = !body.hidden;
    });
    sec.append(body);
    return sec;
  }));
}

function renderMeta(d) {
  $("bd-age").textContent = d.age_seconds > 0 ? "collected " + dur(d.age_seconds) + " ago" : "just collected";
  $("bd-scope").textContent = d.all
    ? "everything, history included — " + d.sprint_total + " sprints, " + d.run_total + " runs"
    : d.sprints.length + " of " + d.sprint_total + " sprints and " +
      d.runs.length + " of " + d.run_total + " runs are live";
  $("bd-foot").textContent =
    d.title + " · " + d.scope + " · re-collected at most every " + dur(d.ttl_seconds) +
    " (it forks a subprocess per weave queue root, so it is not free). " +
    "This view never starts, merges, kills, or leases anything — `bashy sprint board` in a browser.";
}

// ---- loading -----------------------------------------------------------------

let inflight = null;

async function load() {
  if (inflight) return;
  const q = state.all ? "?all=1" : "";
  inflight = fetch(url("api/sprint" + q)).then((r) => r.json());
  let d;
  try {
    d = await inflight;
  } catch (_) {
    return;
  } finally {
    inflight = null;
  }
  if (d.error) {
    $("bd-summary").replaceChildren(el("p", "empty", d.error));
    return;
  }
  renderWarnings(d);
  renderSummary(d);
  renderSprints(d);
  renderLanes(d);
  renderPanels(d);
  renderMeta(d);
}

function init() {
  state.all = new URLSearchParams(location.hash.replace(/^#/, "")).get("all") === "1";
  $("f-all").checked = state.all;
  $("f-all").addEventListener("change", () => {
    state.all = $("f-all").checked;
    history.replaceState(null, "", state.all ? "#all=1" : "#");
    load();
  });
  $("f-refresh").addEventListener("click", load);

  load();
  // Half the server's TTL: often enough that the age line stays small, rarely
  // enough that the poll itself is never what triggers a collect.
  setInterval(load, 15000);
}

init();
