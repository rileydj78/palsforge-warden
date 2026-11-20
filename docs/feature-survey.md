# Feature survey: Palworld server admin tools (draft)

**Date:** 2026-11-20. **Status:** draft for discussion, not a plan. Replaces nothing; it goes deeper
than [research.md](research.md), which missed most of the tools below.

## Method

- Started from [research.md](research.md), then searched GitHub (API search for "palworld admin",
  "palworld server manager", "palworld rcon", "palworld dashboard", "palworld panel"), the web
  (hosts, AMP, Pelican, PalDefender, Nexus), and the big projects' issue trackers for unmet needs.
- **Every claim comes from the tool's own README, docs site or feature page** (or, for hosts, their
  own knowledge base). Nothing was installed or run. Nexus Mods and Shockbyte pages returned 403,
  so those rows rest on search-engine summaries and are marked as such.
- "Last activity" is the GitHub `pushed_at` date (and latest release where one exists), read
  on 2026-11-20.
- Cell legend: **Y** = documented; **P** = partly (see note); **N** = not in the tool's docs or
  feature list (absence of evidence, not proof); **?** = couldn't verify (docs unclear or
  unreachable). "PD" = needs PalDefender.
- Warden's column comes from [README.md](../README.md), the Status table in [design.md](design.md),
  the panel (`internal/web/static`) and the commit log up to 2026-11-20 (mods and PalDefender
  shipped on 2026-11-20 even though the Status table still says "next").

## The tools

### Main columns (the matrix)

| Key | Tool | Form, licence | Last activity |
|---|---|---|---|
| **W** | Palsforge Warden | Go single binary, local window + web over Tailscale; Windows, Linux; MIT | 2026-11-20 (0.1 in testing) |
| **PDF** | PalDefender (was PalGuard), Ultimeit | Server plugin (anti-cheat + admin commands + REST on 17993); Windows servers only | v1.9.3, 2026-10-03 |
| **DCK** | palworld-server-docker, thijsvanloef | Docker image, env-var driven, no UI; MIT; 3.2k stars | 2026-11-16 |
| **PST** | palworld-server-tool, zaigie | Go web panel + save parser + optional agent; Apache-2.0; 1.2k stars | v0.12.2, 2026-07-14 |
| **PSG** | palserver GUI (Dalufish, now `palserver-gui/palserver-gui`) | Web GUI + agent, Windows/Linux, sponsor-only extras | v2.10.0 2026-09-03; pushed 2026-11-17 |
| **MYS** | MystTiq Palworld Server Manager, Wad3M | .NET desktop + headless service, Windows/Linux/Docker; MIT | v1.1.6.0 2026-10-30; pushed 2026-11-17 |
| **PBX** | Palbox Manager, JaredScar | Node web panel (Windows service) + Electron desktop; GPL-3.0 | v0.10.1, 2026-08-02 |
| **PSV** | PalSupervisor | Windows exe, web panel; licence keys (free/paid/trial); needs PD | 2026-08-31 |
| **PHM** | Palhelm, 8tp | Go, one Docker image, save parser; Apache-2.0; image not yet published | 2026-08-03 |
| **DHA** | palworld-dashboard, Dhampyru (fork of RNZ01) | Next.js, Docker + host integration; MIT | 2026-09-05 |
| **RNZ** | Palworld Server Dashboard, RNZ01 | Next.js, Docker; MIT | 2026-07-29 |
| **PSM** | Palworld Server Manager, amantu-qbit | Tauri desktop (Windows) + optional Bridge; MIT; no releases | 2026-07-22 |
| **UIT** | PalPanel, uitok | Go web panel, Chinese UI, Windows + Linux (Docker/Wine); GPL-3.0 | 2026-07-31 |
| **LUK** | Palworld A.D.M.I.N. (`palworld_admin` on PyPI; `Lukium/palworld-servertools` redirects here) | Python + Electron, Windows/Linux, Docker (Wine) | last release v0.10.4, 2024-04-15 |
| **PRK** | Palworld Server Manager, PrakashMandal-IV | Electron desktop, Windows/Linux; GPL-3.0 | 2026-08-31 |
| **TRR** | Palworld Server Manager, TRRabbit | Windows desktop app; free + paid "VIP" tier | 2026-08-29 |
| **AMP** | CubeCoders AMP (Palworld module) | Paid general game-server panel | ? (feature docs not reachable; forum only) |

### Other tools found (summarised after the matrix)

Pal Launcher Server Manager (Nothinx-44), palworld-server-manager (1tsmejp, Docker), PalDSMS
(AlanBacker), Palworld-Dedicated-Server-Manager (Andrew1175), palworld-server-toolkit (fol2),
palworld-server-manager (Rhyse76), PALWORLD-SERVER-MANAGER (sanjay-m6), PalOps (0xle0ne), Palisade
(Shakes63, multi-game), PalworldServerTools (shupershuff, PowerShell), Palworld-server-protector
(KirosHan), PalServerTools (HaitaoJin), palworld-chan (LunacyZeus), palworld-admin (limitcool),
paldmin (riffy, Webmin), palworld-ds-gui (diogomartino), palcon (josephdicarlo1), Palbot (dkoz /
palbot-gg), palhelm-bot, PalBot (Ly-sec), Takaro Palworld-Bridge (mad-001), BattleMetrics RCON,
AdminPal (Nodecraft), pal-conf (Bluefissure), Pelican/Pterodactyl eggs and the Pelican settings
plugin (JanPauw), Nexus desktop managers (mods 3604, 3491, 3576, 3786), and host panels (G-Portal,
Nitrado, ZAP-Hosting, Shockbyte, BisectHosting, Survival Servers, XGamingServer, Supercraft).

## Feature matrix

Columns: W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP

