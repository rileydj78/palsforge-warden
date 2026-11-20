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
function showGate(noAccounts) {
  $("layout").hidden = true; $("signout").hidden = true; $("state").hidden = true; $("gate").hidden = false;
  $("gatetitle").textContent = "Sign in";
  $("gatesub").textContent = noAccounts ? "There are no accounts yet. The owner adds them on the server machine (Start menu: Palsforge Warden → Admins)." : "";
  for (const id of ["g-name", "g-pw", "gatebtn"]) $(id).hidden = !!noAccounts;
  for (const l of document.querySelectorAll("#gateform label")) l.hidden = !!noAccounts;
  $("g-name").focus();
}
$("gateform").addEventListener("submit", async (e) => {
  e.preventDefault();
  try {
    const out = await api("/api/login", { name: $("g-name").value, password: $("g-pw").value });
    $("g-pw").value = ""; $("gatemsg").textContent = "";
    signedIn(out);
  } catch (err) { $("gatemsg").textContent = err.message; }
});
$("signout").addEventListener("click", async () => { try { await api("/api/logout", {}); } catch (e) {} showGate(false); });

function signedIn(out) {
  me = out.user; csrf = out.csrf;
  $("gate").hidden = true; $("layout").hidden = false; $("signout").hidden = false; $("state").hidden = false;
  $("signout").textContent = `Sign out (${me.name})`;
  $("signout").hidden = !!out.local; // on the server machine there's nobody to sign out
  $("adminsnav").hidden = me.role !== "owner";
  $("settingsnav").hidden = me.role !== "owner";
  for (const id of ["b-save", "b-shutdown"]) $(id).hidden = me.role === "moderator";
  for (const e of document.querySelectorAll(".admin-only")) e.hidden = me.role === "moderator";
  api("/api/server").then(() => { $("servernav").hidden = false; $("backupsnav").hidden = false; $("logsnav").hidden = me.role === "moderator"; $("gamenav").hidden = me.role === "moderator"; $("modsnav").hidden = me.role === "moderator"; }).catch(() => {});
  api("/api/paldefender").then((pd) => { pdConnected = pd.connected && !pd.error; }).catch(() => {});
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
let pdConnected = false;
async function ban(p) {
  const m = await ask({ title: `Ban ${p.name}?`, text: "They can't join until someone unbans them." + (pdConnected ? " Start the reason with IP: to ban their IP address too (for players who come back on new accounts)." : ""), label: "Reason (they see it)", ok: "Ban" });
  if (m == null) return;
  const ip = pdConnected && /^ip:/i.test(m.trim());
  doAction("ban", { userId: p.userId, name: p.name, message: m.trim().replace(/^ip:\s*/i, ""), ip }, `${p.name} was banned${ip ? " (and their IP address)" : ""}.`);
}
async function message(p) {
  const m = await ask({ title: `Message ${p.name}`, text: "Only they see it, in their chat.", label: "Message", ok: "Send", required: true });
  if (m) doAction("message", { userId: p.userId, name: p.name, message: m }, "Sent.");
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
  if (n === 0) { const tr = el("tr"); const td = el("td", "Nobody is online.", "empty"); td.colSpan = 6; tr.append(td); rows.append(tr); }
  for (const p of st.players) {
    const tr = el("tr");
    const acts = el("td", null, "acts");
    if (pdConnected) { const msg = el("button", "Message", "small"); msg.onclick = () => message(p); acts.append(msg); }
    const k = el("button", "Kick", "small warn"); k.onclick = () => kick(p); acts.append(k);
    if (me.role !== "moderator") { const b = el("button", "Ban", "small danger"); b.onclick = () => ban(p); acts.append(b); }
    tr.append(el("td", p.name, "strong"), el("td", p.guild || "–"), el("td", String(p.level), "num"), el("td", p.platform), el("td", `${Math.round(p.ping)} ms`, "num"), acts);
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
async function loadRoster() { try { roster = await api("/api/roster"); renderRoster(); } catch (e) {} loadBans(); }
const SRC = { palworld: "The game", paldefender: "PalDefender" };
async function loadBans() {
  try {
    const bans = await api("/api/bans"), rows = $("banrows"); rows.replaceChildren();
    if (!bans.length) { const tr = el("tr"); const td = el("td", "Nobody is banned.", "empty"); td.colSpan = 4; tr.append(td); rows.append(tr); }
    for (const b of bans) {
      const tr = el("tr"), acts = el("td", null, "acts");
      if (me.role !== "moderator" && !b.ip) {
        const u = el("button", "Unban", "small");
        u.onclick = async () => { try { await api("/api/bans/lift", { source: b.source, id: b.id }); toast("Unbanned."); loadRoster(); } catch (e) { toast(e.message); } };
        acts.append(u);
      }
      tr.append(el("td", b.name || b.id, "strong"), el("td", SRC[b.source] || b.source), el("td", b.ip ? "IP address ban (lift it in PalDefender)" : (b.reason || "–")), acts);
      rows.append(tr);
    }
  } catch (e) {}
}
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
    if (!u.length) { const tr = el("tr"); const td = el("td", "No remote accounts yet. On this machine you never need one.", "empty"); td.colSpan = 5; tr.append(td); rows.append(tr); }
    for (const x of u) {
      const tr = el("tr"), acts = el("td", null, "acts"), b = el("button", "Remove", "small danger");
      b.onclick = async () => { const ok = await ask({ title: `Remove ${x.name}?`, text: "They're signed out at once and can't sign in again.", label: "Type REMOVE to confirm", confirmWord: "REMOVE", ok: "Remove" }); if (ok == null) return; try { await api("/api/users/remove", { name: x.name }); toast("Removed."); loadUsers(); } catch (e) { toast(e.message); } };
      acts.append(b);
      tr.append(el("td", x.name, "strong"), el("td", x.role), el("td", when(x.created)), el("td", when(x.lastLogin)), acts); rows.append(tr);
    }
  } catch (e) {}
}
$("adduser").addEventListener("submit", async (e) => {
  e.preventDefault();
  try { await api("/api/users", { name: $("nu-name").value, password: $("nu-pw").value, role: $("nu-role").value }); $("nu-pw").value = ""; $("nu-msg").textContent = ""; toast("Admin added."); loadUsers(); }
  catch (err) { $("nu-msg").textContent = err.message; }
});

// ---- server control, nightly routine, backups
const size = (n) => n > 1 << 30 ? `${(n / (1 << 30)).toFixed(1)} GB` : n > 1 << 20 ? `${(n / (1 << 20)).toFixed(0)} MB` : `${Math.max(1, Math.round(n / 1024))} KB`;
const when2 = (iso) => iso ? new Date(iso).toLocaleString([], { weekday: "short", hour: "numeric", minute: "2-digit" }) : "–";
async function loadServer() {
  try {
    const s = await api("/api/server"), h = s.host;
    $("srv-state").textContent = h.running ? "Running" : "Stopped";
    $("srv-mode").textContent = h.mode === "supervise" ? "Warden runs it and restarts it after a crash" : "Run by another program; Warden starts and stops it";
    $("srv-build").textContent = s.build || "–";
    $("srv-update").textContent = s.update ? "Updated by the nightly routine" : "Updates are off";
    $("srv-next").textContent = s.nextRun ? when2(s.nextRun) : "Off";
    $("srv-nightly").textContent = s.nightly.enabled ? `Warnings ${s.nightly.warnMinutes.join(", ")} min before` : "Turn it on in warden.json";
    $("srv-crashes").textContent = h.crashes24h;
    $("srv-mem").textContent = s.memory ? `${(s.memory / (1 << 30)).toFixed(1)} GB` : "–";
    $("srv-memnote").textContent = s.health && s.health.memoryLimitGB ? `Restarts with warnings above ${s.health.memoryLimitGB} GB` : "No memory limit set";
    $("srv-paused").textContent = h.pausedTill && new Date(h.pausedTill) > new Date() ? `Restarts paused until ${when2(h.pausedTill)}` : "";
    $("srv-head").textContent = s.routineRunning ? "The nightly routine is running…" : "Server";
    $("srv-sub").textContent = s.offsite ? "" : "Backups are kept on this machine only. Add a second place (backups.offsite) so a dead disk can't take them.";
    try {
      const pd = await api("/api/paldefender");
      $("pd-card").hidden = !pd.installed && !pd.connected;
      pdConnected = pd.connected && !pd.error;
      $("pd-state").textContent = pd.connected ? (pd.error ? `Connected, but: ${pd.error}` : `Connected: ${pd.version}. Guilds, exact map positions, private messages and IP bans are on.`) : "Installed, not connected. Connecting turns its API on for this machine only and gives Warden its own token; it takes effect after the server's next restart.";
      $("pd-connect").hidden = pd.connected || me.role !== "owner";
    } catch (e) {}
    const runs = await api("/api/runs"), box = $("runs"); box.replaceChildren();
    if (!runs.length) box.append(el("p", "No runs yet.", "note"));
    for (const r of runs.slice(0, 10)) {
      const d = el("details", null, "run"), sm = el("summary");
      sm.append(el("span", r.ok ? "Completed" : "Problems", r.ok ? "tag" : "tag bad"), el("span", ` ${r.kind} · ${when(r.started)}`));
      d.append(sm);
      const t = el("table"), tb = el("tbody");
      for (const st of (r.report.steps || [])) { const tr = el("tr"); tr.append(el("td", st.ok ? "✓" : "✗", st.ok ? "" : "bad"), el("td", st.name, "strong"), el("td", st.detail || "")); tb.append(tr); }
      t.append(tb); d.append(t); box.append(d);
    }
    const ev = await api("/api/events"), rows = $("eventrows"); rows.replaceChildren();
    if (!ev.length) { const tr = el("tr"); const td = el("td", "Nothing yet.", "empty"); td.colSpan = 3; tr.append(td); rows.append(tr); }
    for (const e of ev.slice(0, 50)) { const tr = el("tr"); tr.append(el("td", when(e.ts)), el("td", e.kind, "strong"), el("td", e.text)); rows.append(tr); }
  } catch (e) {}
}
$("pd-connect").addEventListener("click", async () => {
  const ok = await ask({ title: "Connect PalDefender?", text: "Warden turns PalDefender's API on (this machine only), creates its own token, and restarts itself. PalDefender picks it up at the server's next restart.", label: "Type CONNECT to confirm", confirmWord: "CONNECT", ok: "Connect" });
  if (ok == null) return;
  try { await api("/api/paldefender/connect", {}); toast("Connected. Warden is restarting."); setTimeout(() => location.reload(), 4000); } catch (e) { toast(e.message); }
});
$("b-start").addEventListener("click", async () => { try { await api("/api/server/start", {}); toast("Starting the server."); setTimeout(loadServer, 3000); } catch (e) { toast(e.message); } });
$("b-stop").addEventListener("click", async () => {
  const ok = await ask({ title: "Stop the server?", text: "Players are told it is stopping. It stays off until someone starts it.", label: "Type STOP to confirm", confirmWord: "STOP", ok: "Stop" });
  if (ok == null) return;
  try { await api("/api/server/stop", {}); toast("The server is stopped."); loadServer(); } catch (e) { toast(e.message); }
});
$("b-routine").addEventListener("click", async () => {
  const ok = await ask({ title: "Run the nightly routine now?", text: "Players get warnings, then the world is saved, the server stops, is backed up and updated, and starts again. Type NOW to run it without the warnings, or RUN to include them.", label: "RUN or NOW" });
  if (ok == null) return;
  const w = ok.trim().toUpperCase();
  if (w !== "RUN" && w !== "NOW") { toast("Not confirmed; nothing was done."); return; }
  try { await api("/api/routine/run", { warn: w === "RUN", update: true }); toast("The routine has started."); setTimeout(loadServer, 2000); } catch (e) { toast(e.message); }
});
async function loadBackups() {
  try {
    const places = await api("/api/backups"), box = $("bk-places"); box.replaceChildren();
    places.forEach((p, i) => {
      const c = el("section", null, "card");
      c.append(el("h2", i === 0 ? "On this machine" : "Second place"), el("p", p.dir, "note mono"));
      if (p.error) c.append(el("p", p.error, "problem"));
      const t = el("table"), tb = el("tbody");
      const hr = el("tr"); for (const h of ["Backup", "Kind", "Made", "Size", "Checked", ""]) hr.append(el("th", h)); const th = el("thead"); th.append(hr); t.append(th);
      if (!p.archives.length) { const tr = el("tr"); const td = el("td", "None yet.", "empty"); td.colSpan = 6; tr.append(td); tb.append(tr); }
      for (const a of p.archives) {
        const tr = el("tr"), acts = el("td", null, "acts");
        if (me.role === "owner" && a.kind === "data") { const b = el("button", "Restore…", "small warn"); b.onclick = () => restore(i, a.name); acts.append(b); }
        tr.append(el("td", a.name, "mono"), el("td", a.kind === "server" ? "Whole server" : "World, settings, mods"), el("td", when2(a.when)), el("td", size(a.size), "num"), el("td", a.sha256 ? "✓ checksum" : "–"), acts); tb.append(tr);
      }
      t.append(tb); const w = el("div", null, "tablewrap"); w.append(t); c.append(w); box.append(c);
    });
  } catch (e) {}
}
async function restore(place, name) {
  let pv;
  try { pv = await api(`/api/backups/preview?place=${place}&name=${encodeURIComponent(name)}`); } catch (e) { toast(e.message); return; }
  const what = `${pv.files} files (${size(pv.bytes)}): ${pv.worlds.length} world${pv.worlds.length === 1 ? "" : "s"}${pv.settings ? ", the server settings" : ""}${pv.modFiles ? `, ${pv.modFiles} mod files` : ""}.`;
  const ok = await ask({ title: `Restore ${name}?`, text: `It holds ${what} Players are warned, the server stops, a safety backup of the world as it is now is made, the current folders are kept aside, and the server starts again on the restored world.`, label: "Type RESTORE to confirm", confirmWord: "RESTORE", ok: "Restore" });
  if (ok == null) return;
  try { await api("/api/backups/restore", { place, name }); toast("Restoring. Follow it on the Server page."); location.hash = "#/server"; } catch (e) { toast(e.message); }
}
$("b-backup").addEventListener("click", async () => {
  toast("Backing up… this can take a minute.");
  try { const r = await api("/api/backups/now", {}); toast(`Backed up: ${r.made.join(", ")}`); loadBackups(); } catch (e) { toast(e.message); }
});

// ---- map: a plain grid in the game's coordinates (the map artwork is Pocketpair's, so it isn't used)
async function drawMap() {
  let st; try { st = await api("/api/status"); } catch (e) { return; }
  const ns = "http://www.w3.org/2000/svg", svg = document.createElementNS(ns, "svg");
  const ps = st.players || [];
  let lo = -1000, hi = 1000; // the island's usual extent; grow to fit anyone outside it
  for (const p of ps) { lo = Math.min(lo, p.mapX - 50, p.mapY - 50); hi = Math.max(hi, p.mapX + 50, p.mapY + 50); }
  const S = 800, k = S / (hi - lo), px = (v) => (v - lo) * k, py = (v) => S - (v - lo) * k;
  svg.setAttribute("viewBox", `0 0 ${S} ${S}`);
  const line = (x1, y1, x2, y2, c) => { const l = document.createElementNS(ns, "line"); l.setAttribute("x1", x1); l.setAttribute("y1", y1); l.setAttribute("x2", x2); l.setAttribute("y2", y2); l.setAttribute("class", c); svg.append(l); };
  const text = (x, y, t, c) => { const e = document.createElementNS(ns, "text"); e.setAttribute("x", x); e.setAttribute("y", y); e.setAttribute("class", c); e.textContent = t; svg.append(e); };
  for (let v = Math.ceil(lo / 250) * 250; v <= hi; v += 250) {
    line(px(v), 0, px(v), S, v === 0 ? "axis0" : "grid"); line(0, py(v), S, py(v), v === 0 ? "axis0" : "grid");
    text(px(v) + 3, S - 4, v, "gridlbl"); text(3, py(v) - 3, v, "gridlbl");
  }
  for (const p of ps) {
    const c = document.createElementNS(ns, "circle"); c.setAttribute("cx", px(p.mapX)); c.setAttribute("cy", py(p.mapY)); c.setAttribute("r", 7); c.setAttribute("class", "dot"); svg.append(c);
    text(px(p.mapX) + 10, py(p.mapY) + 4, p.name, "dotlbl");
  }
  $("map").replaceChildren(svg);
  const rows = $("maprows"); rows.replaceChildren();
  if (!ps.length) { const tr = el("tr"); const td = el("td", "Nobody is online.", "empty"); td.colSpan = 3; tr.append(td); rows.append(tr); }
  for (const p of ps) { const tr = el("tr"); tr.append(el("td", p.name, "strong"), el("td", `${p.mapX}, ${p.mapY}`, "num"), el("td", String(p.level), "num")); rows.append(tr); }
}
setInterval(() => { if (me && !$("view-map").hidden) drawMap(); }, 10000);

// ---- the game's settings
let game = null;
async function loadGame() {
  try {
    game = await api("/api/game-settings");
    const box = $("g-groups"); box.replaceChildren();
    const groups = [...new Set(game.catalog.map((o) => o.group))];
    for (const g of groups) {
      const card = el("section", null, "card form wide"); card.append(el("h2", g));
      for (const o of game.catalog.filter((x) => x.group === g)) {
        const id = "g-" + o.key, cur = game.values[o.key];
        const lab = el("label", o.label); lab.htmlFor = id;
        let input;
        if (o.type === "bool") { input = el("select"); for (const [v, t] of [["True", "On"], ["False", "Off"]]) { const op = el("option", t); op.value = v; input.append(op); } input.value = /^true$/i.test(cur ?? o.default) ? "True" : "False"; }
        else if (o.type === "enum") { input = el("select"); for (const c of o.choices) { const op = el("option", c); op.value = c; input.append(op); } input.value = cur ?? o.default; }
        else { input = el("input"); input.value = o.type === "secret" ? "" : (cur ?? ""); if (o.type === "float" || o.type === "int") { input.inputMode = "decimal"; input.value = cur != null ? String(parseFloat(cur)) : ""; } if (o.type === "secret") { input.type = "password"; input.placeholder = game.secretsSet[o.key] ? "Set (type to change; a space removes it)" : "Not set"; input.autocomplete = "new-password"; } }
        input.id = id; input.dataset.key = o.key; input.dataset.orig = input.value; input.disabled = !!o.locked;
        input.addEventListener("input", () => input.classList.toggle("changed", input.value !== input.dataset.orig));
        input.addEventListener("change", () => input.classList.toggle("changed", input.value !== input.dataset.orig));
        const def = o.default ? ` Default: ${o.type === "float" ? parseFloat(o.default) : o.type === "bool" ? (/true/i.test(o.default) ? "on" : "off") : o.default}.` : "";
        const lim = o.max ? ` Range ${o.min} to ${o.max}.` : "";
        card.append(lab, input, el("p", o.help + def + lim + (o.locked ? " (Not editable here.)" : ""), "note"));
      }
      box.append(card);
    }
    const np = Object.keys(game.pending || {}).length;
    $("g-sub").textContent = np ? `${np} change${np === 1 ? " is" : "s are"} waiting: Palworld rewrites its settings when it stops, so Warden writes them at the next stop (the nightly routine, or a restart from here).` : "Changes are written while the server is stopped (Palworld rewrites its settings when it shuts down). Every change keeps a copy of the old file.";
    $("g-unknown-card").hidden = !game.unknown.length; $("g-unknown").textContent = game.unknown.join(", ");
  } catch (e) { toast(e.message); }
}
function gameEdits() {
  const out = {};
  for (const i of document.querySelectorAll("#g-groups [data-key]")) {
    if (i.value === i.dataset.orig) continue;
    out[i.dataset.key] = i.type === "password" && i.value.trim() === "" ? "" : i.value;
  }
  return out;
}
$("g-review").addEventListener("click", async () => {
  const values = gameEdits();
  if (!Object.keys(values).length) { toast("Nothing has changed."); return; }
  let plan;
  try { plan = await api("/api/game-settings/plan", { values }); } catch (e) { toast(e.message); return; }
  if (!plan.changes.length) { toast("Those values are the same as now."); return; }
  const list = plan.changes.map((c) => `${c.label}: ${c.old || "(empty)"} → ${c.new || "(empty)"}`).join("\n");
  const ans = await ask({ title: `Apply ${plan.changes.length} change${plan.changes.length === 1 ? "" : "s"}?`, text: list + "\n\nType LATER to apply them at the next restart, or NOW to restart the server in 1 minute (players are warned).", label: "LATER or NOW" });
  if (ans == null) return;
  const w = ans.trim().toUpperCase();
  if (w !== "LATER" && w !== "NOW") { toast("Not confirmed; nothing was changed."); return; }
  try { await api("/api/game-settings/apply", { values, restart: w === "NOW" ? "now" : "later" }); toast(w === "NOW" ? "Saved. The server restarts in 1 minute and starts with them." : "Saved. Warden writes them at the next stop."); loadGame(); } catch (e) { toast(e.message); }
});

// ---- mods
const KIND = { pak: "PAK mod", ue4ss: "UE4SS Lua mod", palschema: "PalSchema mod", plugin: "Server plugin" };
async function loadMods() {
  try {
    const r = await api("/api/mods"), rows = $("modrows"); rows.replaceChildren();
    const pending = r.mods.filter((m) => m.pending).length;
    $("md-apply").hidden = !(pending && r.running);
    $("md-sub").textContent = r.running ? (pending ? `${pending} change${pending === 1 ? "" : "s"} waiting: they apply at the next nightly run, or now with a restart.` : "Changes wait for the next nightly stop (mod files are locked while the server runs).") : "The server is stopped: changes apply straight away.";
    const pb = $("md-problems"); pb.replaceChildren();
    for (const p of r.problems) pb.append(el("p", `${p.mod} ${p.text}.`, "problem"));
    if (!r.mods.length) { const tr = el("tr"); const td = el("td", "No mods found.", "empty"); td.colSpan = 6; tr.append(td); rows.append(tr); }
    for (const m of r.mods) {
      const tr = el("tr"), acts = el("td", null, "acts");
      const goingOn = m.pending ? m.pending === "enable" : m.enabled;
      const b = el("button", goingOn ? "Turn off" : "Turn on", goingOn ? "small warn" : "small");
      b.onclick = async () => { try { const x = await api("/api/mods/toggle", { id: m.id, op: goingOn ? "disable" : "enable" }); toast(x.applied ? "Done." : "Queued."); loadMods(); } catch (e) { toast(e.message); } };
      acts.append(b);
      const state = m.pending ? `${m.enabled ? "On" : "Off"} → ${m.pending === "enable" ? "on" : "off"}` : (m.enabled ? "On" : "Off");
      tr.append(el("td", m.name, "strong"), el("td", KIND[m.kind] || m.kind), el("td", size(m.size), "num"), el("td", m.changed ? new Date(m.changed).toLocaleDateString() : "–"), el("td", state, m.pending ? "strong" : ""), acts);
      rows.append(tr);
    }
  } catch (e) {}
}
$("md-upload").addEventListener("submit", async (e) => {
  e.preventDefault();
  const f = $("md-file").files[0]; if (!f) return;
  const fd = new FormData(); fd.append("file", f);
  toast("Uploading…");
  try {
    const r = await fetch("/api/mods/install", { method: "POST", headers: { "X-CSRF": csrf }, body: fd });
    const d = await r.json().catch(() => ({}));
    if (!r.ok) throw new Error(d.error || `Warden answered ${r.status}`);
    toast(d.applied ? `Installed ${d.id}.` : `${d.id} is ready: it goes in at the next nightly stop, or now with "Apply now".`);
    $("md-file").value = ""; loadMods();
  } catch (err) { toast(err.message); }
});
$("md-apply").addEventListener("click", async () => {
  const ok = await ask({ title: "Apply the mod changes now?", text: "Players get a minute's warning, the server stops, the mods change, and it starts again.", label: "Type APPLY to confirm", confirmWord: "APPLY", ok: "Apply" });
  if (ok == null) return;
  try { await api("/api/mods/apply-now", {}); toast("Restarting in 1 minute to change mods."); setTimeout(loadMods, 90000); } catch (e) { toast(e.message); }
});

// ---- logs
async function loadLogs(first) {
  const sel = $("lg-src");
  try {
    if (first || !sel.options.length) {
      const r = await api("/api/logs"); const cur = sel.value; sel.replaceChildren();
      for (const s of r.sources) { const o = el("option", s.label); o.value = s.id; sel.append(o); }
      if (cur) sel.value = cur;
      if (!r.sources.length) { $("lg-out").textContent = "No logs found yet."; return; }
    }
    const q = $("lg-q").value.trim();
    const r = await api(`/api/logs?source=${encodeURIComponent(sel.value)}&lines=400&q=${encodeURIComponent(q)}`);
    const out = $("lg-out"), atEnd = out.scrollTop + out.clientHeight >= out.scrollHeight - 30;
    out.textContent = r.lines.length ? r.lines.join("\n") : (q ? "No lines match." : "The log is empty.");
    if (atEnd || first) out.scrollTop = out.scrollHeight;
  } catch (e) {}
}
$("lg-src").addEventListener("change", () => loadLogs(false));
let lgT; $("lg-q").addEventListener("input", () => { clearTimeout(lgT); lgT = setTimeout(() => loadLogs(false), 300); });
setInterval(() => { if (!$("view-logs").hidden && $("lg-follow").checked) loadLogs(false); }, 5000);

// ---- Warden's own settings (owner)
let cfg = null;
const lines = (t) => t.split("\n").map((x) => x.trim()).filter(Boolean);
function showMode() { const ext = $("s-c-mode").value === "external"; $("s-ext").hidden = !ext; $("s-sup").hidden = ext; }
$("s-c-mode").addEventListener("change", showMode);
async function loadSettings() {
  try {
    const r = await api("/api/settings"); cfg = r.settings;
    $("set-path").textContent = `Saved in ${r.path}. The server folder is ${cfg.serverDir}.`;
    $("set-error").hidden = !r.error; $("set-error").textContent = r.error ? `Needs attention: ${r.error}` : "";
    $("s-n-on").checked = cfg.nightly.enabled; $("s-n-time").value = cfg.nightly.time; $("s-n-warn").value = (cfg.nightly.warnMinutes || []).join(", ");
    $("s-b-dir").value = cfg.backups.dir; $("s-b-off").value = (cfg.backups.offsite || []).join("\n"); $("s-b-img").value = cfg.backups.image;
    $("s-b-d").value = cfg.backups.keepDaily; $("s-b-w").value = cfg.backups.keepWeekly;
    $("s-u-on").checked = cfg.update.enabled; $("s-u-path").value = cfg.update.steamcmd; $("s-u-val").checked = cfg.update.validate;
    $("s-c-mode").value = cfg.control.mode; $("s-c-exe").value = cfg.control.exe; $("s-c-args").value = (cfg.control.args || []).join(" ");
    $("s-c-aff").value = cfg.control.affinity; $("s-c-start").value = (cfg.control.startCommand || []).join("\n"); $("s-c-stop").value = cfg.control.stopMarker;
    const hl = cfg.health || {};
    $("s-h-frozen").value = hl.restartFrozenMinutes ?? 5; $("s-h-mem").value = hl.memoryLimitGB || 0;
    const b = cfg.broadcasts || {};
    $("s-m-on").checked = !!b.enabled; $("s-m-list").value = (b.messages || []).join("\n"); $("s-m-every").value = b.everyMinutes || ""; $("s-m-welcome").value = b.welcome || "";
    const n = cfg.notify || {};
    $("s-x-dc").value = n.discordWebhook || ""; $("s-x-ntfy").value = n.ntfyUrl || ""; $("s-x-tok").value = n.ntfyToken || ""; $("s-x-ok").checked = !!n.routineOk;
    showMode();
  } catch (e) {}
}
const notifyForm = () => ({ discordWebhook: $("s-x-dc").value.trim(), ntfyUrl: $("s-x-ntfy").value.trim(), ntfyToken: $("s-x-tok").value.trim(), routineOk: $("s-x-ok").checked });
$("s-x-test").addEventListener("click", async () => { try { await api("/api/notify/test", notifyForm()); toast("Test sent."); } catch (e) { toast(e.message); } });
$("setform").addEventListener("submit", async (e) => {
  e.preventDefault();
  const c = structuredClone(cfg);
  c.nightly = { enabled: $("s-n-on").checked, time: $("s-n-time").value.trim(), warnMinutes: $("s-n-warn").value.split(",").map((x) => parseInt(x, 10)).filter((x) => !isNaN(x)) };
  Object.assign(c.backups, { dir: $("s-b-dir").value.trim(), offsite: lines($("s-b-off").value), image: $("s-b-img").value, keepDaily: +$("s-b-d").value, keepWeekly: +$("s-b-w").value });
  Object.assign(c.update, { enabled: $("s-u-on").checked, steamcmd: $("s-u-path").value.trim(), validate: $("s-u-val").checked });
  c.notify = notifyForm();
  c.health = { restartFrozenMinutes: parseInt($("s-h-frozen").value, 10) || 0, memoryLimitGB: parseFloat($("s-h-mem").value) || 0 };
  c.broadcasts = { enabled: $("s-m-on").checked, messages: lines($("s-m-list").value), everyMinutes: parseInt($("s-m-every").value, 10) || 0, welcome: $("s-m-welcome").value.trim() };
  Object.assign(c.control, { mode: $("s-c-mode").value, exe: $("s-c-exe").value.trim(), args: $("s-c-args").value.split(" ").filter(Boolean), affinity: $("s-c-aff").value.trim(), startCommand: lines($("s-c-start").value), stopMarker: $("s-c-stop").value.trim() });
  try {
    await api("/api/settings", c); $("set-msg").textContent = "";
    toast("Saved. Warden is restarting; this page comes back in a few seconds.");
    setTimeout(() => location.reload(), 4000);
  } catch (err) { $("set-msg").textContent = err.message; }
});

// ---- routing and polling
function route() {
  const v = (location.hash.replace("#/", "") || "dash");
  const view = ["players", "activity", "admins", "server", "backups", "settings", "logs", "game", "map", "mods"].includes(v) ? v : "dash";
  for (const s of document.querySelectorAll(".view")) s.hidden = s.id !== `view-${view}`;
  for (const a of document.querySelectorAll(".side a")) a.classList.toggle("on", a.dataset.view === view);
  if (view === "players") loadRoster();
  if (view === "activity") loadAudit();
  if (view === "admins") loadUsers();
  if (view === "server") loadServer();
  if (view === "backups") loadBackups();
  if (view === "settings") loadSettings();
  if (view === "logs") loadLogs(true);
  if (view === "game") loadGame();
  if (view === "map") drawMap();
  if (view === "mods") loadMods();
}
window.addEventListener("hashchange", route);
async function refresh() {
  try { render(await api("/api/status")); } catch (e) {}
  try { // safety facts first: when was the world last backed up, and when is the next run
    const s = await api("/api/server");
    $("st-backup").hidden = false; $("st-next").hidden = false;
    const lr = s.lastRun;
    $("lastbk").textContent = lr ? when(lr.started) : "Never";
    $("lastbknote").textContent = lr ? (lr.ok ? `${lr.kind} run completed` : `${lr.kind} run had problems: see Server`) : "No backup has run yet";
    $("st-backup").classList.toggle("warn", !lr || !lr.ok || Date.now() / 1000 - lr.started > 36 * 3600);
    $("nextrun").textContent = s.nextRun ? when2(s.nextRun) : "Off";
    $("nextrunnote").textContent = s.nextRun ? (s.offsite ? "Backups copied to a second place" : "No second place for backups yet") : "Turn it on in Settings";
  } catch (e) {}
}

(async function start() {
  try {
    const out = await api("/api/me");
    if (out.user) signedIn(out); else showGate(out.accounts === 0);
  } catch (e) { showGate(false); }
  setInterval(() => { if (me && !$("layout").hidden) refresh(); }, 10000);
  setInterval(() => { if (me && !$("layout").hidden) history(); }, 60000);
})();
