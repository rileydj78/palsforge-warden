# Palworld dedicated-server owners: needs and market (research for Palsforge Warden)


> **What 1.0 did with this (2026-11-20):** built the connection check (pain points 1 and 7), the
> WorldOption.sav check (6) and "update soon" (3); explained the game's own backups. Also built: the co-op
> world upload (9, without the host-character fix, which needs save editing), crash hints, and a
> settings-file check. The Players page can show who
> hasn't played for 30+ days; removing abandoned bases (PalDefender's deletebase) waits until
> its guild data can be checked on a real server. Left for later: clean-up of
> inactive players and bases, and crash explanations. Reddit couldn't be reached by the research
> tools, so its threads aren't counted here.

Researched 2026-11-20. Read-only web research. Warden status is judged against
[CHANGELOG.md](../CHANGELOG.md) (1.0.0-rc.1) and the "Where the gaps stand" / "Nobody has this
yet" sections of `docs/feature-survey.md`; this report tries not to repeat those.

## Method and limits (read first)

- **Reddit could not be reached.** reddit.com and old.reddit.com refuse this tool's fetcher, and the
  search tool rejects `reddit.com` as a domain ("not accessible to our user agent"). Ordinary searches
  for `site:reddit.com …` returned no Reddit results. **So r/Palworld, r/admincraft, r/selfhosted and
  r/homelab are not in this report.** The gap is filled with other sources:
  - **CubeCoders AMP support forum** (discourse.cubecoders.com). Its search API lists 341 Palworld
    topics from 2025-05-30 to 2026-11-14, and **128 of them were opened after 1.0 (2026-07-10)**.
    AMP files topics by type ("Connectivity Problem with Palworld", "Update Failure with
    Palworld", …), so this is the best *counted* sample of real owner problems I found. I sorted the
    128 into themes myself, using keyword rules plus a manual check. The counts are approximate.
  - **Steam Community search hit counts** for the Palworld forum (app 1623730). Steam's search
    reports "N entries" for a query. These are rough relative signals (keyword matches across all
    years, including player chatter), **not** thread counts. The Palworld Dedicated Server app
    (2394010) has no usable discussion board; its page showed only general game threads.
  - **GitHub issue trackers**, sorted by comments and reactions via the GitHub API:
    thijsvanloef/palworld-server-docker (DCK), zaigie/palworld-server-tool (PST),
    Ultimeit/PalDefender (PD), plus pocketpairjp/palworld-dedicated-server-docker.
  - **Host knowledge bases.** The topics hosts write help articles about show what customers ask.
    Nodecraft has 58 Palworld articles (all slugs listed). I also used LOW.MS, NameHero, G-Portal,
    Shockbyte (via search snippets; direct fetch returned 403), xgamingserver, RocketNode, Supercraft,
    WinterNode, MineStrator, DatHost, FREAKHOSTING, Space-Node, GameServerKings and physgun.
  - **Pocketpair's official server guide** (docs.palworldgame.com, v1.0.4 pages).
- Also unreachable: shockbyte.com KB (403 on direct fetch; content seen via search snippets),
  apexminecrafthosting.com (403), nexusmods.com (403; snippets only).
- "(snippet)" = seen only in a search-result summary, not opened.
- About 60 distinct sources are cited.

---

## 1. Who runs Palworld dedicated servers

### Player base (the pool owners come from)

| Period | Steam avg players | Steam peak | Source |
|---|---|---|---|
| Jan 2025 | 97,429 | 208,927 | steamcharts |
| May 2025 (low) | 16,308–22,016 | 30–44k | steamcharts |
| Jun 2026 (pre-1.0) | 26,856 | 51,116 | steamcharts |
| **Jul 2026 (1.0, 10 Jul)** | **404,614** | **956,613** | steamcharts |
| Aug 2026 | 241,576 | 599,628 | steamcharts |
| Sep 2026 | 77,110 | 168,183 | steamcharts |
| Oct 2026 | 40,248 | 78,690 | steamcharts |
| Last 30 days (to ~20 Nov) | 28,768 | 64,415 | steamcharts |

Source: https://steamcharts.com/app/1623730. The press reported the 1.0 peak as 855k on 12 July
(https://www.gamesradar.com/games/survival/palworld-devs-had-high-expectations-for-1-0-but-with-over-855-000-concurrent-steam-players-lead-says-this-is-staggering/,
https://www.pcgamer.com/games/survival-crafting/palworld-pulled-in-almost-900-000-players-over-the-weekend-its-second-highest-peak-ever/
(snippet)) and about 921k on 18 July
(https://news.instant-gaming.com/en/articles/20821-palworld-impresses-with-a-new-peak-of-over-920-000-concurrent-users-on-steam
(snippet)). Lifetime sales are above 30 million
(https://www.notebookcheck.net/Palworld-1-0-surges-to-nearly-1-million-concurrent-Steam-players-as-sales-top-30-million-copies.1352374.0.html
(snippet)). Steam counts leave out Xbox, PS5 and Game Pass players. Since 0.5.0 (March 2025) all of
them can share one dedicated server (https://purexbox.com/news/2025/03/palworlds-crossplay-update-now-live-on-xbox-here-are-the-full-patch-notes (snippet)).

**Takeaway:** the game is back to roughly its 2025 baseline after the 1.0 spike. The next likely
spike is **Update 1.1 "Side by Side", window "winter 2026"**, per Supercraft's roadmap page, which
was last checked 15 Nov 2026 (https://wiki.supercraft.host/article/palworld-roadmap-2026/). A
release should land before that.

### Number of servers
- BattleMetrics lists Palworld servers (https://www.battlemetrics.com/servers/palworld (snippet)), but
  I found no current total. Hostinger cites BattleMetrics for **240,000–270,500 dedicated servers
  since launch** (US about 90k, Germany 48k, China 34k)
  (https://www.hostinger.com/in/tutorials/palworld-statistics (snippet)). **Estimate, and the figures
  probably date from early 2024.**
- The in-game list shows only 200 entries at a time (Shockbyte KB (snippet):
  https://shockbyte.com/help/knowledgebase/articles/palworld-server-known-issues-common-fixes).

### Segments (qualitative; sizes are estimates)

| Segment | Evidence | Typical pain |
|---|---|---|
| **Friends-only, home PC (Windows, SteamCMD or Steam "Palworld Dedicated Server")** | Pocketpair's official path. The docs cover Windows and Linux only (https://docs.palworldgame.com/getting-started/requirements). Most AMP forum posts are home hosts struggling with ports. | Port forwarding, CGNAT, the PC must stay on, RAM (16 GB official; 8 GB "increases the possibility of server crashes") |
| **Homelab / NAS / Docker** | DCK has 3,217 stars; jammsen's image 1,010; Pocketpair's **own** Docker image 200 (https://github.com/pocketpairjp/palworld-dedicated-server-docker). DCK's most-discussed issues are Synology (#35, 67 comments), QNAP ARM (#456, 33), ARM64 (#10, 27), macOS Docker (#556, 25). | Env-var config confusion, ARM/Apple Silicon crashes, permissions. Pocketpair says Docker Desktop on Windows/macOS is "deprecated… increases the risk of saved data corruption" |
| **VPS** | Space-Node: a VPS is "more work than it is worth" for one game server (https://space-node.net/blog/palworld-self-host-vs-vps-vs-game-hosting-2026). A 16 GB cloud VPS costs about $80–96/mo (https://supercraft.host/palworld-self-host-vs-paid-hosting-calculator/ (snippet)). | Owner handles OS, firewall, backups and restarts ("When something breaks at 2am, it is yours") |
| **Rented game host** (G-Portal, Nitrado, Shockbyte, Nodecraft, BisectHosting, LOW.MS, ZAP, Apex, Supercraft…) | Plans about $3–25/mo (snippets via the calculator above). Nodecraft alone has 58 Palworld help articles. Hosts give away tools (Nodecraft's free AdminPal: https://nodecraft.com/support/games/palworld/setup/monitor-your-palworld-server-with-adminpal). | Limited by the host panel: WorldOption.sav and settings confusion, mods, migration, "where is my save" |
| **Public community servers** (`-publiclobby`, crossplay) | Consoles can only join through the in-game community list, not by IP (https://winternode.com/blog/palworld/palworld-1-0-server-guide; https://low.ms/knowledgebase/palworld-server-list-and-password-issues) | Visibility in the list, cheaters and griefers, lag with many bases, moderation |
| **Generic panel users** (AMP, WindowsGSM, Pterodactyl/Pelican) | 341 Palworld topics on the AMP forum since May 2025. WindowsGSM.Palworld plugin has 111 stars. | Same game problems, plus the panel's own quirks |

### GitHub adoption proxy (stars, fetched via the GitHub API on 2026-11-20)

| Stars | Repo | Last push | Note |
|---|---|---|---|
| 3,217 | thijsvanloef/palworld-server-docker | 2026-11-16 | the de-facto Docker path |
| 1,196 | zaigie/palworld-server-tool | 2026-07-16 | panel + save parser; last release v0.12.2 on 14 Jul 2026 |
| 1,010 | jammsen/docker-palworld-dedicated-server | 2026-08-01 | |
| **941** | **xNul/palworld-host-save-fix** | **2024-05-01** | **migration fix, unmaintained, still the 4th-most-starred** |
| 855 | VeroFess/PalWorld-Server-Unoffical-Fix | 2024-01 | memory/crash binary fix from launch |
| 561 | magicbear/palworld-server-toolkit | 2025-04 | save edit and repair |
| 319 | oMaN-Rod/palstudio | 2026-11-08 | save editor |
| 294 | Bluefissure/pal-conf | 2026-07 | settings generator |
| 200 | pocketpairjp/palworld-dedicated-server-docker | 2026-09 | **official** |
| 157 | legoduded/palworld-worldoptions | 2025-04 | WorldOption.sav generator |
| 138 | jmkl009/PalworldCharacterTransfer | 2024-07 | character transfer |
| 116 | palserver-gui/palserver-gui | 2026-11-17 | |
| 81 | Ultimeit/PalDefender | 2026-10 | anti-cheat/admin plugin (binary distributed) |

Reading this table: after Docker images and panels, the **next most-starred things are save and
character fixers and settings generators**. Those needs are about *the world data being wrong*, not
about running the process.

---

## 2. Ranked pain points (centrepiece)

Frequency columns: **AMP** = post-1.0 AMP forum topics in that theme (out of 128; my
classification). **Steam** = Steam Community search hits (rough, all years). **GH** = notable
GitHub issues (comments/reactions). **KB** = hosts with a dedicated help article. "Sources" = number
of independent sources seen in this research.

| # | Pain point | Frequency evidence | Links | Warden status |
|---|---|---|---|---|
| 1 | **Players can't connect** (port 8211/UDP not forwarded, Windows Firewall, CGNAT, hairpin NAT, wrong public IP, playit.gg relays) | AMP **25** (largest single theme, tied). Steam "connection timed out" 518. DCK #856, #866. KB: LOW.MS, GameServerKings, physgun, Liquid Web. Pocketpair documents the hairpin-NAT limit. About 10 sources. | https://www.gameserverkings.com/knowledge-base/palworld/palworld-connection-timed-out/ (snippet) · https://low.ms/de/knowledgebase/troubleshooting-palworld-connection-timed-out-errors (snippet) · https://docs.palworldgame.com/getting-started/deploy-community-server · https://discourse.cubecoders.com/t/connectivity-problem-with-palworld-palworld-trouble-allowing-users-off-local-network-to-join/42365 | **Not addressed.** Tailscale covers the *panel* only. Nothing checks whether the *game port* can be reached from outside, or that the Windows Firewall rule exists. |
| 2 | **Server crashes / won't start / crashes every few hours** (memory growth, OOM, bad config line, mods) | AMP **25**. Steam "server crash" 1,031, "memory leak" 862. DCK #630 (23 c), #888, #920. KB: Shockbyte, NameHero, Nodecraft, xgamingserver, RocketNode, Berrybyte, Liquid Web. More than 10 sources. | https://help.namehero.com/en/article/palworld-known-issues-common-fixes-zbdyrl/ · https://nodecraft.com/support/games/palworld/troubleshooting/fix-palworld-server-crashes · https://rocketnode.com/help/palworld/how-to-prevent-memory-leak-rubber-banding-palword-server-settings (snippet) · https://discourse.cubecoders.com/t/palworld-server-crashing-after-a-few-hours/9418 | **Mostly addressed:** crash and frozen restarts, RAM-threshold restart, nightly restart. **Gaps:** no "why did it crash" (mod culprit, OOM vs. bad ini line); no check that the ini is well formed before start (NameHero: a malformed line makes the server hang or boot with defaults). |
| 3 | **Version mismatch after a game update** ("incompatible version"; server not updated; updater stuck) | AMP **21** (largest Update Failure cluster on 15 Jul, right after 1.0.1). Steam "incompatible version" 89. DCK #947, #878, #104 (19 c). Hosts and press: Dot Esports, xgamingserver, allthings.how, WinterNode. About 9 sources. | https://xgamingserver.com/blog/how-to-fix-palworld-incompatible-version/ (snippet) · https://github.com/thijsvanloef/palworld-server-docker/issues/947 · https://discourse.cubecoders.com/t/palworld-cannot-connect-to-palworld-server/15468 (snippet) · https://winternode.com/blog/palworld/palworld-1-0-server-guide | **Partly.** The nightly SteamCMD update and version hold exist. **Gap:** clients update *during the day*, and every hour until the nightly run is "can't join". Needed: detect a new public build (SteamCMD app_info / build ID) and run a warned update soon after, unless held for mods. Console patches can lag PC by a day or two (snippet, same xgamingserver page), so "hold until consoles have it" also matters on crossplay servers. |
| 4 | **Lost progress: rollbacks, world or character wiped after an update or crash, back to character creation** | AMP 5. Steam "server wipe" 1,670, "new character server" 584, "server rollback" 145. DCK #457, #205, #699, #934. G-Portal wiki page on it. Nodecraft troubleshooting article. Dot Esports coverage. AMP "atomic save failure" topic. About 10 sources. | https://www.g-portal.com/wiki/en/error-loss-of-progress-or-character · https://nodecraft.com/support/games/palworld/troubleshooting/fix-palworld-server-update-wiping-player-progress · https://dotesports.com/palworld/guides/are-palworld-servers-getting-wiped-heres-how-to-restore-your-progress (snippet) · https://github.com/thijsvanloef/palworld-server-docker/issues/457 | **Mostly addressed for whole worlds:** backups, previewed restore with rollback, pre-crash backups kept. **Gaps:** per-player restore (one character back without rolling back everyone) and the save-stall alert (both already on the survey's opportunity list); and the pre-update snapshot should be *labelled* "before update to build X" so owners can find it (the standard advice is "restore Level.sav from right before the update"). |
| 5 | **Mods on servers:** official Workshop mods are Windows-only; UE4SS/PalSchema break on every game update; stale files corrupt saves | AMP **15**. Steam "server mods" 1,678. **DCK's most-reacted open issues are about mods:** #906 "Add Linux UE4SS" (12 reactions), #885 "server-side mod support … on Linux" (9). Pocketpair's mod page. LOW.MS, DatHost, PCGamesN on the 1.0 mod warning. About 9 sources. | https://docs.palworldgame.com/settings-and-operation/mod · https://github.com/thijsvanloef/palworld-server-docker/issues/906 · https://help.dathost.net/article/163-palworld-how-to-install-mods (snippet) · https://www.pcgamesn.com/palworld/1-0-mod-warning (snippet) | **Partly:** update hold while mods are on, and mod changes queued for the next stop. **Gaps:** an official-Workshop-mod manager (`PalModSettings.ini` `ActiveModList`, `PackageName` from `Info.json`, `IsServer` check, `-NoMods` safe boot), and "delete, not disable" cleanup before a major update. |
| 6 | **Settings don't take effect** (WorldOption.sav overrides the ini; the server rewrites the ini on shutdown; malformed OptionSettings line; new 1.0 keys missing) | AMP 8. **DCK #29 "Server config cant modify" has 64 comments, the 2nd most-commented issue ever.** DCK #933, #722, #878; #834 comments ("PalWorldSettings.ini is not being populated"). KB articles: LOW.MS, Apex, PineHosting, xgamingserver. legoduded/palworld-worldoptions (157★). About 10 sources. | https://low.ms/knowledgebase/palworld-worldoption-sav-vs-palworldsettings-ini · https://github.com/thijsvanloef/palworld-server-docker/issues/933 · https://github.com/thijsvanloef/palworld-server-docker/issues/29 · https://pinehosting.com/clientarea/knowledgebase/77/How-To-Fix-Palworld-Settings-Not-Updating.html (snippet) | **Partly:** settings written only while stopped, with plain-words labels. **Not addressed: `WorldOption.sav`** (the survey marks Warden "?" and no Warden code mentions it). LOW.MS lists the symptoms: settings revert, egg timers stuck at 72 h, worker counts reset, Global Palbox import fails. Fix: detect the file, warn, and offer "back up and remove" or "regenerate from my settings". Also: re-read the ini after start to confirm the values stuck. |
| 7 | **Not visible in the community list / consoles can't find or join** (consoles can't join by IP; `-publiclobby`; `CrossplayPlatforms`; list lag of 5–10 min; name filters) | AMP **11** (Xbox, PS5, "make public", "cant find the server", "login error on other platforms"). Steam "server list" 1,004, "crossplay server" 184. KB: Shockbyte, GhostCap, LOW.MS, WinterNode, Nodecraft crossplay article. About 8 sources. | https://low.ms/knowledgebase/palworld-server-list-and-password-issues · https://www.ghostcap.com/palworld-server-not-showing-in-community-server-list (snippet) · https://winternode.com/blog/palworld/palworld-1-0-server-guide · https://discourse.cubecoders.com/t/connectivity-problem-with-palworld-consoles-can-t-join/ (title seen in AMP search) | **Not addressed.** Warden's public status page is a web page; it doesn't help a console player find the server. Needed: a "crossplay readiness" check: `-publiclobby` set, `CrossplayPlatforms` includes Xbox/PS5, game port reachable, name free of odd characters, plus "is it in the list yet?" if any lobby query is possible (unverified). |
| 8 | **Lag / rubber-banding** (bases and worker Pals, dropped items, memory growth, wrong region) | AMP 4. Steam "server lag" 852. At least 8 host performance guides: Nodecraft, ZAP, Supercraft, WinterNode, Gamever, PineHosting, ConnectHosting, Liquid Web. | https://nodecraft.com/support/games/palworld/setup/best-palworld-server-settings-for-performance-and-less-lag · https://gamever.io/knowledge-base/how-to-reduce-lag-optimize-a-busy-palworld-server (snippet) · https://zap-hosting.com/guides/docs/palworld-performance-guide (snippet) | **Partly:** FPS and host metrics, RAM restart, presets. "Which base is causing it" is on the survey's opportunity list. A **performance preset** that applies the settings hosts agree on (BaseCampWorkerMaxNum, DropItemMaxNum, etc.) is a quick win if not already in Warden's presets. |
| 9 | **Moving a co-op / single-player world to a server (host character lost) and character transfer** | AMP 3. Steam "host save" 591, "transfer world server" 175. **xNul host-save-fix has 941★ and is unmaintained since 2024.** Other tools: jmkl009 (138★), physgun converter, TroubleChute 1.0 videos. Guides: Nodecraft, Apex, RespawnHost, ConnectHosting, Gamever. NameHero lists "host character wiped after uploading a local world". About 12 sources. | https://github.com/xNul/palworld-host-save-fix (via search) · https://github.com/dalufishe/palworld-host-save-fix (snippet) · https://hub.tcno.co/videos/easy-migrate-co-op-save-to-dedicated-server-palworld-1-0/ (snippet) · https://connecthosting.net/blog/move-palworld-co-op-world-to-dedicated-server (snippet) | **Not addressed** (deliberately left out of 1.0 because of Oodle save compression). The evidence says this is one of the most *widely searched* needs. It hits at the moment someone first sets up a server, which is exactly when they choose a tool. |
| 10 | **Cheaters, griefers, moderation** (bans, whitelists, base destruction on PvE) | Steam "cheater server" 938, "griefing" 372, "whitelist" 172. PST #83 whitelist request (15 c). Pocketpair's 2024 anti-cheat statements. PalDefender exists for this. Host moderation guides: Nodecraft, Apex, G-Portal. About 9 sources. | https://www.pcgamer.com/palworld-says-public-server-problems-are-due-to-cheating-and-the-influence-of-fraudulent-activities-outlines-plans-to-add-anti-cheat-software-to-nip-it-in-the-bud (snippet) · https://github.com/zaigie/palworld-server-tool/issues/83 · https://help.freakhosting.com/games/palworld/using-palguard-anti-cheat (snippet) | **Addressed** (kick/ban, allow-list, bans from every source, PD cheat logs, fast-levelling flag, audit). Caveat: **PalDefender is Windows-only**, so Linux owners get no anti-cheat (FREAKHOSTING, snippet). |
| 11 | **Admin password / RCON / REST confusion** ("Unauthorized (AdminPassword is empty)", RCON auth failures, `/adminpassword` in game, password visible) | AMP 5. DCK #214 (24 c), #886 (15 c), #939. PST #11, #164, #185. Pocketpair: **"RCON is now deprecated… scheduled to stop functioning in an upcoming update."** About 7 sources. | https://docs.palworldgame.com/api/rcon/ · https://github.com/thijsvanloef/palworld-server-docker/issues/886 · https://github.com/thijsvanloef/palworld-server-docker/issues/939 | **Addressed:** REST-only by design, secrets kept off the browser, setup writes the first settings. Tools that still rely on RCON (AdminPal, many Discord bots, older panels) will break when Pocketpair turns RCON off. |
| 12 | **Install / platform friction** (SteamCMD, NAS, ARM/Apple Silicon, permissions, Docker Desktop) | DCK #35 (67 c), #456 (33), #10 (27), #556 (25), #846 (17), #888, #874. AMP install and update failures (Wine step stalls). Several free "one-click" Windows managers on Nexus (3604, 3576, 3491; snippets). | https://github.com/thijsvanloef/palworld-server-docker/issues/35 · https://github.com/thijsvanloef/palworld-server-docker/issues/888 · https://www.nexusmods.com/palworld/mods/3604 (snippet) | **Mostly addressed** for Windows and Linux x86 (`install-server`, double-click setup). Not for ARM or macOS, which are niche and unsupported by Pocketpair anyway. |
| 13 | **Power and auto-pause** | DCK #32 (20 c, 11 reactions, 2nd most-reacted ever), #911 (29 c), #718, #775, #960, #971 | https://github.com/thijsvanloef/palworld-server-docker/issues/32 · https://github.com/thijsvanloef/palworld-server-docker/issues/911 | **Addressed** (knock-to-wake). DCK's long trail of auto-pause bugs (won't wake, blocks updates and backups) is a warning to test those interactions. |
| 14 | **Inactive players and abandoned bases clutter the world** | PD #77 (purge inactive bases and dropped items). Steam threads on guild auto-reset (`bAutoResetGuildNoOnlinePlayers`). Guides: FREAKHOSTING (abandoned bases), Supercraft (delete player data), Nodecraft ("how to reset player data"). About 6 sources. | https://github.com/Ultimeit/PalDefender/issues/77 · https://help.freakhosting.com/games/palworld/managing-abandoned-bases-auto-decay (snippet) · https://nodecraft.com/support/games/palworld/troubleshooting/how-to-reset-player-data-for-palworld | **Not addressed.** Warden knows "everyone who has played" and last seen, so an "inactive for N days" report, a guarded guild-reset setting, and a backup-first player-data reset fit naturally. |
| 15 | **Join/leave and status to Discord** | AMP 1. DCK #496 (12 c), #322 (11 c). AdminPal exists for this. | https://github.com/thijsvanloef/palworld-server-docker/issues/496 | **Addressed.** |

Smaller items seen once or twice: wrong player names for non-Steam (console) players (DCK #889);
Kubernetes and Global Palbox import (DCK #781, which ties to WorldOption.sav); "0 active users shown"
(AMP); custom IP and port (AMP); verbose logs (DCK #724).

---

## 3. How owners find and choose tools

- **Search → host KB articles and guides first.** For almost every problem query, results were
  dominated by host knowledge bases (Nodecraft, LOW.MS, Shockbyte, G-Portal, ZAP, xgamingserver,
  Supercraft…). Hosts use these to sell plans. Unless a tool is mentioned *inside* those guides or
  ranks for the problem query, owners don't find it while they have the problem. (Observation from
  every search in this report.)
- **GitHub** is the channel for the Docker/homelab segment. Stars concentrate in Docker images
  (DCK 3.2k), with the CN-originated panel PST (1.2k) next.
- **Nexus Mods** lists several "server manager" desktop apps (mods 3604, 3576, 3491 (snippets)). That
  is where Windows players already go for Palworld add-ons. One listing notes "SmartScreen may warn…
  not code-signed" (snippet), the same issue Warden has.
- **YouTube / TroubleChute** for migration (hub.tcno.co "Easy migrate co-op save to dedicated server
  — Palworld 1.0" (snippet)).
- **Host-provided free tools**: Nodecraft's AdminPal (RCON-based; RCON is now deprecated).
- **Generic panels**: AMP (paid), WindowsGSM plugin (111★), Pterodactyl/Pelican eggs.

**What makes them pick (evidence-based):**
- Windows "one-click" + no Docker: three Nexus listings lead with this (snippets). Pocketpair itself
  advises against Docker Desktop on Windows/macOS.
- Free: Royalty Manager is "free for one server" (snippet); PalSupervisor and TRRabbit have paid tiers
  (from the feature survey).
- Works after updates: the most-reacted PST issue is "please add 1.0 compatibility" (#263, 9
  reactions, 10 Jul 2026).

**Complaints about existing tools:**
- **Broken after game updates:** PST save parsing broke on the 0.6.0 save format change
  ("found b'PlM' instead of b'PlZ'", #248; Oodle library load failure, #252). PST asked for a 1.0 fix
  (#263) and its last release was 14 Jul 2026. PalDefender broke on game 1.0.4 (#129 "Not working on
  v1.0.4.102642", 7 Sep 2026: server crashes on login) and on 1.8.1 (#104).
  https://github.com/zaigie/palworld-server-tool/issues/248 ·
  https://github.com/Ultimeit/PalDefender/issues/129
- **Abandoned but still needed:** xNul host-save-fix (941★, last push May 2024); VeroFess fixes
  (2024); palbot archived. palworld-ds-gui (2024).
- **Configuration opacity:** DCK's env-var model generates most of its top issues (#29, #212, #213,
  #239, #722, #878).
- **Security:** the AMP forum topic "under security & privacy the password can be read" (19 Jul 2026);
  REST-auth confusion (#886); Pocketpair notes RCON mishandles multi-byte names. I found no public
  security incident involving a Palworld panel.

---

## 4. What players (not admins) complain about that an admin tool could fix

| Player complaint | Evidence | Admin-tool fix | Warden |
|---|---|---|---|
| Can't join after an update ("incompatible version") | AMP cluster 15 Jul; DCK #947 | Prompt update on a new build; tell players the server version on the status page | Partly (nightly; status page) |
| Rollbacks / lost hours after a crash | Steam "Weird rollback on crash?" (10 replies), "Dedicated server keeps rollbacking"; DCK #699 (saving stops when idle) | Save-stall alert; shorter autosave; per-player restore | Partly |
| Back to character creation | Nodecraft / G-Portal articles | Per-player restore; detect "player file shrank" | Not yet |
| Lag / rubber-banding | Steam "server lag" 852 | Performance preset; heavy-base finder; restart on memory | Partly |
| Can't find the server (console players) | AMP Xbox/PS5 topics | Crossplay readiness check | No |
| Cheaters and griefers | Steam 938 / 372 | Bans, allow-list, PD logs | Yes |
| Wipes and unclear rules | Steam "server wipe" 1,670 (mostly bug wipes) | Announced maintenance and wipes; a rules page | Partly (MOTD, status page) |
| Players unable to rejoin a modded server | AMP "cant rejoin modded palworld" | "How to join" page with the client mod list (survey opportunity #10) | No |

---

## 5. What changed in 2025–2026 and the new needs it created

| Change | Date | New need | Source |
|---|---|---|---|
| **Crossplay** Steam/Xbox/PS5/Mac (0.5.0). Consoles join **only through the community list**. `AllowConnectPlatform` replaced by `CrossplayPlatforms`. | Mar 2025 → 1.0 | List visibility and crossplay checks; migrating old settings keys | https://shacknews.com/article/143534/palworld-update-050-patch-notes (snippet) · https://docs.palworldgame.com/settings-and-operation/configuration · https://low.ms/knowledgebase/palworld-server-list-and-password-issues |
| **Save format changed** (PlZ → PlM, Oodle) | 0.6.0, Jun 2025 | Save parsers broke; migration tools went stale | https://github.com/zaigie/palworld-server-tool/issues/248 |
| **1.0 release**: missions and bounties reset, XP recalculated, bases in reworked areas moved; Pocketpair recommends fresh characters; "improved save process" | 10 Jul 2026 | "Fresh start or keep the world?" choice; pre-update labelled backup | https://minestrator.com/blog/post/palworld-10-is-out-patch-notes-breakdown-and-what-changes-for-your-server · https://winternode.com/blog/palworld/palworld-1-0-server-guide |
| **Official Workshop mod support on servers, Windows only**, "experimental". Old mods must be **deleted, not disabled**. | 1.0 | Mod manager for `PalModSettings.ini`; cleanup before updates | https://docs.palworldgame.com/settings-and-operation/mod · https://wiki.supercraft.host/article/palworld-roadmap-2026/ |
| **Voice chat** on dedicated servers (`bEnableVoiceChat`, distances). 31 new config keys. New defaults (e.g. DeathPenalty `Item`). | 1.0 | Settings UI must track new keys; settings diff across versions | https://github.com/thijsvanloef/palworld-server-docker/issues/834 · https://wiki.supercraft.host/article/palworld-roadmap-2026/ |
| **RCON deprecated**, "scheduled to stop functioning in an upcoming update" | doc v1.0.4 | RCON-based tools (AdminPal, bots) will break; REST-only tools win | https://docs.palworldgame.com/api/rcon/ |
| **Official Docker image** from Pocketpair | repo since Oct 2024, active 2026 | Docker users may move off community images; Warden's sidecar should work with it | https://github.com/pocketpairjp/palworld-dedicated-server-docker |
| **Built-in rolling backups** (`bIsUseBackupSaveData`: 5 per 30 s, 6 per 10 min, 12 per hour, 7 per day; "increases disk load") | current docs | Warden should explain or coordinate with it rather than duplicate it | https://docs.palworldgame.com/settings-and-operation/configuration |
| **Clustering** announced but not usable in 1.0 | Jun–Jul 2026 | Watch for it: multi-server worlds would be a new admin need | https://minestrator.com/blog/post/palworld-10-is-out-patch-notes-breakdown-and-what-changes-for-your-server · https://wiki.supercraft.host/article/palworld-roadmap-2026/ |
| PS5 dedicated *hosting*: conflicting reports; PS5 is a crossplay *client* | — | none for Warden | https://www.neowin.net/news/palworld-gains-crossplay-pal-transfers-between-worlds-photo-mode-and-more/ (snippet) |
| **Update 1.1 "Side by Side"**, "winter 2026" | upcoming | Next player wave, mod breakage and update clusters | https://wiki.supercraft.host/article/palworld-roadmap-2026/ |

---

## 6. Biggest needs Warden does not cover yet (recommendations)

1. **Connectivity and crossplay doctor** (pain points #1 and #7, the two largest AMP themes, about 36
   of 128 topics). Checks: game port listening; Windows Firewall rule present (offer to add it);
   outside reachability (needs an external echo, which conflicts with Warden's "no network calls"
   stance, so make it opt-in or use a documented manual test); CGNAT hint (router WAN IP vs. public
   IP); hairpin-NAT explanation; `-publiclobby` and `CrossplayPlatforms` sanity. **Effort S–M.
   Value: highest.** It answers the most common question at the moment of choosing a tool.
2. **`WorldOption.sav` detection and fix** (#6). Detect, explain, and offer "back up and remove" or
   "regenerate from settings" (legoduded-style). Verify after start. **Effort S (detect, remove) to
   M (write the .sav). Value: high.** Silent and very confusing, and it hits uploaded worlds.
3. **Update-on-new-build** (#3). Poll the public build ID; when the client updates, warn players and
   update within N minutes (respecting mod hold and player count). Label pre-update backups.
   **Effort S. Value: high.** It removes the post-patch "can't join" rush.
4. **Co-op → server migration / host-save fix** (#9). Strongest unmet-need signal (941★ abandoned
   tool, many guides, TroubleChute video). It is L because of Oodle, but a narrower first step is
   possible: a guided "upload your co-op world" flow that handles WorldOption.sav, folder naming and
   `DedicatedServerName`, and *tells* the host to park items in a chest. That is **S–M** and covers
   most cases short of GUID remap.
5. **Inactive-player and base housekeeping** (#14). Report "not seen for N days", a guarded
   guild-reset setting, and a backup-first player reset. **Effort S–M. Value: medium** (community
   servers).
6. **Official Workshop mod manager** (#5). `ActiveModList` editor, `IsServer` check, `-NoMods` safe
   boot. **M. Value: medium-high** for Windows owners, which is Warden's core segment.

---

## Source list (opened unless marked)

Official: docs.palworldgame.com (/, /category/getting-started, /getting-started/requirements,
/getting-started/deploy-community-server, /getting-started/connect-server,
/category/settings-and-operations, /settings-and-operation/mod, /settings-and-operation/configuration,
/api/rcon/); github.com/pocketpairjp/palworld-dedicated-server-docker.

GitHub (API): thijsvanloef/palworld-server-docker issues #10 #29 #32 #35 #104 #205 #212 #213 #214 #239
#283 #322 #456 #457 #496 #556 #630 #699 #722 #781 #834 #846 #856 #866 #878 #885 #886 #888 #889 #906
#911 #920 #933 #934 #939 #947 #960 #971; zaigie/palworld-server-tool #11 #83 #164 #185 #248 #252 #263;
Ultimeit/PalDefender #77 #104 #115 #129; repo search for star counts.

Forums: discourse.cubecoders.com search API (341 topics; 128 post-1.0); steamcommunity.com Palworld
search pages (dedicated server, server rollback, server lag, plus hit counts for 20 queries); Steam
thread 595136433013950072.

Hosts and guides: nodecraft.com (KB index of 58 articles; switching platforms; AdminPal; crossplay);
low.ms (WorldOption.sav; server list and password; KB index); help.namehero.com; g-portal.com wiki;
winternode.com 1.0 guide; minestrator.com 1.0 breakdown; wiki.supercraft.host roadmap; space-node.net;
steamcharts.com. Snippet-only: shockbyte.com KB (403), apexminecrafthosting.com (403),
nexusmods.com (403), xgamingserver, rocketnode, gameserverkings, physgun, pinehosting, dathost,
freakhosting, ghostcap, gamesradar, pcgamer, instant-gaming, notebookcheck, hostinger statistics,
battlemetrics, supercraft calculator, hub.tcno.co, connecthosting, dotesports, pcgamesn, shacknews,
purexbox, neowin, comicbook.

Unreachable: reddit.com (all subreddits), shockbyte.com KB pages (403), apexminecrafthosting.com
(403), nexusmods.com (403).