### A. Players and moderation

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Online players (level, ping, platform) | Y | Y | P¹ | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | ? |
| Player history (first/last seen, playtime) | Y | N | P² | P | Y | Y | Y | Y | Y | Y | N | N | Y | N | N | ? | ? |
| Kick (with message) | Y | Y | P¹ | Y | Y | Y | Y | Y | Y | Y | Y | Y | ? | Y | Y | Y | P³ |
| Ban / unban with reason | Y | Y | P¹ | Y | Y | Y | Y | Y | Y | Y | Y | Y | ? | P⁴ | Y | Y | P³ |
| Ban/unban offline players by ID | Y | Y | P¹ | ? | Y | ? | ? | Y | ? | ? | ? | Y | ? | Y | ? | ? | P³ |
| Temporary (timed) bans | N | ? | N | N | N | Y | N | N | N | N | N | N | N | N | N | N | N |
| IP ban | Y (PD) | Y | N | N | ? | ? | N | ? | N | N | N | N | N | N | N | N | N |
| Ban list view incl. bans made elsewhere (banlist.txt, PD, global list) | P⁵ | Y | N | N | ? | ? | ? | Y | N | ? | N | P⁶ | N | N | N | ? | N |
| Whitelist / allow-list | N | Y | N | Y | Y | Y | Y | Y | N⁷ | ? | N | N | ? | N⁸ | N | Y | ? |
| Maintenance lockdown (whitelist-only window) | N | N | N | N | N | N | Y | N | N | N | N | N | N | N | N | N | N |
| Announce to all | Y | Y | P¹ | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | P³ |
| Private message to one player | Y (PD) | Y | N | N | ? | ? | N | N | N | N | N | N | N | N | N | N | N |
| Player notes / tags | N | N | N | N | N | Y | Y | N | Y | N | N | N | N | N | N | N | N |
| Rename player | N | Y | N | N | N | ? | N | Y | N | N | N | N | N | N | N | N | N |
| Join gate (Steam sign-in / character check on join) | N | N | N | N | N | Y⁹ | N | N | N | N | N | N | N | Y¹⁰ | N | N | N |
| Anti-cheat detection / scans | N | Y | N | N | N | Y¹¹ | N | Y¹¹ | N | N | N | N | N | N | N | N | N |
| Character / player-ID migration, co-op host save fix | N | N | N | N | Y | Y | N | N | N | N | N | N | Y | N | N | N | N |

1. DCK: only through `rcon-cli` / REST from the shell; no panel. 2. DCK logs and announces joins and
leaves (`ENABLE_PLAYER_LOGGING`). 3. AMP: through its RCON console; forum users report RCON
dropping out. 4. LUK: ban works, unban "planned". 5. Warden tags bans made through Warden in the
roster; it doesn't read `banlist.txt` or PalDefender's ban list. 6. PSM keeps a local record of bans
made in the app. 7. Palhelm: "Player notes are not a whitelist"; deferred until a supported mechanism
exists. 8. LUK: whitelist and whitelist mode on the roadmap, not done. 9. MystTiq checks every join
and moves a character the game re-assigned to a new player ID back to its owner. 10. LUK "SteamAuth":
players sign in with Steam on a page first; with PalGuard it also pins their IP. 11. Level-gap
scans (levels gained vs time played); PSV adds kick/ban/reset actions on flags.

### B. World data and game-master tools

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Player inventory view | N | Y | N | Y | Y (PD) | Y | ? | Y | P¹² | Y | N | Y (Bridge) | Y | N | N | N | N |
| Player's Pals (party, Palbox, IVs, passives) | N | Y | N | Y | Y (PD) | Y | Y | Y | Y | Y | N | Y (Bridge) | Y | N | N | N | N |
| Guilds (members, leader) | P¹³ | Y | N | Y | ? | Y | Y | Y | Y | Y | N | Y (Bridge) | Y | N | N | N | N |
| Bases / base camps (workers, storage, health) | N | P¹⁴ | N | ? | P (sponsor) | Y | Y | Y | Y | ? | N | Y (Bridge) | Y | N | N | N | N |
| Server-wide search (who has item / Pal) | N | N | N | N | N | N | Y (Pal browser) | Y | Y (Pal explorer) | N | N | N | Y | N | N | N | N |
| Give items | N | Y | N | N | P (sponsor) | P (provider) | N | Y | P¹⁵ | P (save edit) | N | N | Y (PD) | N | N | N | N |
| Give Pals / eggs / Pal templates | N | Y | N | N | P (sponsor) | P (provider) | Y (PD) | Y | N | P (save edit) | N | P (clone via save) | Y (PD) | N | N | N | N |
| Kits / starter kits for new players | N | N | N | N | N | Y | N | Y | N | N | N | N | N | N | N | N | N |
| Grant XP / level / tech / stat points | N | Y | N | N | N | ? | N | Y | N | P (save edit) | N | P (save edit) | N | N | N | N | N |
| Teleport players | N | Y (command) | N | N | P (sponsor) | P (teleport points) | N | Y | N | N | N | N | N | N | N | N | N |
| Offline save editing (guarded) | N | N | N | N | N | Y | N | P | N | Y | N | Y (Bridge) | P (migration) | N | N | N | N |
| Inactive base / guild cleanup | N | Y (`/findbases`) | P (game settings) | N | N | P (repair workflows) | N | Y (destroy bases) | N | N | N | N | N | N | N | N | N |
| Breeding planner | N | N | N | N | P (sponsor) | N | N | N | N | N | N | N | Y | N | N | N | N |

12. Palhelm shows parties and Palboxes; full item inventory not documented (item grants via an
optional, disabled-by-default UE4SS bridge). 13. Warden shows each player's guild name when
PalDefender is connected. 14. PalDefender REST can delete a base; `/findbases` finds empty ones.
15. Palhelm: bounded, allow-listed item grants through a separate bridge, off by default.

### C. Map

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Live player positions | Y¹⁶ | P (data) | N | Y | Y | Y | Y | Y | Y | Y | Y | Y | Y | N | Y | Y | N |
| Bases / guild camps | N | P (data) | N | ? | Y | ? | Y | Y | Y | ? | N | Y (Bridge) | Y | N | N | ? | N |
| Pals / Alpha bosses | N | N | N | ? | Y (Alpha) | N | N | N | P (game-data) | N | N | Y | Y (save entities) | N | N | N | N |
| Landmarks (fast travel, dungeons, towers) | N | N | N | ? | Y | N | N | P (loot) | P (layers) | N | N | Y | N | N | N | N | N |
| Real game map artwork | N¹⁷ | – | – | Y | Y (bundled) | Y | Y | Y | P (fetch script) | Y | Y | Y | Y | – | Y | ? | – |
| Offline players' last position | N | N | N | N | Y | N | N | N | N | N | N | Y | N | N | N | N | N |
| Click map to pick coordinates (teleport/spawn) | N | N | N | N | Y | N | N | Y | N | N | N | N | N | N | N | N | N |

16. Warden plots players in the game's coordinates; exact positions from PalDefender when
connected; the transform still needs an in-game check. 17. Warden draws its own grid (the official
artwork belongs to Pocketpair); design lets an admin add an image they may use.

