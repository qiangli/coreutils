// The steward board page.
//
// A full browser page like the terminal and the message board, and its
// <base href> is the LAUNCHER's, so "api/board" and "./" resolve correctly at /
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

const state = { all: false, open: {} };

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
  if (["done", "merged", "abandoned", "killed", "no-op"].includes(s)) return "past";
  return "";
}

// ---- rendering ---------------------------------------------------------------

function stat(label, value, cls) {
  const n = el("div", "bd-stat" + (cls ? " " + cls : ""));
  n.append(el("div", "v", String(value)));
  n.append(el("div", "k", label));
  return n;
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
function storiesEl(stories) {
  const wrap = el("div", "refs");
  wrap.append(el("span", "k", "stories"));
  const shown = stories.slice(0, 24);
  for (const t of shown) {
    const chip = el("span", "ref " + stateClass(t.status), "#" + (t.number || t.id));
    // The full title on hover, so a chip line stays scannable without hiding
    // what each chip is.
    chip.title = [t.priority, t.status, t.title].filter(Boolean).join(" · ");
    wrap.append(chip);
  }
  if (stories.length > shown.length) {
    wrap.append(el("span", "more", "+" + (stories.length - shown.length) + " more"));
  }
  return wrap;
}

function storyListEl(stories) {
  const btn = el("button", "more", "stories — " + stories.length);
  btn.type = "button";
  const body = el("div", "continuity");
  body.hidden = true;
  for (const t of stories) {
    const row = el("div", "sec");
    row.append(el("div", "sec-k " + stateClass(t.status),
      "#" + (t.number || t.id) + (t.priority ? " " + t.priority : "")));
    row.append(el("div", "sec-v", t.title || ""));
    body.append(row);
  }
  btn.addEventListener("click", () => {
    body.hidden = !body.hidden;
    btn.classList.toggle("open", !body.hidden);
  });
  return [btn, body];
}

// A SPRINT card. The sprints are the reason this page exists — the lanes below
// are runs, and a run is the execution of one item, not the time-boxed set a
// conductor drives.
function sprintEl(sp, stories) {
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
  n.append(head);
  n.append(el("p", "label", sp.title || ""));

  const holder = sp.conductor || sp.lease_holder;
  if (holder) {
    n.append(el("div", "meta", (sp.lease_stale ? "lease STALE — " : "held by ") + holder));
  }

  const refs = sp.run_refs || [];
  if (refs.length) n.append(runRefsEl(refs));

  stories = stories || [];
  if (stories.length) {
    n.append(storiesEl(stories));
    n.append(...storyListEl(stories));
  }

  const sections = continuitySections(sp.continuity);
  if (sections.length) {
    const labelled = sections.filter((x) => x.label).length;
    const btn = el("button", "more", "continuity — " +
      (labelled ? labelled + " sections" : sections.length + " note" + (sections.length === 1 ? "" : "s")));
    btn.type = "button";
    const body = el("div", "continuity");
    body.hidden = true;
    for (const sec of sections) {
      const row = el("div", "sec");
      if (sec.label) row.append(el("div", "sec-k", sec.label));
      row.append(el("div", "sec-v", sec.body));
      body.append(row);
    }
    btn.addEventListener("click", () => {
      body.hidden = !body.hidden;
      btn.classList.toggle("open", !body.hidden);
    });
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
    const d = await fetch(url("api/board/panel/" + encodeURIComponent(p.id) + "?limit=500")).then((r) => r.json());
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
    "This view never starts, merges, kills, or leases anything — `bashy board` in a browser.";
}

// ---- loading -----------------------------------------------------------------

let inflight = null;

async function load() {
  if (inflight) return;
  const q = state.all ? "?all=1" : "";
  inflight = fetch(url("api/board" + q)).then((r) => r.json());
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
