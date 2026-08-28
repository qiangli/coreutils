// bashy apps — the launcher.
//
// A pill search, then Overview, Favorites, Recent and the app grid, each section
// toggleable, with a settings sheet for background, theme, section visibility
// and recent depth.
//
// Plain JS on purpose: the launcher ships inside a Go binary with no build step,
// and it is bashy's own surface — free to evolve without reference to anything
// else that happens to render tiles.
//
// EVERY url is built from document.baseURI. The server rewrites <base href> per
// request, so the same bytes work at / on loopback and under outpost's
// /matrix/h/<host>/app/<name>/ prefix without knowing which it is.
const url = (p) => new URL(p, document.baseURI);

// A script error used to leave a blank card and no clue — the page simply
// stopped painting. Surface it instead: a launcher that cannot render should say
// so on the launcher.
addEventListener("error", (e) => showFatal(e.message || String(e.error)));
addEventListener("unhandledrejection", (e) => showFatal(String(e.reason)));
function showFatal(msg) {
  const host = document.getElementById("grid-host") || document.body;
  if (document.getElementById("fatal")) return;
  const box = document.createElement("div");
  box.id = "fatal";
  box.className = "fatal";
  const h = document.createElement("strong");
  h.textContent = "The launcher failed to render.";
  const p = document.createElement("p");
  p.textContent = msg;
  box.append(h, p);
  host.prepend(box);
}

const view = document.getElementById("grid-host");
const whoEl = document.getElementById("who");
const bgEl = document.getElementById("bg");
const searchEl = document.getElementById("search");
const footLeft = document.getElementById("foot-left");
const verEl = document.getElementById("ver");

let apps = [];
let search = "";

// ------------------------------------------------------------------ config --
// localStorage only: this is per-viewer chrome, not state anything else reads.
// Every access is guarded — a private window or blocked site data throws on
// access rather than returning null.
const CFG_KEY = "bashy.apps.config";
// Bump when a default changes in a way a previously-saved config would mask.
const CFG_VERSION = 2;
const DEFAULTS = {
  v: CFG_VERSION,
  // A real background by default. The glass card only engages over one (a
  // translucent panel on a flat colour is just a muddier flat colour), so
  // defaulting to "none" meant the launcher shipped looking like an unstyled
  // page unless you went hunting in settings for the thing that styles it.
  background: "sky",
  theme: "system",
  showSummary: true,
  showFavorites: true,
  showRecents: true,
  recentLimit: 8,
  favorites: [],
  recents: [],
};
function loadCfg() {
  let saved = {};
  try {
    saved = JSON.parse(localStorage.getItem(CFG_KEY) || "{}");
  } catch (_) {
    return { ...DEFAULTS };
  }
  // A saved config shadows every default, and the whole config is persisted on
  // any tile click (see recordVisit) — so a single click under an older build
  // pinned that build's defaults forever, and changing one later had no visible
  // effect for anyone who had already used the page. Migrate the fields whose
  // default actually moved rather than silently losing to a stale value.
  if ((saved.v || 1) < 2) {
    delete saved.background; // was "none"; the launcher now ships a real one
    saved.v = CFG_VERSION;
  }
  return { ...DEFAULTS, ...saved };
}
let cfg = loadCfg();
function saveCfg(patch) {
  cfg = { ...cfg, ...patch };
  try { localStorage.setItem(CFG_KEY, JSON.stringify(cfg)); } catch (_) {}
  applyChrome();
}
function applyChrome() {
  const bg = cfg.background || "none";
  bgEl.setAttribute("data-bashy-bg", bg);
  // The glass card and glassy header only make sense over a real background.
  document.body.classList.toggle("has-bg", bg !== "none");
  const root = document.documentElement;
  if (cfg.theme === "system") root.removeAttribute("data-theme");
  else root.setAttribute("data-theme", cfg.theme);
}

function isFav(name) { return cfg.favorites.includes(name); }
function toggleFav(name) {
  const f = isFav(name) ? cfg.favorites.filter((n) => n !== name) : [...cfg.favorites, name];
  saveCfg({ favorites: f });
  render();
}
function recordVisit(name) {
  saveCfg({ recents: [name, ...cfg.recents.filter((n) => n !== name)].slice(0, 24) });
}

// ------------------------------------------------------------------- icons --
// A pinned glyph + colour for the apps bashy knows, and a deterministic hash
// into a fixed palette for everything else, so an app keeps its identity across
// restarts without anyone assigning one.
const ICON_SPEC = {
  ycode: { color: "#1f2328", glyph: "Y" },
  shell: { color: "#0a0e1a", glyph: "$" },
  terminal: { color: "#0a0e1a", glyph: "$" },
  desktop: { color: "#af52de", glyph: "D" },
  ssh: { color: "#0f766e", glyph: "⌥" },
  files: { color: "#3478f6", glyph: "\u{1F5C0}" },
};
const ICON_PALETTE = ["#4f46e5","#3478f6","#ec4899","#f59e0b","#10b981","#8b5cf6",
  "#5ac8fa","#ef4444","#06b6d4","#22c55e","#0ea5e9","#a855f7","#84cc16"];