### D. Server operations

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Start / stop / restart from the panel | Y | N | P (container) | P (shutdown) | Y | Y | Y | Y | P (shutdown only) | Y | ? | Y (Bridge) | Y | Y | Y | Y | Y |
| Graceful shutdown with in-game countdown | Y | N | Y | Y | Y | ? | Y | Y | Y | ? | ? | Y | Y | Y | ? | ? | P³ |
| Crash auto-restart | Y | N | P (restart policy) | N | Y | Y | Y | Y | N | Y | N | N | ? | Y | Y | Y | ? |
| Frozen / hung server restart | P¹⁸ | N | N | N | N | Y | Y | N | N | N | N | N | N | N | N | Y | ? |
| Crash-loop limit | Y | N | N | N | Y (hourly cap) | ? | ? | ? | N | Y (hourly cap) | N | N | ? | ? | ? | ? | ? |
| Scheduled restarts (any interval / times) | P¹⁹ | N | Y (cron) | P (scheduled RCON) | Y | Y | Y | Y | N | Y | N | N | Y | N | Y | Y | Y |
| In-game restart warnings | Y (15/5/1 min) | N | Y | ? | Y | ? | Y | Y (custom text) | Y | ? | N | N | Y | ? | ? | ? | P³ |
| Memory-threshold restart | N | N | N | N | Y | N | N | N | N | Y | N | N | N | Y | N | N | ? |
| Hold reboot while players online | N | N | Y | N | P (backup skips empty) | N | N | N | N | N | N | N | N | N | N | N | N |
| Auto-pause when empty | N | N | Y | N | N | N | N | N | N | N | N | N | N | N | N | N | N |
| Game updates through SteamCMD | Y (nightly) | N | Y (cron / boot) | N | Y | Y | Y | ? | N | ? | N | N | Y | Y | Y | Y (scheduled) | Y |
| "Update available" check against Steam | ? | N | N | N | Y | Y | Y | ? | N | ? | N | N | Y | N | ? | ? | ? |
| Pin a game version | N | N | Y (manifest ID) | N | N | N | N | N | N | N | N | N | N | N | N | N | ? |
| Install the server from scratch | N²⁰ | N | Y | N | Y | Y | ? | N | N | P (compose) | N | N | Y | Y | Y | Y | Y |
| Adopt an existing install (auto-detect) | Y | – | – | – | Y | Y | ? | Y | – | – | – | – | Y | Y (import) | Y | ? | ? |
| Work alongside an existing watchdog / service | Y | – | – | – | N | N | N | N | P²¹ | N | N | N | N | N | N | N | N |
| Launch arguments editor | P²² | N | P (env) | N | Y | Y | N | Y | N | ? | N | N | Y | N | N | N | ? |
| Game metrics history (FPS, players) | Y (24 h / 7 d) | N | N | P | ? | Y | Y (30 d) | Y | Y | Y | Y | Y | P | N | N | N | ? |
| Host metrics (CPU, RAM, disk) | N | N | N | N | ? | Y (7 d) | Y | Y | N | N | N | N | Y | Y | N | N | Y |
| Threshold alert rules | P²³ | N | N | N | N | ? | Y | P (alert feed) | N | N | N | N | N | N | N | N | ? |
| Uptime % / outage timeline | P (events) | N | N | N | N | N | Y | N | N | N | N | N | N | N | N | N | ? |
| "Why it stopped" record | P (events, audit) | N | N | N | N | Y | N | N | N | N | N | N | N | N | N | N | N |
| Diagnostics / health page | P | N | N | N | Y (NAT/CGNAT) | Y | N | Y (API health) | Y | N | N | N | N | N | N | N | N |
| RCON / command console | N²⁴ | – | P (CLI) | Y (custom + scheduled) | Y (forms) | ? | Y (+ macros) | Y | Y (saved cmds) | Y | N | N (REST console) | ? | Y | N | Y (buttons) | Y |
| Live server console / log viewer | Y (IPs masked) | – | N | N | Y | Y | Y | Y | N | ? | N | N | Y | P (console exe) | Y | N | ? |
| Scheduled timed broadcasts / MOTD | N | N | N | Y (scheduled RCON) | Y (MOTD) | N | Y | N | N | N | N | N | N | N | N | N | Y³ |
| Scheduled events (e.g. double XP weekend, auto-revert) | N | N | N | N | N | N | Y | Y | N | N | N | N | N | N | N | Y | N |
| File manager | N | N | N | N | Y | N | N | N | N | N | N | N | N | N | N | N | Y |

18. Warden notifies "the server isn't answering" after 3 minutes but doesn't restart a hung
process. 19. One nightly routine at a chosen time (plus weekly image); no other schedules.
20. Warden adopts an existing install and updates it; it doesn't install one. 21. Palhelm says
restarts "belong to your host supervisor". 22. Warden stores the start command (supervise or
external mode); no flag editor. 23. Fixed alerts only: crash, crash loop, not answering, back up,
routine problems, game updated. 24. Deliberate: Warden is REST-only (RCON is deprecated).

### E. Backups and restore

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Manual backup | Y | N | Y (CLI) | Y | Y | Y | Y | Y | Y | Y | N | P (pre-edit) | Y | Y | Y | Y | Y |
| Scheduled backups | Y | N | Y (cron) | Y | Y | Y | Y | Y | Y | Y | N | N | Y | Y | Y | Y | Y |
| Includes settings + mods, not just the world | Y | N | N | N | ? | ? | ? | P (config restore) | N | ? | N | N | ? | N | N | ? | ? |
| Whole-server image backup | Y | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | ? |
| Retention policy | Y (14 daily + 8 weekly) | – | Y (days) | ? | Y (count) | Y | Y | Y | Y | ? | – | Y (20/file) | ? | Y (count) | ? | Y (rotation) | ? |
| Copy to a second place / off-machine | Y (folder, NAS) | – | N | N | N | N | N | N | N | N | – | N | Y (WebDAV) | N | N | N | Y (S3) |
| Checksums / verified copies | Y | – | N | N | N | Y | N | N | ? | N | – | P (re-parse) | Y (manifest/hash) | N | N | N | ? |
| Restore from the panel | Y | – | P (CLI) | ? | Y | Y | Y | Y | Y | Y | – | N | Y | Y | Y | Y | ? |
| Preview / diff before restore | Y | – | N | N | N | P (retention preview) | N | N | Y (dry-run diff) | N | – | N | N | N | N | N | N |
| Safety backup + rollback on failed restore | Y | – | N | N | P (pre-restore backup) | ? | N | N | P (pre-restore backup) | N | – | N | ? | N | N | N | N |
| Download a backup from the panel | N | – | N | ? | Y | ? | N | N | P (browse) | N | – | N | Y | N | N | N | ? |
| Backup before risky changes (update, mods, settings) | Y | – | P (before update) | N | Y (restart) | Y | Y (update) | Y (before restart) | N | N | – | Y (save edits) | Y | N | N | ? | N |
| Multiple worlds / switch active world | N | – | N | N | Y | ? | N | N | N | Y | – | N | Y | N | Y | ? | ? |

