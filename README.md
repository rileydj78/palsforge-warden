# Palsforge Warden

A free, open-source admin panel for **Palworld dedicated servers**, from the people behind the
[Palsforge](https://palsforge.com) community server.

> **Status: 1.0 release candidate.** It runs a live community server. Windows builds aren't
> code-signed yet (see below). What's in 1.0 and what isn't: [CHANGELOG.md](CHANGELOG.md).

One program on the machine that runs your Palworld server. Use it **locally** in its own window,
or **remotely** as a web page over your own **Tailscale** network. Nothing is opened to the
internet, and Palworld's admin password never leaves the server.

## What it does

- **The server at a glance:** online or not, players, FPS, uptime, game day, bases, and graphs of
  the last 24 hours or 7 days.
- **Players:** who is on (level, platform, ping), everyone who has ever played (first and last
  seen, time played), kick, ban and unban with a reason, announcements.
- **A nightly routine that can't strand your players:** in-game warnings, save, stop, back up,
  update through SteamCMD, start, and a health check. Whatever fails, the server is started again,
  and the run's report says what happened.
- **Backups you can trust:** every night a backup of the world, settings and mods (and a whole-server
  image weekly or nightly). Each is checksummed, copied to a second place (another disk or a NAS
  share) and checked there. Keeps the newest 14 plus one a week for 8 weeks.
- **Backups on another machine over SFTP:** `warden sftp-setup sftp://user@host/folder` makes a key
  and prints the host key to pin; each backup is uploaded, read back and checksum-matched there,
  and the other machine keeps what this one keeps.
- **Backups in S3-compatible storage** (Amazon S3, Backblaze B2, Wasabi, Cloudflare R2) **or on a
  WebDAV server** (Nextcloud, ownCloud, many NAS boxes): each copy is read back and checked, and
  retention there is applied to what's there (a lost local folder never wipes the off-site copies).
- **Safe restore:** preview what's in a backup, then restore it. Warden checks the backup first,
  makes a safety backup of the world as it is now, keeps the old folders aside, and rolls back if
  anything goes wrong.
- **Runs the server for you, or works alongside what you have:** Warden can start the server
  itself and restart it after a crash, or work with an existing service, scheduled task or
  watchdog script.
- **Community and moderation:** server messages and a private welcome, weekly events (double XP
  weekends switch on and off by themselves), an allow-list, one list of every ban (game and
  PalDefender), player notes; with PalDefender: inventories, Pals, guilds, bases on the map, and
  giving items or Pals. Discord or ntfy alerts, and a live status message in Discord.
- **Keeps a stuck server going:** restarts a frozen server, optionally restarts when memory grows
  too large (with warnings), keeps the last backup before a crash, and stops after 3 automatic
  restarts in 6 hours so a broken server isn't restarted forever.
- **Game settings in plain words**, with presets, import/export, and changes written only while
  the server is stopped (Palworld rewrites its settings when it shuts down).
- **API keys for bots and websites:** `Authorization: Bearer wk_…` on the administration port; a
  key is a moderator or admin (never the owner), revocable at once, and in the activity log.
- **Several worlds:** name them, switch between them, or start a fresh one (a new season) while
  keeping the old: the current world is backed up first and Warden only switches if that worked.
- **A public status page** (optional): name, online, player count, on its own read-only port;
  `warden remote public on` publishes only that page through Tailscale Funnel.
- **New server from nothing:** `warden install-server C:\PalServer` (or type NEW in setup) gets
  SteamCMD and the server and writes first settings Warden can use.
- **No sign-in on the server, accounts for remote admins:** at the machine you're the owner; for
  others you create moderator, admin or owner accounts that sign in over Tailscale, with optional
  two-step sign-in (authenticator app). Every action is in the audit log.

## Install (Windows)

1. Turn on Palworld's REST API: in `Pal\Saved\Config\WindowsServer\PalWorldSettings.ini` set
   `RESTAPIEnabled=True` and an `AdminPassword`, then restart the server.
2. Put `warden.exe` anywhere (for example `C:\Warden`) and **double-click it**. It finds your
   server (or asks where it is), asks Windows for permission once, sets itself to start with
   Windows, and opens the panel. (From a terminal: `warden install -server C:\PalServer`.)
   Warden's data, logs and backups live in `C:\ProgramData\Warden`.
3. Open the panel from the Start menu (**Palsforge Warden**), or run `C:\Warden\warden.exe open`.
   On the server machine there's no account or password: whoever is at the machine is the owner.
4. In **Settings**, choose a second place for backups, the nightly time, and SteamCMD's location,
   then turn the nightly routine on. (It's off until you've looked at the settings.)

**Remote admins:** in **Accounts**, add an account for each person (moderator, admin or owner).
Install [Tailscale](https://tailscale.com) on the server and on their devices, then run
`warden remote on`: the administration port (sign-in required) appears on your Tailscale network
only, at port 8443. The no-sign-in local panel is never published. `warden remote off` takes it
away again.

**Already have a watchdog script or service?** In Settings choose "Another program runs it", give
the command that starts it, and (if your script stops when a file exists) that file. Warden then
stops the server with warnings and starts it again with your command.

**Several servers on one machine:** install one Warden per server with a name, e.g.
`warden install -server C:\PalServer2 -name pvp`. Each gets its own data, ports, task and
Start-menu entry, and a server switcher appears in every panel. They stay fully separate: one
server's problems never touch another's. Pin each server to its own cores in Settings
("CPU cores", e.g. `8-11`). `warden remote on -name pvp` shares that one on its own port.

## Install (Linux)

Build or download `warden-linux-amd64`, then run `warden install -server /path/to/PalServer`:
it prints a systemd unit to install. Everything else is the same.

## Docker

A `Dockerfile` builds Warden as a sidecar for a Palworld server container; see
[docs/docker.md](docs/docker.md) (compose example with a Tailscale sidecar, and
`warden adduser` for accounts without a browser).

## Build from source

Go 1.27 or newer, no C compiler needed:
```
go build -o warden ./cmd/warden                                   # this machine
GOOS=windows GOARCH=amd64 go build -o warden.exe ./cmd/warden    # Windows
```

## Principles

- **Free and open:** MIT licence, no accounts, no telemetry, no update check.
- **Safe by default:** the panel listens on 127.0.0.1 only. Remote use goes through your own
  Tailscale network. Every change needs a signed-in admin, is protected against forged requests,
  and is written to the audit log. Player IP addresses never reach the browser.
- **Plain words:** errors say what's wrong and what to do about it.

See [docs/design.md](docs/design.md) for the design and its status, and
[docs/feature-survey.md](docs/feature-survey.md) for how Warden compares with 16 other tools.

## Signing and privacy

Windows builds are not code-signed yet: SignPath Foundation's free programme asks for an
established project first, so signing comes later (see [the code signing policy](docs/code-signing-policy.md)).
Until then Windows SmartScreen shows a warning ("More info" → "Run anyway"), and Smart App
Control, where it's turned on, blocks the program. Each release lists SHA-256 checksums of its
files. Warden sends nothing anywhere unless you configure it to.

Palworld is a trademark of Pocketpair, Inc. Palsforge Warden is an unofficial fan project and is
not affiliated with or endorsed by Pocketpair.

## Licence

[MIT](LICENSE)
