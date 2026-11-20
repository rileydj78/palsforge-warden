"use strict";
// Warden panel. No framework and no third-party code: plain DOM, text only (never innerHTML
// with data), so nothing a player names themselves can run in the panel.
const $ = (id) => document.getElementById(id);
let me = null, csrf = "", range = "day";

function el(tag, text, cls) {
  const e = document.createElement(tag);
  if (text != null) e.textContent = text;
  if (cls) e.className = cls;
  return e;
}
function toast(msg) {
  const t = $("toast");
  t.textContent = msg; t.hidden = false;
  clearTimeout(toast.h); toast.h = setTimeout(() => (t.hidden = true), 5000);
}
function duration(s) {
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}
const when = (ts) => ts ? new Date(ts * 1000).toLocaleString([], { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }) : "–";

async function api(path, body) {
  const opt = body ? { method: "POST", headers: { "Content-Type": "application/json", "X-CSRF": csrf }, body: JSON.stringify(body) } : {};
  const r = await fetch(path, { cache: "no-store", ...opt });
  let data = null;
  try { data = await r.json(); } catch (e) {}
  if (r.status === 401 && path !== "/api/login" && path !== "/api/setup") { showGate(false); throw new Error("signed out"); }
  if (!r.ok) throw new Error((data && data.error) || `Warden answered ${r.status}`);
  return data;
}

// ---- sign-in and first-run setup
function showGate(setup) {
  $("layout").hidden = true; $("signout").hidden = true; $("state").hidden = true; $("gate").hidden = false;
  $("codewrap").hidden = !setup;
  $("gatetitle").textContent = setup ? "Set up Warden" : "Sign in";
  $("gatesub").textContent = setup ? "Create the owner account. You'll need the setup code Warden printed when it started." : "";
  $("gatebtn").textContent = setup ? "Create the owner account" : "Sign in";
  $("g-pw").autocomplete = setup ? "new-password" : "current-password";
  $("gate").dataset.setup = setup ? "1" : "";
  $("g-name").focus();
}
$("gateform").addEventListener("submit", async (e) => {
  e.preventDefault();
  const setup = $("gate").dataset.setup === "1";
  try {
    const out = await api(setup ? "/api/setup" : "/api/login", { code: $("g-code").value, name: $("g-name").value, password: $("g-pw").value });
    $("g-pw").value = ""; $("gatemsg").textContent = "";
    signedIn(out);
  } catch (err) { $("gatemsg").textContent = err.message; }
});
$("signout").addEventListener("click", async () => { try { await api("/api/logout", {}); } catch (e) {} showGate(false); });

function signedIn(out) {
  me = out.user; csrf = out.csrf;
  $("gate").hidden = true; $("layout").hidden = false; $("signout").hidden = false; $("state").hidden = false;
  $("signout").textContent = `Sign out (${me.name})`;
  $("adminsnav").hidden = me.role !== "owner";
  for (const id of ["b-save", "b-shutdown"]) $(id).hidden = me.role === "moderator";
  route(); refresh(); history();
}

// ---- dialogs (one at a time, typed confirmation for anything that can't be undone)
function ask({ title, text, label, value = "", ok = "OK", required = false, confirmWord = "" }) {
  return new Promise((resolve) => {
    $("dlgtitle").textContent = title; $("dlgtext").textContent = text || "";
    $("dlglabel").textContent = label || ""; const inp = $("dlginput");
    inp.value = value; inp.hidden = !label; $("dlglabel").hidden = !label; inp.required = required;
    inp.placeholder = confirmWord ? `Type ${confirmWord} to confirm` : "";
    $("dlgok").textContent = ok;
    const d = $("dlg");
    d.onclose = () => {
      if (d.returnValue !== "ok") return resolve(null);
      if (confirmWord && inp.value.trim().toUpperCase() !== confirmWord) { toast("Not confirmed; nothing was done."); return resolve(null); }
      resolve(inp.value);
    };
    d.showModal(); if (label) inp.focus();
  });
}

