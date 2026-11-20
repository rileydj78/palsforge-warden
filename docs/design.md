# Palsforge Warden: design (draft)

Status: first draft, 2026-11-19. Decisions marked **(decided)**; everything else is a proposal to
discuss. The research behind it is in [research.md](research.md).


## Status (2026-11-20)

Built, tested and running on a live server (Linux tests + CI; Windows builds wait for signing,
because Smart App Control blocks new unsigned programs):

| Area | State |
|---|---|
| Dashboard, history graphs, last backup / next run | done |
| Players: online, roster, kick / ban / unban, announce | done |
| Logins, roles, audit log, CSRF, loopback-only | done |
| Nightly routine (warn, save, stop, back up, update, start, health check) | done |
| Backups: data + image, checksums, verified second places, retention | done |
| Restore: preview, safety backup, reversible | done |
| Server control: supervise (crash restarts) or external (watchdog / service) | done |
| Warden settings page; `warden install / open / remote` | done |
| Notifications: Discord, ntfy | done |
| Logs viewer (IPs masked) | done |
| Game settings editor | done |
| Map (players) | done; coordinate transform to be checked in game |
| Mods: scan, mistakes, on/off, zip install | done |
| PalDefender: connect, guilds, map positions, messages, IP bans | done |
| Signed Windows release | waiting on SignPath Foundation |
| Access: no sign-in on the server; accounts on a separate admin port | done |
| Server messages + welcome; join/leave alerts; live Discord status message | done |
| Frozen-server and memory-limit restarts; save watchdog | done |
| Unified ban list; player notes; guilds; bases on the map; player inventory/Pals; give items/Pals | done |
| Weekly events; game-settings presets; settings written only while stopped | done |
| Proven restores; crash-aware retention (pinned backups); backup download | done |
| Setup check with exposure check | done |
| Allow-list, Discord commands, save migration, multi-server, translations, Docker, first-run installer | next (see feature-survey.md) |

The full competitor comparison and the parity gaps are in [feature-survey.md](feature-survey.md).

## 1. What it is

**One program on the machine that runs the Palworld server** (Windows first, Linux too), used in
exactly two ways **(decided 2026-11-19)**:

1. **Local administration:** on the server machine itself, `warden.exe` opens its own window (the
   same panel, in a native window; no browser needed).
