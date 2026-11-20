# Changelog

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