async function doAction(name, body, done) {
  try { await api(`/api/action/${name}`, body); toast(done); refresh(); }
  catch (err) { toast(err.message); }
}
$("b-announce").addEventListener("click", async () => {
  const m = await ask({ title: "Announce to everyone in the game", label: "Message", ok: "Send", required: true });
  if (m) doAction("announce", { message: m }, "Announced.");
});
$("b-save").addEventListener("click", () => doAction("save", {}, "World saved."));
$("b-shutdown").addEventListener("click", async () => {
  const m = await ask({ title: "Shut the server down", text: "Players get a countdown in the game. If nothing restarts the server automatically, it stays off until someone starts it.", label: "Message to players", value: "The server is restarting in 60 seconds.", ok: "Shut down in 60 s" });
  if (m != null) doAction("shutdown", { seconds: 60, message: m }, "Shutting down in 60 seconds.");
});
async function kick(p) {
  const m = await ask({ title: `Kick ${p.name}?`, text: "They can join again straight away.", label: "Reason (they see it)", ok: "Kick" });
  if (m != null) doAction("kick", { userId: p.userId, name: p.name, message: m }, `${p.name} was kicked.`);
}
async function ban(p) {
  const m = await ask({ title: `Ban ${p.name}?`, text: "They can't join until someone unbans them.", label: "Reason (they see it)", ok: "Ban" });
  if (m != null) doAction("ban", { userId: p.userId, name: p.name, message: m }, `${p.name} was banned.`);
}
async function unban(p) { doAction("unban", { userId: p.userId, name: p.name }, `${p.name} was unbanned.`); }

// ---- dashboard
function render(st) {
  const state = $("state");
  if (!st.online) {
    state.classList.add("down"); $("statetext").textContent = "Not answering";
    $("headline").textContent = "The server isn't answering"; $("subline").textContent = st.error || "";
    return;
  }
  state.classList.remove("down");
  const m = st.metrics || {}, n = st.players.length;
  $("servername").textContent = st.server.servername || "";
  document.title = `Warden · ${st.server.servername || "Palworld"}`;
  $("statetext").textContent = `Online · ${st.server.version}`;
  $("headline").textContent = n === 0 ? "All quiet. Nobody is on right now." : `${n} ${n === 1 ? "player is" : "players are"} on.`;
  $("subline").textContent = m.serverfps >= 50 ? "The server is running smoothly." : m.serverfps ? "The server is running slower than usual." : "";
  $("players").textContent = `${m.currentplayernum ?? n} / ${m.maxplayernum ?? "?"}`;
  $("fps").textContent = m.serverfps != null ? Number(m.serverfps).toFixed(0) : "–";
  $("fpsnote").textContent = m.serverframetime != null ? `${Number(m.serverframetime).toFixed(1)} ms per frame` : "";
  $("uptime").textContent = m.uptime != null ? duration(m.uptime) : "–";
  $("daysnote").textContent = m.days != null ? `Day ${m.days} in the world` : "";
  $("bases").textContent = m.basecampnum ?? "–";
  const rows = $("playerrows"); rows.replaceChildren();
  if (n === 0) { const tr = el("tr"); const td = el("td", "Nobody is online.", "empty"); td.colSpan = 5; tr.append(td); rows.append(tr); }
  for (const p of st.players) {
    const tr = el("tr");
    const acts = el("td", null, "acts");
    const k = el("button", "Kick", "small warn"); k.onclick = () => kick(p); acts.append(k);
    if (me.role !== "moderator") { const b = el("button", "Ban", "small danger"); b.onclick = () => ban(p); acts.append(b); }
    tr.append(el("td", p.name, "strong"), el("td", String(p.level), "num"), el("td", p.platform), el("td", `${Math.round(p.ping)} ms`, "num"), acts);
    rows.append(tr);
  }
}

// Small SVG line chart drawn from the samples; gaps where the server was down.
function chart(box, samples, key, label, color, maxHint) {
  const W = 760, H = 120, P = 28;
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", `0 0 ${W} ${H}`); svg.setAttribute("preserveAspectRatio", "none");
  box.replaceChildren(el("div", label, "chartlabel"), svg);
  if (samples.length < 2) { box.append(el("p", "Not enough history yet. Warden records a point every minute.", "note")); return; }
  const t0 = samples[0].ts, t1 = samples[samples.length - 1].ts;
  const max = Math.max(maxHint, ...samples.map((s) => s[key] || 0));
  const x = (t) => P + ((t - t0) / Math.max(1, t1 - t0)) * (W - P - 4);
  const y = (v) => H - 14 - (v / max) * (H - 26);
  for (const v of [0, max]) {
    const g = document.createElementNS(ns, "text");
    g.setAttribute("x", 2); g.setAttribute("y", y(v) + 4); g.setAttribute("class", "axis"); g.textContent = Math.round(v);
    svg.append(g);
  }
  let d = "", prev = null;
  for (const s of samples) {
    if (!s.online) { prev = null; continue; }
    d += `${prev ? "L" : "M"}${x(s.ts).toFixed(1)},${y(s[key] || 0).toFixed(1)} `;
    prev = s;
  }
  const path = document.createElementNS(ns, "path");
  path.setAttribute("d", d); path.setAttribute("fill", "none"); path.setAttribute("stroke", color); path.setAttribute("stroke-width", "2");
  path.setAttribute("vector-effect", "non-scaling-stroke");
  svg.append(path);
}
async function history() {
  try {
    const h = await api(`/api/history?range=${range}`);
    $("hist").textContent = range === "day" ? "The last 24 hours" : "The last 7 days";
    chart($("chart-players"), h, "players", "Players online", "var(--accent)", 4);
    chart($("chart-fps"), h, "fps", "Server FPS", "var(--chart2)", 60);
    const down = h.filter((s) => !s.online).length;
    $("histnote").textContent = down ? `The server didn't answer for about ${down} minute${down === 1 ? "" : "s"} in this period.` : "";
  } catch (e) {}
}
for (const [id, r] of [["r-day", "day"], ["r-week", "week"]]) $(id).addEventListener("click", () => {
  range = r; $("r-day").setAttribute("aria-pressed", r === "day"); $("r-week").setAttribute("aria-pressed", r === "week"); history();
});

