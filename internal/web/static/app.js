"use strict";
// Warden panel: polls /api/status and fills the page. No framework, no third-party code.
const $ = (id) => document.getElementById(id);

function duration(s) {
  const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  return `${m}m`;
}

function cell(text, cls) {
  const td = document.createElement("td");
  td.textContent = text;
  if (cls) td.className = cls;
  return td;
}

function render(st) {
  const state = $("state");
  if (!st.online) {
    state.classList.add("down");
    $("statetext").textContent = "Not answering";
    $("headline").textContent = "The server isn't answering";
    $("subline").textContent = st.error || "";
    return;
  }
  state.classList.remove("down");
  const m = st.metrics || {};
  const n = st.players.length;
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
  const rows = $("playerrows");
  rows.replaceChildren();
  if (n === 0) {
    const tr = document.createElement("tr");
    const td = cell("Nobody is online.", "empty");
    td.colSpan = 4;
    tr.append(td);
    rows.append(tr);
  }
  for (const p of st.players) {
    const tr = document.createElement("tr");
    tr.append(cell(p.name), cell(String(p.level), "num"), cell(p.platform), cell(`${Math.round(p.ping)} ms`, "num"));
    rows.append(tr);
  }
  $("foot").textContent = `Warden ${st.warden} · updated ${new Date(st.checked).toLocaleTimeString()}`;
}

async function tick() {
  try {
    const r = await fetch("/api/status", { cache: "no-store" });
    render(await r.json());
  } catch (e) {
    $("statetext").textContent = "Warden isn't answering";
    $("state").classList.add("down");
  }
}
tick();
setInterval(tick, 10000);
