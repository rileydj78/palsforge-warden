# Research: existing Palworld admin tools (November 2026)

What the current free tools do, where they stop, and what that means for Palsforge Warden.
Sources are at the end; check each project's own page for its latest state.

## The ground truth: what the game offers

- **Palworld REST API** (port 8212, off by default): `RESTAPIEnabled=True` and an `AdminPassword`
  in `PalWorldSettings.ini`. HTTP Basic auth, user `admin`. Base `http://<server>:8212/v1/api`.
  - Read: `info`, `players`, `settings`, `metrics` (and `game-data` when enabled with a launch
    flag, which adds Pal positions).
  - Write: `announce`, `kick`, `ban`, `unban`, `save`, `shutdown` (with a countdown), `stop`.
  - It grants full control of the server and should never be exposed to the internet.
  - Settings are **read-only** over the API: changing them means editing the `.ini` and restarting.
- **RCON** is deprecated in favour of the REST API; guides disagree on exactly when it stops
  working, so Warden should not depend on it.
- **PalDefender** (anti-cheat and admin plugin for Windows servers) has its own local REST API
  (port 17993, bearer tokens from its `Tokens` folder). Its endpoint list needs checking on its
  documentation site before we rely on it; it is an optional extra for Warden.
- **Save files** hold players, Pals, guilds and bases. Reading them needs a save parser; editing
  them is only safe while the server is stopped.

## The tools

| Tool | Form | Strengths | Gaps |
|---|---|---|---|
| **Palworld Server Manager** (amantu-qbit) | Windows desktop app (Tauri + React), MIT | Live dashboard and trends, world map with players and Pals, roster with kick/ban, console (announce, save, timed shutdown), settings browser, ban manager. An optional **Bridge** on the server host edits saves and the `.ini`, with backups before each edit | Windows only; two pieces to install for anything beyond monitoring; save edits need the server stopped; unsigned installers; very early (few users, no releases yet) |
| **Palworld Server Dashboard** (RNZ01) | Self-hosted web panel (Next.js, Docker), MIT | Monitoring with FPS history, roster with kick/ban/unban, announcements, live map, **admin and moderator logins**, keeps the game's admin password server-side, optional read-only public view | No backups, no settings editing, no multi-server; needs Node.js or Docker |
| **Palworld Admin** (PyPI `palworld_admin`) | GUI, Python | Deploys the server, configuration, monitoring, **backups**, RCON kick/ban | RCON-based (deprecated); allow-list still on the roadmap |
| **palworld-servertools** (Lukium) | Desktop tools | Automatic and manual backups, RAM monitoring, RCON kick/ban | RCON-based; allow-list unfinished |
| **palworld-ds-gui** (diogomartino) | TypeScript GUI, also as a web app | Manages local or remote servers | Small project; backups and moderation unclear |
| **palserver GUI** (Dalufish) | Desktop app | Friendly editor for world and server settings | Little on players or backups |
| **palcon** | Simple web UI | Lightweight RCON console | RCON only, minimal |
| **palworld-server-tool** (zaigie) | Web + REST, save reader | Reads players, guilds and inventories from saves | Inspection, not administration |
| **Palbot** | Discord bot and companion | Admin commands from Discord | Discord-centred |
| Microsoft Store panels | Paid desktop apps | Polished | Not free |

## What nobody does well (Warden's opening)

1. **One install that does everything.** The good tools split into a pretty panel *plus* a
   bridge, or a script pile. Warden runs on the server host, so it can see the files, the process
   and the API at once.
2. **Backups you can trust.** Complete (saves, settings, mods, logs, the server itself), on a
   schedule tied to the nightly restart, **copied off the machine and checked**, with retention.
   Most tools stop at "zip the save folder locally".
3. **The nightly routine.** Warn players, save, stop, back up, update, verify the copy, restart
   (or reboot), with a fallback if any step fails. Today admins write this themselves.
4. **Settings with explanations.** Every option described in plain words, with sane ranges and a
   preview of what changes on the next restart.
5. **Phone-first.** Admins check on their server from the couch or the road; the panel should be
   comfortable on a phone.
6. **An audit log** of who did what, which the multi-admin panels mostly lack.
7. **Windows and Linux** from day one, without Node.js or Docker as a requirement.

## Sources

- [Palworld REST API reference (XGamingServer)](https://xgamingserver.com/docs/palworld/rest-api)
- [The Palworld REST API: every endpoint, and why not to expose it (gamever)](https://gamever.io/knowledge-base/the-palworld-rest-api-every-endpoint-and-why-not-to-expose-it)
- [Palworld REST API and server administration (GuildOrder)](https://guildorder.com/games/palworld/wiki/rest-api-administration)
- [How to enable the Palworld REST API (G-Portal)](https://www.g-portal.com/wiki/en/how-to-enable-the-palworld-rest-api-gportal/)
- [Palworld server admin tools 2026 (Supercraft)](https://supercraft.host/wiki/palworld/official_character_migration_v2/)
- [PalDefender REST API docs](https://ultimeit.github.io/PalDefender/RESTAPI)
- [Palworld Server Manager (amantu-qbit)](https://github.com/amantu-qbit/palworld-server-manager)
- [Palworld Server Dashboard (RNZ01)](https://github.com/RNZ01/palworld-server-dashboard)
- [palworld_admin on PyPI](https://pypi.org/project/palworld_admin)
- [palworld-ds-gui](https://www.Github.com/diogomartino/palworld-ds-gui)
- [palcon](https://www.github.com/josephdicarlo1/palcon)
- [Palbot docs](https://palbot-gg.github.io/docs)