2. **Remote administration:** the same panel as a **web page reached over Tailscale only**
   (`tailscale serve`, HTTPS, visible only on the admin's own tailnet).

There is **no phone app**. The web page still works in a phone's browser over Tailscale, but
that is the web page, not a separate app.

```
 warden.exe window (local) ─┐                  ┌─ Palworld REST API (127.0.0.1:8212, never exposed)
                            ├─ Warden panel ───┤─ PalDefender REST (127.0.0.1:17993, optional)
 browser over Tailscale ────┘  (127.0.0.1 +    ├─ server files: saves, PalWorldSettings.ini, logs
   (remote, tailnet only)       tailscale serve)
                                              ├─ the server process (start, stop, watchdog)
                                              └─ SteamCMD (updates), backup targets
```

Because Warden runs next to the server it can do what panel-only tools can't: back up files, edit
settings, run the nightly routine and restart the server, with **one install**.

## 1b. For any Palworld server **(decided: anyone can use it)**

Warden must not assume the Palsforge setup. It has to fit how people actually run servers:

| How the server runs | How Warden fits | What works |
|---|---|---|
| **Self-hosted on Windows** (SteamCMD, often with PalDefender) | Warden installed on the same machine, as a Windows service | Everything |
| **Self-hosted on Linux** (native or Wine) | Warden as a systemd service | Everything except Windows-only mod loaders (see §2b) |
| **Docker** (e.g. the popular `thijsvanloef/palworld-server-docker` image) | Warden as a second container sharing the server's data volume, restarting the server container through the Docker API | Everything the image allows |
| **Rented from a game host** (no install allowed on the host) | Warden runs on the admin's own PC or a small box and reaches the server through the host's REST port (over the host's own secure path) and the files through **FTP/SFTP** | Dashboard, players, announcements, backups and settings through FTP; restarts only if the host offers them |

- A **first-run wizard** finds the server (install folder, SteamCMD, the `.ini`, the REST port and
  password, PalDefender if present), checks what it can do, and **shows which features are on and
  why any are off** instead of failing later.
- Nothing is Palsforge-specific: no fixed paths, addresses, schedules or names. Palsforge is just
  the first user (§5).
- The panel's text is ready for translation from the start (English first).

## 2. Features

### Version 0.1: the essentials
1. **Dashboard:** online/offline, version, players now, FPS and frame time, uptime, last save,
   last backup, next scheduled restart; history graphs (24 h / 7 days) stored locally.
2. **Players:** who is online (name, level, ping, platform), everyone seen before (first and last
   seen, total time), kick, ban and unban with a reason, announcements.
3. **The nightly routine** (configurable time): warn players in-game (15 / 5 / 1 minutes), save,
   stop, back up, update through SteamCMD, start; every step logged; if a step fails the server is
   still started again and the admin is told.
4. **Backups:** two kinds per run: **data** (saves, settings, mods such as PalDefender, logs) and
   **server image** (the whole server folder). Written as `.part` and renamed when complete;
   checksummed; retention of the newest 14 plus one per week for 8 weeks. **Copied to a second
   place** (a folder, a network share or SFTP) and checked against the checksum before old local
   copies are removed. Restore from the panel: stop, restore, start, with a clear preview first.
5. **Logins and the audit log:** an owner account created at first run; admin and moderator roles
   (moderators can kick and announce, not ban or change settings); every action recorded (who,
   what, when, result).

### Version 0.2
6. **Settings editor:** every `PalWorldSettings.ini` option with a plain explanation, its default,
   sensible limits, and a diff of what changes; applied at the next restart (or now, with a timed
   warning). The file's order and unknown keys are kept.
7. **Logs:** the server log, Warden's own log and PalDefender's, live and searchable.
8. **Notifications:** Discord webhook and ntfy: server down, backup failed, update installed,
   a player banned.
9. **PalDefender integration** (when installed): its allow-list and bans, and any richer player
   data its API offers (to be checked against its documentation).
10. **Map:** player positions (from the REST API), Pals when the game-data option is on. The
    official map artwork belongs to Pocketpair, so Warden draws its own simple grid and lets an
    admin add a map image they have the right to use.

### Mods (from 0.2, the most-wanted area after backups)
See §2b.

### Version 0.3 and later
11. **Tray icon** for the local window: the server's state at a glance, with a click to open.
12. **More than one server** in one panel.
13. **Linux packages and a Docker image.**

## 2b. Mod administration

How Palworld server mods work today (verified sources in research.md):

| Kind | Where it lives on the server | Notes |
|---|---|---|
| **PAK mods** | `Pal/Content/Paks/~mods` | Data and asset mods; work on Windows and Linux |
| **UE4SS** (mod loader) + **Lua mods** | `Pal/Binaries/Win64/ue4ss/Mods` | Windows build only; often breaks for a while after a game update |
| **PalSchema** (JSON data mods, needs UE4SS) | `.../ue4ss/Mods/PalSchema/mods/<mod>` | Most mods must also be on **every player's game at the same version** |
| **Server plugins** such as **PalDefender** | `Pal/Binaries/Win64` (DLLs, own config folder) | Anti-cheat and admin features; Windows |
| **Steam Workshop** (official, added with Palworld 1.0) | Through SteamCMD | Experimental and Windows-only so far; clients need the same list |

What Warden does with them:
1. **See what's installed:** scan every mod location; show each mod's name, kind, version, size,
   where it came from, and whether it's on or off. Spot common mistakes (a PalSchema mod in the
   wrong folder, a Lua mod without UE4SS, a Windows-only mod on Linux).
2. **Turn mods on and off** without deleting them (Warden moves them to its own "disabled" area).
3. **Install and update:** upload a mod's zip, give a download link, or a Workshop item ID;
   Warden unpacks it into the right place for its kind. Version pinning: a mod stays at the
   version you chose until you update it.
4. **Change safely:** every mod change takes a backup first, is applied at the next restart (or
   now, with an in-game warning), and Warden checks the server comes back healthy. If it doesn't,
   Warden **rolls the mods back automatically** and tells the admin which change failed.
5. **Game-update guard:** before the nightly game update, Warden can check whether the installed
   loaders (UE4SS, PalSchema) are known to work with the new build, and hold the update or turn
   the affected mods off, the admin's choice.
6. **Tell players what they need:** because many mods must match on players' games, Warden makes a
   shareable **mod list** (names, versions, download links) for the community's site or Discord,
   and shows it on the optional public status page.
7. **Mod sites:** download links and uploaded zips work for everything. Direct installs from mod
   sites (Nexus Mods, CurseForge, Thunderstore) depend on each site's API terms (Nexus, for example,
   limits direct downloads through its API); to be checked per site before building.

