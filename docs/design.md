# Palsforge Warden: design (draft)

Status: first draft, 2026-11-19. Decisions marked **(decided)**; everything else is a proposal to
discuss. The research behind it is in [research.md](research.md).

## 1. What it is

**One program on the machine that runs the Palworld server** (Windows first, Linux too), plus a
**web panel** served by that program, plus later a **small desktop launcher** **(decided: both)**.

```
 phone / laptop browser ─┐                    ┌─ Palworld REST API (127.0.0.1:8212, never exposed)
                         ├─ Warden web panel ─┤─ PalDefender REST (127.0.0.1:17993, optional)
 desktop launcher (later)┘   (HTTPS, logins)  ├─ server files: saves, PalWorldSettings.ini, logs
                                              ├─ the server process (start, stop, watchdog)
                                              └─ SteamCMD (updates), backup targets
```

Because Warden runs next to the server it can do what panel-only tools can't: back up files, edit
settings, run the nightly routine and restart the server, with **one install**.

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

### Version 0.3 and later
11. **Desktop launcher:** a small app that remembers your servers and opens each panel in its own
    window, with a tray icon showing their state.
12. **More than one server** in one panel.
13. **Linux packages and a Docker image.**

## 3. Safety by default

- Palworld's admin API stays on `127.0.0.1`; Warden is the only thing that talks to it and keeps
  its password on the server. Warden never offers to expose port 8212.
- The panel listens on the local network only by default, over HTTPS (a certificate it makes on
  first run, or Tailscale's when the admin uses `tailscale serve`). The setup guide recommends
  Tailscale or a VPN for use away from home, not port forwarding.
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
| Desktop launcher | **Tauri** (later) | Small native app around the same web panel |
| Releases | GitHub Releases, signed; a download page on palsforge.com | Free and transparent |

Alternative considered: Python (faster for us to write, but admins would need Python installed or
a large bundled runtime) and Node.js (the RNZ01 panel's route; same runtime problem).

## 5. Palsforge as the first user

The Palsforge server already runs a hand-made version of the routine (PowerShell scripts: nightly
warnings, save, stop, data backup and server image, game update, copy to a second machine with
checksums, reboot, and a fallback). Warden turns that into a product anyone can use, and Palsforge
switches to Warden once version 0.1 does the same job.

## 6. Open questions

1. Confirm the stack (Go + Svelte + SQLite; Tauri for the launcher later).
2. Allow-list: Palworld has none of its own; use PalDefender's when present, and the server
   password otherwise?
3. Which second places for backups in 0.1: folder and network share for sure; SFTP too?
4. Discord in 0.1 or 0.2?
5. Should the panel have an optional, read-only **public status page** (online, players, uptime)
   for a community website like palsforge.com?
