# Changelog

## 1.0.0-rc.3 (2026-11-20)

- Linux tested with a real Palworld server in Docker (backups, settings with a restart, a full
  restore); three sidecar problems found and fixed; the Docker guide is the tested setup.
- Bring a world from co-op; crash hints; the settings file checked before every start.
- "Update soon" checks at start-up and watches Pocketpair's patch announcements on Steam.
- Status page: how to join, who can join, rules. Players: filter by recent or inactive.
- Backups page lists the off-site places with their last copy; on phones each backup is a card.
- Dashboard tells "paused", "stopped" and "not answering" apart.
- A fourth security review: 11 findings, none high, all fixed.

## 1.0.0-rc.2 (2026-11-20)

Everything in rc.1, plus what the needs research showed server owners struggle with most, and a
polish pass from screenshots of every page (phone and desktop, light and dark) and an
accessibility scan (no WCAG 2 A/AA violations).

**From the needs research (server owners' most common problems)**
- "Can players get in?": checks the game port, Windows Firewall (rules that really cover it),
  the community-list flag consoles depend on, crossplay, and gives the router/CGNAT hint, all on
  the machine, each problem with its exact fix.
- A world's WorldOption.sav silently overrides the settings file: Warden detects it, can set it
  aside, and shows any setting the running server isn't using.
- Update soon after a new game build (optional), so players aren't turned away as
  "incompatible version" until the night: checked 2 minutes after Warden starts and every 30
  minutes, and every 5 minutes for 6 hours after Pocketpair announces a patch on Steam. New
  patch announcements reach the admin (Discord/ntfy) with the link; the dashboard shows the
  latest Palworld news. (Only with this option on does Warden read Steam's public news feed.)
- REST API only: Pocketpair has deprecated RCON and says it will stop working.
- Bring a world from co-op: upload the world folder's zip; Warden adds it beside the current
  world (never replacing one), sets its WorldOption.sav aside, explains the host-character
  limit, and switches with a backup first.
- Crash hints: after a crash, the alert names what the crash report and logs point at (a mod,
  running out of memory, the game's error).
- The settings file is checked before every start (a broken file makes the game run on defaults).
- Linux: tested with a real server in Docker; the Docker guide is the tested setup.
- Phones: one-row section menu, readable graphs; the last-backup tile shows the newest backup.

## 1.0.0-rc.1 (2026-11-20)

The first release candidate. Warden has run a live Palworld community server on Windows since
19 November, with three test servers beside it for the multi-server features.

**Running the server**
- Supervise the server (crash restarts, CPU cores) or work alongside an existing watchdog or service.
- Nightly routine: in-game warnings, save, stop, back up, update through SteamCMD, start, health check.
  Whatever fails, the server is started again, and the run's report says what happened.
- Restarts a frozen server and (optionally) one using too much memory, at most 3 times in 6 hours.
- Holds game updates while mods are on until an admin allows the new build.
- Auto-pause when empty (private servers): the first connection attempt wakes it.
- Several servers on one machine (one Warden each, with a switcher) and several worlds per server.
- Install a server from nothing: `warden install-server`, or NEW in the double-click setup.

**Backups**
- World, settings and mods nightly; a whole-server image weekly or nightly; SHA-256 checksums.
- Second places: another folder or NAS, SFTP (pinned host key), S3-compatible storage, WebDAV.
  Every copy is read back and checked; retention there is decided from what's there.
- Restore with a preview, a safety backup and automatic rollback. Backups before a crash are kept.

**Players and community**
- Who's on, everyone who has played, kick / ban / unban (game and PalDefender lists), notes,
  allow-list, a fast-levelling flag, a map with bases and last known positions.
- With PalDefender: inventories, Pals, guilds, giving items or Pals, starter kits, cheat logs.
- Server messages and welcomes, weekly events (e.g. double-XP weekends), join/leave alerts,
  a live Discord status message, a public read-only status page.

**Settings**
- The game's settings in plain words, with presets and import/export, written only while the
  server is stopped (Palworld rewrites its file on shutdown). Engine.ini and launch options.

**Access and safety**
- No sign-in at the server machine itself; accounts for remote admins on a separate port, shared
  only on your Tailscale network. Roles, two-step sign-in, API keys for bots, an audit log.
- Nothing listens beyond 127.0.0.1; secrets never reach the browser; player IP addresses are
  never shown; Warden's folders are restricted to SYSTEM and Administrators on Windows.
- Three independent security reviews; every finding fixed.

**Known limits**
- Windows builds are not code-signed yet: SmartScreen warns ("More info" → "Run anyway"), and
  Smart App Control, where it's on, blocks the program.
- No translations yet (planned for 1.1); no character / host-save migration (Palworld's saves use
  proprietary Oodle compression); no Discord command bot (API keys let existing bots use Warden).
- Not yet tried by hand: the double-click setup's Windows permission prompt; the map's
  coordinates against in-game positions.