## 3. Safety by default

- Palworld's admin API stays on `127.0.0.1`; Warden is the only thing that talks to it and keeps
  its password on the server. Warden never offers to expose port 8212.
- The panel listens on **127.0.0.1 only**. Remote use is **through Tailscale only**: the setup
  wizard turns on `tailscale serve` for it (HTTPS with Tailscale's certificate, visible only to the
  admin's tailnet). Warden never opens a port on the network or the router.
- Logins: passwords hashed (Argon2id), optional two-factor codes (TOTP), sessions in HttpOnly
  cookies, a CSRF token on every change, rate-limited sign-in.
- Destructive actions (ban, restore, stop) need a second tap and are always in the audit log.
- Warden updates itself only from signed releases.
- No telemetry, no accounts with us, nothing sent anywhere the admin didn't set up.

## 4. How it's built (proposal)

| Part | Proposal | Why |
|---|---|---|
| Server program | **Go**, one file per platform (`warden.exe`, `warden`), runs as a Windows service or a systemd service | No runtime to install; small; easy for admins; good at long-running services and file work |
| Web panel | **Svelte** (built to static files embedded in the program) | Fast and small on phones; no Node.js needed on the server |
| Local data | **SQLite** (history, players seen, audit log, settings of Warden itself) | One file, easy to back up |
| Local window | The same panel in a native webview window opened by `warden.exe` (e.g. Go's webview bindings; Tauri only if needed) | One program, no browser needed on the server machine |
| Releases | GitHub Releases, signed; a download page on palsforge.com | Free and transparent |

Alternative considered: Python (faster for us to write, but admins would need Python installed or
a large bundled runtime) and Node.js (the RNZ01 panel's route; same runtime problem).

## 5. Palsforge as the first user

The Palsforge server already runs a hand-made version of the routine (PowerShell scripts: nightly
warnings, save, stop, data backup and server image, game update, copy to a second machine with
checksums, reboot, and a fallback). Warden turns that into a product anyone can use, and Palsforge
switches to Warden once version 0.1 does the same job.

## 6. Open questions

1. Confirm the stack (Go + Svelte + SQLite; a native window from the same program).
2. Allow-list: Palworld has none of its own; use PalDefender's when present, and the server
   password otherwise?
3. Which second places for backups in 0.1: folder and network share for sure; SFTP too?
4. Discord in 0.1 or 0.2?
5. Should the panel have an optional, read-only **public status page** (online, players, uptime,
   mod list) for a community website like palsforge.com?
6. Rented hosts: which ones to support first for FTP/SFTP and restart control?
7. Mod sites: which to integrate directly, after checking their API terms?