### F. Game settings

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| `PalWorldSettings.ini` editor | Y | N | P (env vars) | N²⁵ | Y (80+) | Y | Y (form + raw) | Y | Y (compose env) | Y | N | Y (Bridge) | Y | Y | Y (100+) | Y | ? |
| Explanations, ranges, defaults | Y | – | P (README table) | – | Y | ? | ? | ? | P (pending vs effective) | ? | – | P (grouped) | ? | N | ? | Y | ? |
| Review / diff before applying | Y | – | N | – | N | ? | N | N | Y | N | – | P (typed confirm) | N | N | N | N | ? |
| Presets (casual, PvP, performance) | N | – | N | – | Y | N | N | Y | N | Y | – | N | N | N | N | N | ? |
| Import / export settings | N | – | N | – | N | P (mod profiles) | N | Y (JSON) | N | N | – | P (JSON export) | N | N | Y (zip) | Y | ? |
| `Engine.ini` tuning | N | – | N | – | Y | N | N | N | N | Y | – | N | N | N | N | N | ? |
| `WorldOption.sav` handling | ? | – | N | – | N | N | N | N | N | N | – | N | N | Y | N | N | ? |
| Corrupt config detection / rebuild | ? | – | N | – | Y | P | N | P (config backup) | N | N | – | N | N | N | N | N | ? |
| Change some settings live, without restart | N | P (`/setting`, experimental) | N | – | N | N | N | N | N | N | – | N | N | N | N | N | N |

25. PST credits pal-conf (a config generator) but its feature list has no settings editor.

### G. Mods

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| List installed mods (PAK, UE4SS, PalSchema, PD) | Y | – | N | N | Y | Y | Y (UE4SS) | Y | N | Y | N | N | Y | N | Y | P | ? |
| Enable / disable without deleting | Y | – | N | N | P (Lua) | Y | Y | Y | N | ? | N | N | ? | N | Y | ? | ? |
| Install from an uploaded zip | Y | – | P (folder) | N | ? | Y | Y | Y | N | Y | N | N | ? | N | ? | ? | ? |
| Spot common mistakes / health checks | Y | – | N | N | N | Y | N | P (dependencies) | N | N | N | N | N | N | N | N | N |
| Changes queued for the next stop | Y | – | N | N | N | ? | N | N | N | N | N | N | N | N | N | N | N |
| One-click UE4SS / PalSchema / PalDefender install + update | N | – | Y (env) | N | Y (stable/beta) | P (switches) | N | N | N | Y (update detection) | N | N | Y | N | Y | Y | ? |
| Steam Workshop | N | – | Y (IDs) | N | N | Y | N | N | N | Y | N | N | Y | N | Y | N | ? |
| Browse Nexus / CurseForge / Thunderstore | N | – | N | N | N | Y (all + GitHub) | N | Y (Nexus) | N | Y (Nexus) | N | N | N | N | N | N | ? |
| Load order | N | – | N | N | N | Y | N | Y | N | N | N | N | N | N | N | N | N |
| Mod profiles (export / import a setup) | N | – | N | N | N | Y | N | Y | N | Y | N | N | N | N | Y (world profile) | Y | N |
| Per-mod config editor | N | – | – | N | Y (PD) | ? | N | Y | N | Y (PD) | N | N | Y (diff + backups) | N | N | N | N |
| Rollback / disable after a bad mod change | N (planned) | – | N | N | N | Y | N | N | N | N | N | N | N | N | N | N | N |
| Shareable client mod list for players | N (planned) | – | N | N | N | N | N | N | N | Y | N | N | N | N | N | N | N |

### H. Notifications, Discord and community

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Discord webhook alerts | Y | Y (PalWebhooks) | Y (per event) | N | ? | Y | Y | ? | N | N | N | N | N | N | Y | Y | ? |
| Other channels (ntfy, email, generic webhook) | Y (ntfy) | N | N²⁶ | N | N | Y (email, webhook) | N | N | N | N | N | N | N | N | N | N | ? |
| Delivery tracking for alerts | N | N | N | N | N | Y | N | N | N | N | N | N | N | N | N | N | N |
| Join / leave notifications | N | Y | Y | N | N | ? | Y | N | N | N | N | N | N | N | ? | ? | ? |
| Discord bot with commands | N | N | N | N | N | Y | N | Y (role-mapped) | Y (palhelm-bot) | N | N | N | P (QQ bot) | N | Y | Y | N |
| Chat log view | P²⁷ | Y (webhook) | N | N | N | ? | P (log viewer) | N | N | Y | N | N | N | N | Y | N | N |
| Chat relay Discord ↔ game | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N | N |
| Public read-only status page | N | N | N | P (visitor mode) | N | P (`/web`) | N | N | N | N | Y (`/view`, anonymise) | N | N | N | N | N | N |
| Player leaderboards / stats for the community | N | N | N | N | N | N | N | N | Y (bot) | N | N | P (top players) | N | N | N | N | N |

26. Open request in DCK for generic (e.g. ntfy) webhooks: issue #614. 27. Warden's log viewer
shows the server console, `Pal.log` and PalDefender logs; no dedicated chat view.

Chat relay exists only outside the matrix: Takaro bridge (UE4SS mod, both ways) and Palbot.

### I. Access, security and platform

| Feature | W | PDF | DCK | PST | PSG | MYS | PBX | PSV | PHM | DHA | RNZ | PSM | UIT | LUK | PRK | TRR | AMP |
|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|
| Named accounts | Y | – | – | P | ? | Y | Y | Y | P (2 passwords) | Y | P | N | ? | P (1 password) | N | ? | Y |
| Roles | Y (owner/admin/mod) | P (admin list) | – | P | ? | Y (4 + scoped) | Y (3) | Y (6, per action) | Y (admin/viewer) | Y (admin/mod) | Y (admin/mod) | N | ? | N | N | ? | ? |
| Two-factor (TOTP) | N (designed) | – | – | N | ? | ? | Y | N | N | N | N | N | N | N | N | N | ? |
| Audit log of admin actions | Y | P (logs) | – | N | ? | Y | Y | P (per-flag log) | P (grant ledger) | N | N | P (activity log) | Y | N | N | N | ? |
| Game admin password stays server-side | Y | – | – | ? | ? | Y (encrypted) | ? | ? | Y | Y | Y | N (desktop holds it) | ? | N | ? | ? | ? |
| Player IPs kept out of the browser | Y | – | – | ? | ? | ? | ? | ? | Y (redaction) | ? | P (public view) | ? | ? | ? | ? | ? | ? |
| Safe remote access built in | Y (Tailscale only) | – | – | N | P (Tailscale guide for players) | Y (pinned TLS) | N (open port 4000) | N | P (advice: VPN) | N | N | – | P (LAN rule) | N (port forward 8210) | – | – | ? |
| Local window on the server | Y | – | – | N | N | Y | Y | N | N | N | N | Y | N | Y | Y | Y | N |
| Mobile-friendly web page | P²⁸ | – | – | Y | ? | P (read-only) | ? | Y | ? | ? | ? | N | ? | ? | N | N | ? |
| Translations | N (ready) | – | – | ? | Y (4) | Y (12) | N | N | N | N | N | N | zh only | N | Y (4) | ? | ? |
| Windows server | Y | Y | Y (tested) | Y | Y | Y | Y | Y | P (Docker) | P (Docker) | P (Docker) | Y | Y | Y | Y | Y | Y |
| Linux server | Y | N | Y | Y | Y | Y | N | N | Y | Y | Y | N | Y | Y | Y | N | Y |
| Docker image | N (planned) | – | Y | Y | P (beta backend) | Y | N | N | Y | Y | Y | N | P (Wine) | Y | N | N | ? |
| More than one server | N (planned 0.3) | – | P (per container) | N | Y | Y | Y | N | N | Y (fleet) | N | P (per-server history) | ? | N | Y | Y | Y |
| API for other tools | N | Y | – | Y (Swagger) | N | N | P (REST proxy) | N | Y (OpenAPI + integration keys) | N | P | N | P (tokens) | N | N | N | ? |
| Self-update of the tool | N (deliberate) | – | Y (image) | N | Y (SHA-256 checked) | Y (checksums) | Y | N | N | N | N | Y | N | N | ? | ? | ? |
| Signed Windows releases | P (SignPath pending) | ? | – | ? | ? | P (SignPath pending) | N | N | – | – | – | N (unsigned) | ? | ? | ? | N | ? |
| Free, no paid tier | Y | Y | Y | Y | P (sponsor extras) | Y | Y | N (keys) | Y | Y | Y | Y | Y | Y | Y | P (VIP) | N |
| No telemetry | Y | P²⁹ | Y | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? | ? |

