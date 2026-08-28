// bashy apps — the launcher shell.
//
// No build step and no framework: the page loads two vendored UMD files
// (xterm.js, addon-fit) and this script. That is deliberate — `go build` alone
// has to produce a working launcher, so there must be nothing between a
// checkout and a running binary.
//
// EVERY url is built from document.baseURI. The server rewrites <base href> per
// request, so the same bytes work at / on loopback and under outpost's
// /matrix/h/<host>/app/<name>/ prefix without knowing which it is.
const url = (p) => new URL(p, document.baseURI);

const view = document.getElementById("view");
const titleEl = document.getElementById("title");
const backEl = document.getElementById("back");
const whoEl = document.getElementById("who");

let apps = [];

// ---------------------------------------------------------------- icons ----
// Drawn as inline SVG paths rather than shipped as PNGs: they inherit the theme
// colour, stay sharp at any density, and cost bytes in the hundreds.
const ICONS = {
  terminal: "M5 7l5 5-5 5M13 17h6",
  files: "M4 8a2 2 0 0 1 2-2h3l2 2h7a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2z",
  relay: "M4 6a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v5a2 2 0 0 1-2 2H8l-4 3zM18 9h2a2 2 0 0 1 2 2v5l-3-2",
  dag: "M12 4v4M6 20v-3a2 2 0 0 1 2-2h8a2 2 0 0 1 2 2v3M12 8v7",
  loom: "M6 4v16M18 4v16M6 9h12M6 15h12",
  _: "M4 4h7v7H4zM13 4h7v7h-7zM4 13h7v7H4zM13 13h7v7h-7z",
};
const dots = { terminal: 1, dag: 1, loom: 1 };

function icon(name, status) {
  const wrap = document.createElement("span");
  wrap.className = "icon";
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 24 24");
  const path = document.createElementNS("http://www.w3.org/2000/svg", "path");
  path.setAttribute("d", ICONS[name] || ICONS._);
  svg.append(path);
  if (dots[name]) {
    const c = document.createElementNS("http://www.w3.org/2000/svg", "circle");
    c.setAttribute("cx", "12"); c.setAttribute("cy", "12"); c.setAttribute("r", "1.4");
    c.setAttribute("fill", "currentColor"); c.setAttribute("stroke", "none");
    if (name === "terminal") { c.setAttribute("cx", "12"); c.setAttribute("cy", "17"); }
    if (name === "loom") { c.setAttribute("cx", "12"); c.setAttribute("cy", "12"); }
    svg.append(c);
  }
  wrap.append(svg);
  const b = document.createElement("span");
  b.className = "badge " + status;
  wrap.append(b);
  return wrap;
}

// ------------------------------------------------------------------ home ----
function renderHome() {
  titleEl.textContent = "Apps";
  backEl.hidden = true;

  const pad = document.createElement("div");
  pad.className = "pad";
  const grid = document.createElement("div");
  grid.className = "grid";

  for (const a of apps) {
    const btn = document.createElement("button");
    btn.className = "app";
    btn.type = "button";
    btn.append(icon(a.name, a.status));

    const label = document.createElement("span");
    label.className = "label";
    label.textContent = a.label;
    btn.append(label);

    if (a.status === "unavailable") {
      btn.disabled = true;
      btn.title = a.note || "unavailable on this host";
      const h = document.createElement("span");
      h.className = "hint";
      h.textContent = a.note || "unavailable";
      btn.append(h);
    } else if (a.status === "stopped") {
      const h = document.createElement("span");
      h.className = "hint";
      h.textContent = "not running";
      btn.append(h);
      btn.title = a.start_hint ? "Start it with: " + a.start_hint : "not running";
    }

    btn.addEventListener("click", () => {
      location.hash = a.name === "terminal" ? "#/terminal" : "#/app/" + a.name;
    });
    grid.append(btn);
  }

  pad.append(grid);
  if (!apps.length) {
    const p = document.createElement("p");
    p.className = "note";
    p.textContent = "No surfaces declared.";
    pad.append(p);
  }
  view.replaceChildren(pad);
}