function appIcon(name) {
  if (ICON_SPEC[name]) return ICON_SPEC[name];
  let h = 0;
  for (let i = 0; i < name.length; i++) h = ((h << 5) - h + name.charCodeAt(i)) | 0;
  return { color: ICON_PALETTE[Math.abs(h) % ICON_PALETTE.length], glyph: (name[0] || "?").toUpperCase() };
}

// ------------------------------------------------------------------- tiles --
function tile(a, opts = {}) {
  const ic = appIcon(a.name);
  const wrap = document.createElement("div");
  wrap.className = "tile-wrap";

  // A real link, opened in its own tab: every app gets the whole browser
  // window, its own history and its own URL — nothing is framed inside the
  // launcher. The launcher is a start page, not a container.
  const open = a.status === "ready";
  const btn = document.createElement(open ? "a" : "button");
  btn.className = "tile";
  if (open) {
    btn.href = url(a.path.replace(/^\//, ""));
    btn.target = "_blank";
    btn.rel = "noopener";
  } else {
    btn.type = "button";
  }
  if (a.status === "unavailable") {
    btn.disabled = true;
    btn.title = a.note || "unavailable on this host";
  } else if (a.status === "stopped") {
    btn.title = a.start_hint ? "Not running. Start it with: " + a.start_hint : "not running";
  }

  const icon = document.createElement("span");
  icon.className = "icon";
  icon.style.background = ic.color;
  icon.append(document.createTextNode(ic.glyph));
  const badge = document.createElement("span");
  badge.className = "badge " + a.status;
  icon.append(badge);
  btn.append(icon);

  const label = document.createElement("span");
  label.className = "label";
  label.textContent = a.label;
  btn.append(label);


  btn.addEventListener("click", () => { if (open) recordVisit(a.name); });
  wrap.append(btn);

  if (!opts.noStar) {
    const star = document.createElement("button");
    star.type = "button";
    star.className = "star" + (isFav(a.name) ? " on" : "");
    star.textContent = isFav(a.name) ? "★" : "☆";
    star.title = isFav(a.name) ? "Remove from favorites" : "Add to favorites";
    star.setAttribute("aria-label", star.title);
    star.addEventListener("click", (e) => { e.stopPropagation(); toggleFav(a.name); });
    wrap.append(star);
  }
  return wrap;
}

function sectionEl(title, nodes) {
  const s = document.createElement("section");
  s.className = "sect";
  const head = document.createElement("div");
  head.className = "sect-head";
  const h = document.createElement("h2");
  h.textContent = title;
  head.append(h);
  s.append(head);
  const grid = document.createElement("div");
  grid.className = "grid";
  grid.append(...nodes);
  s.append(grid);
  return s;
}

// -------------------------------------------------------------------- home --
// render is the single repaint entry point. Everything that changes what the
// page shows — a search keystroke, a settings toggle, a liveness poll — calls
// this rather than reaching into the DOM itself.
function render() { renderHome(); }

function matches(a) {
  const t = search.trim().toLowerCase();
  if (!t) return true;
  return a.name.toLowerCase().includes(t) || (a.label || "").toLowerCase().includes(t);
}

function renderHome() {
  const pad = document.createElement("div");
  pad.className = "pad";
  const shown = apps.filter(matches);
  const byName = new Map(apps.map((a) => [a.name, a]));

  if (cfg.showSummary) {
    const ready = apps.filter((a) => a.status === "ready").length;
    const stopped = apps.filter((a) => a.status === "stopped").length;
    const na = apps.filter((a) => a.status === "unavailable").length;
    const s = document.createElement("section");
    s.className = "sect";
    const head = document.createElement("div");
    head.className = "sect-head";
    const h = document.createElement("h2"); h.textContent = "Overview"; head.append(h);
    s.append(head);
    const card = document.createElement("div");
    card.className = "summary";
    for (const [n, l] of [[apps.length, "Apps"], [ready, "Ready"], [stopped, "Stopped"], [na, "Unavailable"]]) {
      const d = document.createElement("div");
      const b = document.createElement("b"); b.textContent = String(n);
      const sp = document.createElement("span"); sp.textContent = l;
      d.append(b, sp); card.append(d);
    }
    s.append(card);
    pad.append(s);
  }

  if (cfg.showFavorites) {
    const favs = cfg.favorites.map((n) => byName.get(n)).filter(Boolean).filter(matches);
    if (favs.length) pad.append(sectionEl("Favorites", favs.map((a) => tile(a))));
  }

  if (cfg.showRecents) {
    const rec = cfg.recents.map((n) => byName.get(n)).filter(Boolean).filter(matches)
      .slice(0, Math.max(1, cfg.recentLimit | 0));
    if (rec.length) pad.append(sectionEl("Recent", rec.map((a) => tile(a, { noStar: true }))));
  }

  if (shown.length) {
    pad.append(sectionEl("Apps", shown.map((a) => tile(a))));
  } else {
    const s = document.createElement("section");
    s.className = "sect";
    const p = document.createElement("p");
    p.className = "empty";
    p.textContent = apps.length ? "No app matches that search." : "No surfaces declared.";
    s.append(p);
    pad.append(s);
  }

  view.replaceChildren(pad);
}

// ---------------------------------------------------------------- settings --
const BG_PRESETS = [
  ["none", "None"], ["sky", "Sky"], ["ocean", "Ocean"], ["mountains", "Mountains"],
  ["plateau", "Plateau"], ["lakes", "Lakes"], ["bamboo", "Bamboo"],
];
const SECTIONS = [["showSummary", "Overview"], ["showFavorites", "Favorites"], ["showRecents", "Recent"]];
const dlg = document.getElementById("settings");

function buildSettings() {
  const sw = document.getElementById("bg-swatches");
  sw.replaceChildren(...BG_PRESETS.map(([id, label]) => {
    const b = document.createElement("button");
    b.type = "button"; b.className = "swatch-btn";
    b.setAttribute("aria-pressed", String((cfg.background || "none") === id));
    const s = document.createElement("div");
    s.className = "bg-swatch"; s.setAttribute("data-bg", id);
    const t = document.createElement("span"); t.textContent = label;
    b.append(s, t);
    b.addEventListener("click", () => { saveCfg({ background: id }); buildSettings(); });
    return b;
  }));

  for (const b of document.querySelectorAll("#theme-seg button")) {
    b.setAttribute("aria-pressed", String(b.dataset.themeVal === cfg.theme));
    b.onclick = () => { saveCfg({ theme: b.dataset.themeVal }); buildSettings(); };
  }

  const st = document.getElementById("section-toggles");
  st.replaceChildren(...SECTIONS.map(([key, label]) => {
    const row = document.createElement("label");
    row.className = "row";
    const s = document.createElement("span"); s.textContent = label;
    const sw2 = document.createElement("span"); sw2.className = "switch";
    const inp = document.createElement("input"); inp.type = "checkbox"; inp.checked = !!cfg[key];
    const knob = document.createElement("i");
    inp.addEventListener("change", () => { saveCfg({ [key]: inp.checked }); render(); });
    sw2.append(inp, knob);
    row.append(s, sw2);
    return row;
  }));

  const rl = document.getElementById("recent-limit");
  rl.value = cfg.recentLimit;
  rl.onchange = () => { saveCfg({ recentLimit: Math.max(1, Math.min(24, +rl.value || 8)) }); render(); };

  document.getElementById("fav-summary").textContent =
    `${cfg.favorites.length} favorite${cfg.favorites.length === 1 ? "" : "s"}, ${cfg.recents.length} recent.`;
}
document.getElementById("settings-btn").addEventListener("click", () => { buildSettings(); dlg.showModal(); });
document.getElementById("clear-favs").addEventListener("click", () => { saveCfg({ favorites: [] }); buildSettings(); render(); });
document.getElementById("clear-recents").addEventListener("click", () => { saveCfg({ recents: [] }); buildSettings(); render(); });
dlg.addEventListener("close", render);

// ------------------------------------------------------------------- build --
// Which binary am I looking at, and is this page from it or from a cache?
//
// `assets` is the content hash of this very script, so a stale cached copy shows
// a different value from the one the server reports — the question that took a
// whole round trip to answer becomes a glance.
function renderBuild(b) {
  if (!b) return;
  const short = (b.version || "devel") + (b.commit ? " · " + b.commit : "") + (b.dirty ? "-dirty" : "");
  verEl.textContent = b.commit ? b.commit.slice(0, 7) + (b.dirty ? "*" : "") : (b.version || "devel");
  verEl.title = [short, b.time, b.go, "assets " + (b.assets || "?")].filter(Boolean).join("\n");
  const foot = document.getElementById("foot-build");
  if (foot) {
    foot.textContent = short + (b.time ? " · " + b.time.slice(0, 10) : "") + (b.go ? " · " + b.go : "");
  }
}

// ------------------------------------------------------------------ wiring --
searchEl.addEventListener("input", () => { search = searchEl.value; render(); });
document.getElementById("theme-btn").addEventListener("click", () => {
  const order = ["system", "light", "dark"];
  saveCfg({ theme: order[(order.indexOf(cfg.theme) + 1) % order.length] });
});

async function refresh() {
  try {
    const [a, s] = await Promise.all([
      fetch(url("api/apps")).then((r) => r.json()),
      fetch(url("api/session")).then((r) => r.json()).catch(() => null),
    ]);
    apps = a.apps || [];
    if (s) {
      whoEl.textContent = s.user + " · " + s.via;
      renderBuild(s.build);
      const ready = apps.filter((x) => x.status === "ready").length;
      footLeft.textContent = `${ready}/${apps.length} ready · signed in as ${s.user} (${s.via})`;
    }
  } catch (e) {
    apps = [];
    whoEl.textContent = "offline";
  }
  // Only the home grid reflects liveness; repainting under a live terminal
  // would tear down the session every few seconds.
  render();
}

applyChrome();
refresh();
setInterval(refresh, 5000);