// ---- players (roster), activity, admins
let roster = [];
function renderRoster() {
  const q = $("rsearch").value.trim().toLowerCase();
  const rows = $("rosterrows"); rows.replaceChildren();
  const list = roster.filter((p) => !q || p.name.toLowerCase().includes(q));
  if (!list.length) { const tr = el("tr"); const td = el("td", roster.length ? "No match." : "Nobody yet. Players appear here once Warden has seen them online.", "empty"); td.colSpan = 7; tr.append(td); rows.append(tr); }
  for (const p of list) {
    const tr = el("tr"); const acts = el("td", null, "acts");
    if (me.role !== "moderator") {
      const b = el("button", p.banned ? "Unban" : "Ban", p.banned ? "small" : "small danger");
      b.onclick = () => (p.banned ? unban(p) : ban(p)).then(loadRoster);
      acts.append(b);
    }
    const name = el("td", p.name, "strong"); if (p.banned) name.append(el("span", "banned", "tag"));
    tr.append(name, el("td", p.platform), el("td", String(p.level), "num"), el("td", when(p.firstSeen)), el("td", when(p.lastSeen)), el("td", duration(p.seconds), "num"), acts);
    rows.append(tr);
  }
}
async function loadRoster() { try { roster = await api("/api/roster"); renderRoster(); } catch (e) {} }
$("rsearch").addEventListener("input", renderRoster);
async function loadAudit() {
  try {
    const a = await api("/api/audit"); const rows = $("auditrows"); rows.replaceChildren();
    if (!a.length) { const tr = el("tr"); const td = el("td", "Nothing yet.", "empty"); td.colSpan = 4; tr.append(td); rows.append(tr); }
    for (const e of a) {
      const what = el("td", e.action + (e.target ? ` ${e.target}` : "")); if (!e.ok) what.append(el("span", "failed", "tag bad"));
      const tr = el("tr"); tr.append(el("td", when(e.ts)), el("td", e.who, "strong"), what, el("td", e.detail)); rows.append(tr);
    }
  } catch (e) {}
}
async function loadUsers() {
  try {
    const u = await api("/api/users"); const rows = $("userrows"); rows.replaceChildren();
    for (const x of u) { const tr = el("tr"); tr.append(el("td", x.name, "strong"), el("td", x.role), el("td", when(x.created)), el("td", when(x.lastLogin))); rows.append(tr); }
  } catch (e) {}
}
$("adduser").addEventListener("submit", async (e) => {
  e.preventDefault();
  try { await api("/api/users", { name: $("nu-name").value, password: $("nu-pw").value, role: $("nu-role").value }); $("nu-pw").value = ""; $("nu-msg").textContent = ""; toast("Admin added."); loadUsers(); }
  catch (err) { $("nu-msg").textContent = err.message; }
});

// ---- routing and polling
function route() {
  const v = (location.hash.replace("#/", "") || "dash");
  const view = ["players", "activity", "admins"].includes(v) ? v : "dash";
  for (const s of document.querySelectorAll(".view")) s.hidden = s.id !== `view-${view}`;
  for (const a of document.querySelectorAll(".side a")) a.classList.toggle("on", a.dataset.view === view);
  if (view === "players") loadRoster();
  if (view === "activity") loadAudit();
  if (view === "admins") loadUsers();
}
window.addEventListener("hashchange", route);
async function refresh() { try { render(await api("/api/status")); } catch (e) {} }

(async function start() {
  try {
    const out = await api("/api/me");
    if (out.user) signedIn(out); else showGate(out.setupNeeded);
  } catch (e) { showGate(false); }
  setInterval(() => { if (me && !$("layout").hidden) refresh(); }, 10000);
  setInterval(() => { if (me && !$("layout").hidden) history(); }, 60000);
})();