// ----------------------------------------------------------------- panel ----
function renderPanel(name) {
  const app = apps.find((a) => a.name === name);
  titleEl.textContent = app ? app.label : name;
  backEl.hidden = false;

  if (app && app.status === "stopped") {
    const pad = document.createElement("div");
    pad.className = "pad";
    const p = document.createElement("p");
    p.className = "note";
    p.append(document.createTextNode(app.label + " is not running. Start it with "));
    const c = document.createElement("code");
    c.textContent = app.start_hint || "";
    p.append(c, document.createTextNode(" — this page refreshes on its own."));
    pad.append(p);
    view.replaceChildren(pad);
    return;
  }

  const f = document.createElement("iframe");
  // The panel keeps its own <base href>; the console only frames it, so one nav
  // and one auth cover every app without each one re-implementing chrome.
  f.src = url((app ? app.path : "/" + name + "/").replace(/^\//, ""));
  f.title = app ? app.label : name;
  view.replaceChildren(f);
}

// -------------------------------------------------------------- terminal ----
function renderTerminal() {
  titleEl.textContent = "Terminal";
  backEl.hidden = false;

  const host = document.createElement("div");
  host.id = "term";
  view.replaceChildren(host);

  const term = new Terminal({
    cursorBlink: true,
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, "Cascadia Mono", monospace',
    fontSize: 13,
    theme: matchMedia("(prefers-color-scheme: dark)").matches
      ? { background: "#0b0b0d", foreground: "#fafafa", cursor: "#fafafa" }
      : { background: "#1b1b1f", foreground: "#f4f4f5", cursor: "#f4f4f5" },
  });
  const fit = new FitAddon.FitAddon();
  term.loadAddon(fit);
  term.open(host);
  fit.fit();

  const ws = url("term/ws");
  ws.protocol = ws.protocol === "https:" ? "wss:" : "ws:";
  ws.searchParams.set("cols", term.cols);
  ws.searchParams.set("rows", term.rows);

  const sock = new WebSocket(ws);
  sock.binaryType = "arraybuffer";
  const enc = new TextEncoder();

  sock.onopen = () => term.focus();
  // Binary frames both ways: raw PTY bytes, never re-encoded. A text frame is
  // reserved for control messages, which is why resize is JSON and input is not.
  sock.onmessage = (e) => {
    if (e.data instanceof ArrayBuffer) term.write(new Uint8Array(e.data));
    else term.write(e.data);
  };
  term.onData((d) => {
    if (sock.readyState === WebSocket.OPEN) sock.send(enc.encode(d));
  });

  const sendSize = () => {
    if (sock.readyState !== WebSocket.OPEN) return;
    sock.send(JSON.stringify({ type: "size", cols: term.cols, rows: term.rows }));
  };
  const ro = new ResizeObserver(() => { try { fit.fit(); sendSize(); } catch (_) {} });
  ro.observe(host);

  const ended = (why) => {
    ro.disconnect();
    if (document.getElementById("term-msg")) return;
    const bar = document.createElement("div");
    bar.className = "term-msg";
    bar.id = "term-msg";
    bar.append(document.createTextNode(why));
    const again = document.createElement("button");
    again.textContent = "New session";
    again.addEventListener("click", () => renderTerminal());
    bar.append(again);
    view.append(bar);
  };
  // The server closes with a reason when it can (a shell that quit during its
  // own startup is the common one); 1006 means the connection simply dropped.
  sock.onclose = (e) => ended(e.reason || (e.code === 1006 ? "Connection lost." : "Session ended."));
  sock.onerror = () => ended("Connection failed.");

  // Leaving the view must not leave a shell running with nobody attached.
  routeCleanup = () => { ro.disconnect(); sock.close(); term.dispose(); };
}

// ----------------------------------------------------------------- route ----
let routeCleanup = null;

function route() {
  if (routeCleanup) { try { routeCleanup(); } catch (_) {} routeCleanup = null; }
  const h = location.hash || "#/";
  if (h === "#/terminal") return renderTerminal();
  const m = h.match(/^#\/app\/([\w.-]+)$/);
  if (m) return renderPanel(m[1]);
  renderHome();
}

backEl.addEventListener("click", () => { location.hash = "#/"; });
addEventListener("hashchange", route);

async function refresh() {
  try {
    const [a, s] = await Promise.all([
      fetch(url("api/apps")).then((r) => r.json()),
      fetch(url("api/session")).then((r) => r.json()).catch(() => null),
    ]);
    apps = a.apps || [];
    if (s) whoEl.textContent = s.user + " · " + s.via;
  } catch (e) {
    apps = [];
    whoEl.textContent = "offline";
  }
  // Only the home grid and a stopped-panel notice reflect liveness; repainting
  // under a live terminal would tear down the session every few seconds.
  if (!location.hash || location.hash === "#/") renderHome();
}

refresh().then(route);
setInterval(refresh, 5000);