28. Warden's stylesheet has responsive rules and the design says it works in a phone browser; not
checked on a phone for this survey. 29. PalDefender sends a status heartbeat every 10 seconds (no
player names, IDs, IPs, chat or save data, by its own docs).

### Tools outside the matrix (documented features)

| Tool | Last activity | What stands out |
|---|---|---|
| Pal Launcher Server Manager (Nothinx-44, MIT) | 2026-07-27 | One-click install, watchdog, scheduled restarts with warnings, backups, map, kick/ban, history and playtime, roles (admin/user/read-only), activity log, PD commands, Discord alerts incl. joins, one-click UE4SS + PD, REST API, 4 languages |
| palworld-server-manager (1tsmejp, Docker) | 2026-09-07 | All 179 settings validated, **config-drift detection**, **review-and-deploy pending changes**, Workshop browser, world export/migration, provision servers |
| PalDSMS (AlanBacker, MIT, terminal) | 2026-07-14 | Verified backup before update, **rolls back the image tag if the server won't start**, validates and rolls back restores, `doctor` command, 3 languages |
| Palworld-Dedicated-Server-Manager (Andrew1175) | 2026-07-16 | Discord webhooks, auto-update of game and tool, **player session totals and "most active times" graph**, launch args, scheduled restarts with messages |
| palworld-server-toolkit (fol2, PowerShell) | 2026-02-23 | **Watchdog disables UE4SS after 2 fast crashes**, UE4SS updates via GitHub, git + robocopy + VSS backups, give/spawn editor |
| palworld-server-manager (Rhyse76) | 2026-10-04 | Multi-game (Palworld, ARK SA, Enshrouded), install/update, config editor, REST dashboard, pak mods, backups with pruning, crash watchdog |
| PALWORLD-SERVER-MANAGER (sanjay-m6, Tauri) | 2026-08-14 | Nexus and Modrinth mod install, difficulty presets, **one-click Windows Firewall rules**, restart schedules |
| PalOps (0xle0ne) | 2026-09-19 | **OpenID Connect sign-in**, restart tracking, bans, read-only settings |
| Palisade (Shakes63, MIT, 22 games) | 2026-11-20 | Docker panel, viewer/operator/admin roles with per-server access, SSO, verified DB backups; Palworld via the thijsvanloef image |
| PalworldServerTools (shupershuff, PowerShell) | 2026-07-09 | Ban offline players, shutdown reminders, player log, **daily settings switcher**, **grandfather-father-son retention**, whitelist (kick if not listed), Home Assistant |
| Palworld-server-protector (KirosHan, C#) | 2024-03-09 | Memory-threshold graceful restart, process guard, backups, WeCom webhook incl. joins |
| PalServerTools (HaitaoJin) | 2025-06-20 | Auto restart, backups/restore, settings, console, auto-update, multi-instance, `WorldOption.sav` |
| palworld-chan (LunacyZeus) | 2024-02-14 | Qiniu cloud backups, download backups, CPU/RAM/disk/network monitoring |
| palworld-ds-gui (diogomartino) | 2024-04-06 | Server + client (also web app), settings, admin, backups; early development |
| palcon (josephdicarlo1) | 2024-02-02 | Web RCON console only |
| Palbot (dkoz, GPL; palbot-gg docs) | 2025-04-01 | Discord: multi-server, RCON, join/leave logs, economy + shop (kits), PD integration, Steam API, localisation, **chat relay**, ticket system; palbot-gg adds live map and guild bases from PD |
| palhelm-bot (8tp) | 2026-07-15 | Public Discord commands: status, players, Pal box, map, leaderboards, breeding paths, "who has" |
| Takaro Palworld-Bridge (mad-001) | 2026-10-08 | Takaro integration; chat relay and player locations through a UE4SS mod; item catalogue for shops |
| BattleMetrics RCON | ? | Paid ($10/month): player tracking, Steam ID logs, bans; whether Ban Sync covers Palworld is unconfirmed |
| AdminPal (Nodecraft) | ? | Free RCON web tool: players + Steam IDs, join/leave, broadcast |
| Pelican / Pterodactyl eggs (pelican-eggs) | ? | Native and Proton eggs; config parser for settings; mods only on Proton; restart every ~6 h advised |
| Pelican settings plugin (JanPauw) | 2026-07-10 | Typed, grouped settings with search, presets, writes blocked while running, config backups, reset to defaults, keeps unknown keys |
| pal-conf (Bluefissure) | 2026-07-11 | Web generator for `PalWorldSettings.ini` and `WorldOption.sav` |
| Nexus managers (mods 3604, 3491, 3576, 3786) | ? | Not verified (Nexus 403). Per search summaries: 3604 has pak enable/disable by rename, RCON tools, zip backups; 3491 adds Discord alerts |

### Host panels (Palworld-specific docs only)

| Host | Documented Palworld panel features | Unknown from docs |
|---|---|---|
| G-Portal | Backups tab: manual backup, restore (stop first); game auto-backup interval setting | Player admin, mods, retention |
| Nitrado | Web-interface settings, "Restore Backup" roll-back, config profiles | Player admin |
| ZAP-Hosting | Settings forms with explanations and "Caution" flags, raw view, read-only console, backups (plan-dependent), cron schedules for restart/backup, 1 h–30 d stats, activity log, file manager, AI agent | Player admin, mods, RCON |
| Shockbyte | Modding tab with Nexus Mods (needs "Palworld (modded)" server type) (search summary; site 403) | Rest |
| BisectHosting | Starbase panel start/stop, "3 days of free backups", "full mod support", "player management tools" (marketing) | What the player tools are |
| Survival Servers | Settings form, supported mod install, scheduled restarts, automatic backups every 30 min | Player admin |
| XGamingServer | Dashboard with CPU/RAM/disk, console, files, settings; docs for RCON/REST/admin commands | Schedules, whitelist |
| Supercraft | Web RCON console, **character recovery**, allow/block list management, Thunderstore mods | – |
| CubeCoders AMP | Scheduler triggers, backups incl. S3, RCON console; several forum reports of Palworld backup and RCON failures | Roles/audit details (KB not readable) |

## Where the gaps stand (updated 2026-11-20, 0.1.0-rc19)

| # | Gap | Status |
|---|---|---|
| 1 | Allow-list / maintenance lockdown | **Done** (Warden's own list, kick on join; lockdown = turn it on) |
| 2 | Frozen-server restart, RAM threshold | **Done** (capped at 3 automatic restarts per 6 h) |
| 3 | Inventory, Pals, guilds, bases | **Done through PalDefender**; save parser for servers without it: not started |
| 4 | Give items / Pals | **Done through PalDefender**; kits and starter kits: not started |
| 6 | Richer map | **Partly**: bases; Alpha Pals, landmarks, offline positions not started |
| 7 | Discord | **Partly**: join/leave alerts, live status message; command bot and chat relay not started |
| 8 | Character migration / host-save fix | Not started |
| 9 | More than one server | Not started (0.3 plan) |
| 10 | Translations | Not started |
| 11 | Two-factor sign-in | **Done** (TOTP, escalating lock, codes work once) |
| 12 | Public status page | **Partly**: the Discord live status; a web page not started |
| 13 | Install the server from scratch | Not started |
| 14 | Host metrics | **Partly**: CPU and RAM in history; disk and network not started |
| 15 | Settings extras | **Partly**: presets, import/export; Engine.ini and launch arguments not started |
| 16 | Scheduled events | **Done** (weekly events, admin edits kept, survive restarts) |
| 17 | Timed broadcasts / MOTD | **Done** (rotating messages, private welcome) |
| 18 | Ban list from every source | **Done** (game list, PalDefender incl. IP bans) |
| 19 | Anti-cheat review | Not started |
| 20 | S3 / WebDAV / SFTP targets | Not started |
| 21 | Download a backup | **Done** |
| 22–25, 27–30 | Version hold, Docker, API keys, auto-pause, worlds, self-update, console, files | Not started |
| 26 | Player notes | **Done** |

Not release-ready until the "Not started" items that matter most to owners (8, 9, 10, 13, 22,
23) are decided: built, or consciously left out of 1.0 and said so.

## Parity gaps (others have it, Warden doesn't)

Ordered roughly by how much they matter to a server owner. Effort: **S** = days, **M** = a week or
two, **L** = more.

1. **Allow-list / whitelist, plus a whitelist-only maintenance mode.** PDF, PST, PSG, MYS, PBX, PSV,
   TRR, shupershuff (PBX: lockdown). Palworld has none of its own: use PalDefender's
   (`/whitelist_*`, `WhiteList.json`) when present, else kick-on-join from the REST player list.
   **M**.
2. **Frozen-server restart and more restart triggers** (hung but not crashed; any interval; RAM
   threshold; hold while players are online). MYS, PBX, TRR (freeze); LUK, PSG, DHA (RAM); DCK (hold
   while online); many (schedules). Warden already detects "not answering" — it only needs to act.
   **S**.
3. **Player inventory, Pals, guild and base inspection.** PST, PSG, MYS, PBX, PSV, PHM, DHA, PSM, UIT.
   Cheapest path: PalDefender REST (`/items`, `/pals`, `/guilds`, `/progression`) which Warden
   already talks to (**M**); a save parser for servers without PD is **L** (Oodle-compressed 1.0
   saves, format drift after updates).
4. **Game-master actions: give items / Pals / eggs / XP / tech, kits, starter kits.** PDF, PSV, UIT,
   PBX, MYS, PSG (sponsor). Through PalDefender's REST `give/*` endpoints with role checks and
   audit. **M**.
5. **Mod sources and loaders: one-click UE4SS / PalSchema / PalDefender install and update,
   Workshop IDs, Nexus/Thunderstore/CurseForge, load order, profiles, per-mod config.** MYS, PSV,
   DHA, PSG, UIT, PRK, TRR, DCK. Already in Warden's design (§2b). **M** for loaders + Workshop +
   profiles; **L** for site browsers (API terms first).
6. **Richer map:** bases, guild areas, Alpha Pals, landmarks, offline last positions, click-to-pick
   coordinates. PSG, PBX, PSV, PHM, PSM, UIT. Data from PD or saves; artwork stays the licensing
   question Warden already noted. **M**.
7. **Discord beyond webhooks:** join/leave alerts (DCK, PBX, PDF — **S**), a command bot (MYS, PSV,
   PRK, TRR, palhelm-bot — **M**), chat view / relay (DHA, PRK, PDF webhooks, Takaro — **M**).
8. **Character / player-ID migration and the co-op host save fix.** MYS (automatic on join), UIT
   (wizard with rollback), PSG (built-in host-save fix), Supercraft (character recovery). A common,
   painful support problem since 1.0. **L**.
9. **More than one server.** PBX, MYS, PSG, DHA, PRK, TRR, AMP. In Warden's 0.3 plan. **L**.
10. **Translations.** MYS (12), PSG (4), PRK (4), Nothinx (4), PalDSMS (3). Warden's text is
    "ready for translation"; the work is the catalogue and reviewers. **M**.
11. **Two-factor sign-in.** PBX. In Warden's design. **S**.
12. **Public read-only status page.** RNZ (with name anonymising), MYS, PST. Open question 5 in the
    design. **S**.
13. **Install the server from scratch** (SteamCMD/DepotDownloader wizard). PSG, MYS, LUK, UIT, PRK,
    TRR, DCK, AMP. **M**.
14. **Host metrics** (CPU, RAM, disk, network history) next to FPS. MYS, PBX, PSV, UIT, LUK. **S**.
15. **Settings extras:** presets (PSG, DHA, PSV, JanPauw, sanjay-m6), import/export (PSV, PRK, TRR),
    `Engine.ini` tuning (PSG, DHA), launch-argument editor (PSG, MYS, UIT, Andrew1175). **S** each.
16. **Scheduled events** (double-XP weekend applied and reverted automatically; daily settings
    switch). PBX, PSV, TRR, shupershuff. Fits the routine engine. **M**.
17. **Timed broadcasts / MOTD.** PBX, PSG, PST. **S**.
18. **Ban list from every source** (game `banlist.txt`, PalDefender `/banlist`, IP bans, temporary
    bans). PDF, PSV, MYS (temporary). Warden only knows its own bans. **S–M**.
19. **Anti-cheat review** (level-gap scans, flags to review). MYS, PSV, PDF. **M**.
20. **Off-machine targets beyond folder/NAS:** S3 (AMP), WebDAV (UIT), cloud (palworld-chan); SFTP is
    already in Warden's open questions. **M**.
21. **Download a backup** from the panel. PSG, UIT, palworld-chan. **S**.
22. **Game version pinning / hold an update.** DCK. Pairs with the planned mod update guard. **S–M**.
23. **Docker image / sidecar.** DCK-based panels (PHM, DHA, RNZ, PST, MYS). In Warden's plan. **M**.
24. **API / integration keys for bots and sites.** PHM, PST, Nothinx, PDF. **M**.
25. **Auto-pause when empty** (power saving). DCK only. **M** (needs knock-to-wake).
26. **Player notes and tags.** PBX, MYS, PHM. **S**.
27. **Multiple worlds / switch the active world.** PSG, DHA, UIT, PRK. **M**.
28. **Self-update** of the tool (checksummed). PSG, MYS, PBX, PSM. Warden deliberately has no update
    check; could be opt-in and signed-only, as design §3 already allows. **M**.
29. **RCON-style command console.** Most panels. Warden is REST-only on purpose; a "commands" page of
    REST + PD actions with forms (PSG's approach) would cover the need without RCON. **S**.
30. **File manager.** PSG, AMP, hosts. Low priority for a tool that runs on the box. **M**.

## Only Warden (not found in any other surveyed tool's docs)

- **Remote access built around Tailscale only** (`warden remote on`), panel bound to 127.0.0.1;
  others open a port (PBX 4000, LUK 8210), leave it to the admin (PHM), or use their own TLS (MYS).
- **Works alongside an existing watchdog or service** ("another program runs it", with a stop-file):
  every other tool either owns the process or can't start it.
- **Whole-server image backups** in addition to data backups.
- **Backups copied to a second place and checksum-verified there before local copies are pruned.**
  (UIT verifies and uploads to WebDAV; MYS verifies locally; nobody documents verify-at-destination
  before pruning.)
- **Restore that is previewed, makes a safety backup, keeps old folders aside, and rolls back on
  failure**, all in a panel (PalDSMS does the rollback in a terminal script; PHM has preview +
  pre-restore backup but no rollback).
- **A nightly routine that always starts the server again whatever step fails**, with a run report.
- **Mod changes queued for the next stop** (1tsmejp stages settings and Workshop mods for review,
  but doesn't document applying them at the next stop).
- **ntfy push** (DCK has an open request for it, #614).
- **No telemetry and no network calls at all unless configured** (PalDefender heartbeats; most
  tools don't say).
- **One small binary, Windows and Linux, no runtime, no Docker** (PST and MYS come close: PST ships
  binaries but needs 1–3 GB RAM for save parsing; MYS needs its service + desktop pair).

## Nobody has this yet (opportunities)

Needs seen in issues, forums and tool docs that no surveyed tool covers, or only partly.

1. **Crash-aware backup retention.** A server crash followed by routine backups can push the last
   good pre-crash backups out of retention (DCK issue #643 describes exactly this). Pin the last
   backup taken before any crash or failed restart until an admin clears it. Nobody does this.
2. **Save-stall and save-damage watchdog.** Reports of the server silently stopping writing saves
   after idle (DCK #699), player files overwritten on restart (DCK #457), and atomic-save failures
   disconnecting everyone (AMP forum). Alert when `Level.sav` stops changing while players are
   online, or a player file shrinks or loses levels across a restart, and keep the pre-restart copy.
   Not found anywhere (PHM's diagnostics show save-parse freshness, not this).
3. **Proven restores.** Periodically restore the latest off-machine backup into a scratch folder,
   check it parses (or at least that every file matches and the world folder is complete), and show
   "restorable as of …". MYS mentions "restore checks" without detail; nobody documents a restore
   drill.
4. **Exposure self-check.** Warn when the REST port (8212), RCON (25575) or PalDefender REST
   (17993) listens beyond 127.0.0.1, or the admin password is weak or empty (DCK #886 shows
   "Unauthorized (AdminPassword is empty)" confusion). Only LUK enforces an 8-character minimum.
5. **Game-update guard for mods.** Hold the nightly update, or turn affected mods off, when UE4SS /
   PalSchema aren't known to work with the new build, and roll back if the server won't come up.
   Partial pieces exist (fol2 disables UE4SS after crashes; DHA detects framework updates; DCK pins
   versions; PalDSMS rolls back the image tag) but nobody joins them. Already in Warden's design.
6. **Rented-host mode.** Manage a G-Portal/Nitrado-style server from the admin's own PC through
   the host's REST port plus FTP/SFTP for backups and settings. Every self-hosted tool needs to run
   on, or install an agent on, the server. Already in Warden's design (§1b).
7. **Per-player rollback.** Restore one player's character (and their Pals) from a backup without
   rolling back the whole world. Only a host panel (Supercraft "character recovery") and the
   migration tools come close.
8. **One honest ban list** across the game's `banlist.txt`, Pocketpair's global `BanListURL`, and
   PalDefender (incl. IP and temporary bans), with who/when/why, and export/import to share with
   friendly servers. Paid BattleMetrics covers only its own bans.
9. **"Which base is making the server lag?"** Correlate FPS drops with players, base count and
   base-worker counts to name heavy bases. PHM's game-data layer and PDF's base data have the raw
   material; nobody turns it into an answer.
10. **A players' "how to join" page**: connect info, rules, and the exact client mod list with
    versions and links, generated from what's installed (DHA has shareable client bundles; RNZ has
    a public status view; nobody combines them).
11. **Crash culprit hints.** After a crash, read the crash log/dump and name the likely mod (UE4SS
    Lua error, PalSchema load failure) instead of only restarting. fol2's crash counter is the
    nearest thing.
12. **One maintenance window for all pending changes** (settings + mods + update), applied together
    after warnings, health-checked, and rolled back as a unit. 1tsmejp stages changes; nobody rolls
    them back together.

## Sources

Warden
- [README.md](../README.md), [design.md](design.md) (Status table), `internal/web/static/`, git log
  (2026-11-19 to 2026-11-20).

Matrix tools
- PalDefender: [home](https://ultimeit.github.io/PalDefender/), [Commands](https://ultimeit.github.io/PalDefender/Commands), [Features](https://ultimeit.github.io/PalDefender/Features), [REST API](https://ultimeit.github.io/PalDefender/RESTAPI), [GitHub](https://github.com/Ultimeit/PalDefender)
- palworld-server-docker: [README](https://github.com/thijsvanloef/palworld-server-docker), issues [#614](https://github.com/thijsvanloef/palworld-server-docker/issues/614), [#643](https://github.com/thijsvanloef/palworld-server-docker/issues/643), [#699](https://github.com/thijsvanloef/palworld-server-docker/issues/699), [#457](https://github.com/thijsvanloef/palworld-server-docker/issues/457), [#886](https://github.com/thijsvanloef/palworld-server-docker/issues/886), [#885](https://github.com/thijsvanloef/palworld-server-docker/issues/885)
- palworld-server-tool (zaigie): [README](https://github.com/zaigie/palworld-server-tool), [English README](https://github.com/zaigie/palworld-server-tool/blob/main/docs/README.en.md)
- palserver GUI: [GitHub](https://github.com/palserver-gui/palserver-gui), [docs](https://dalufishes-team.gitbook.io/palserver-gui-en)
- MystTiq: [GitHub](https://github.com/Wad3M/MystTiq-Palworld-Server-Manager)
- Palbox Manager: [GitHub](https://github.com/JaredScar/Palbox-Manager)
- PalSupervisor: [GitHub](https://github.com/PalSupervisor/PalSupervisor), [site](https://palsupervisor.github.io/PalSupervisor/)
- Palhelm: [GitHub](https://github.com/8tp/palhelm), [docs](https://docs.palhelm.com), [palhelm-bot](https://github.com/8tp/palhelm-bot)
- Dhampyru palworld-dashboard: [GitHub](https://github.com/Dhampyru/palworld-dashboard)
- RNZ01 Palworld Server Dashboard: [GitHub](https://github.com/RNZ01/palworld-server-dashboard)
- amantu-qbit Palworld Server Manager: [GitHub](https://github.com/amantu-qbit/palworld-server-manager)
- uitok PalPanel: [GitHub](https://github.com/uitok/palworld-panel)
- Palworld A.D.M.I.N. (Lukium): [GitHub](https://github.com/Lukium/palworld-admin), [PyPI](https://pypi.org/project/palworld_admin)
- PrakashMandal-IV: [GitHub](https://github.com/PrakashMandal-IV/palworld-server-manager)
- TRRabbit: [GitHub](https://github.com/TRRabbit/palworld-server-manager)
- CubeCoders AMP: [S3 backup guide](https://discourse.cubecoders.com/t/guide-to-setup-a-free-offsite-backblaze-s3-backup-for-amp-instances/40410), [Palworld RCON/password thread](https://discourse.cubecoders.com/t/configuration-with-palworld-palworld-rcon-admin-password-issues/9484), [backup task failing](https://discourse.cubecoders.com/t/palworld-the-backup-task-is-failing-it-cannot-find-a-file/15003), [save lost after backup](https://discourse.cubecoders.com/t/palworld-save-command-completes-but-progress-is-lost-after-shutdown-or-backup/28041), [atomic save failure](https://discourse.cubecoders.com/t/palworld-dedicated-server-disconnects-all-players-after-atomic-save-failure-failed-to-save-failed-copy-from-backup/42094)

Other tools
- [Nothinx-44 Pal Launcher](https://github.com/Nothinx-44/palworld-launcher-server-manager) · [1tsmejp manager](https://github.com/1tsmejp/palworld-server-manager) · [PalDSMS](https://github.com/AlanBacker/PalWorld-Docker-Server-Management-System-Script) · [Andrew1175](https://github.com/Andrew1175/Palworld-Dedicated-Server-Manager) · [fol2 toolkit](https://github.com/fol2/palworld-server-toolkit) · [Rhyse76](https://github.com/Rhyse76/palworld-server-manager) · [sanjay-m6](https://github.com/sanjay-m6/PALWORLD-SERVER-MANAGER) · [PalOps](https://github.com/0xle0ne/palops) · [Palisade](https://github.com/Shakes63/palisade) · [PalworldServerTools](https://github.com/shupershuff/PalworldServerTools) · [KirosHan protector](https://github.com/KirosHan/Palworld-server-protector-DotNet) · [PalServerTools](https://github.com/HaitaoJin/PalServerTools) · [palworld-chan](https://github.com/LunacyZeus/palworld-chan) · [limitcool palworld-admin](https://github.com/limitcool/palworld-admin) · [paldmin](https://github.com/riffy/paldmin) · [palworld-ds-gui](https://github.com/diogomartino/palworld-ds-gui) · [palcon](https://github.com/josephdicarlo1/palcon) · [Palbot (dkoz)](https://github.com/dkoz/palworld-palbot) · [Palbot docs](https://palbot-gg.github.io/docs) · [Ly-sec PalBot](https://github.com/Ly-sec/PalBot) · [Takaro bridge](https://github.com/mad-001/Palworld-Bridge) · [pal-conf](https://github.com/Bluefissure/pal-conf) · [Pelican eggs](https://github.com/pelican-eggs/games-steamcmd/tree/main/palworld) · [Pelican settings plugin](https://github.com/JanPauw/palworld-settings-editor)
- [BattleMetrics Palworld RCON](https://www.battlemetrics.com/rcon/palworld) (403 to fetch; via search), [BattleMetrics Ban Sync](https://learn.battlemetrics.com/article/12-ban-sync)
- [AdminPal (Nodecraft)](https://nodecraft.com/support/games/palworld/setup/monitor-your-palworld-server-with-adminpal)
- Nexus (403 to fetch; via search summaries): [3604](https://www.nexusmods.com/palworld/mods/3604), [3491](https://www.nexusmods.com/palworld/mods/3491), [3576](https://www.nexusmods.com/palworld/mods/3576), [3786](https://www.nexusmods.com/palworld/mods/3786)

Hosts
- [G-Portal backups](https://www.g-portal.com/wiki/en/palworld-backups-create-restore/) · [Nitrado panel guide](https://server.nitrado.net/en-US/guides/navigating-nitrado-server-panel) · [Nitrado Palworld settings](https://server.nitrado.net/en-GB/guides/palworld-server-settings-guide) · [ZAP-Hosting dashboard](https://zap-hosting.com/en/help/articles/palworld-dashboard-overview) · [Shockbyte Palworld KB](https://shockbyte.com/help/knowledgebase/categories/palworld) (403; via search) · [BisectHosting Palworld](https://www.bisecthosting.com/palworld-server-hosting) · [Survival Servers Palworld](https://www.survivalservers.com/r/Murga/palworld/), [new panel](https://www.survivalservers.com/news/new-control-panel/) · [XGamingServer docs](https://xgamingserver.com/docs/palworld) · [Supercraft character migration + panel](https://wiki.supercraft.host/wiki/palworld/official_character_migration_v2/)

Game facts
- [Official server docs](https://docs.palworldgame.com/) · No native whitelist: [Apex guide](https://apexminecrafthosting.com/guides/palworld/palworld-server-moderation-whitelist-ban-and-kick-guide/) · Ban list file and `BanListURL`: [Nodecraft](https://nodecraft.com/support/games/palworld/general/banning-and-unbanning-players-on-your-palworld-server), [wiki.gg](https://palworld.wiki.gg/wiki/PalWorldSettings.ini)
