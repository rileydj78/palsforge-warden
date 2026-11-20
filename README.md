# Palsforge Warden

A free, open-source admin panel for **Palworld dedicated servers**, from the people behind the
[Palsforge](https://palsforge.com) community server.

> **Status: 0.1 in testing.** It runs on a live community server. The first Windows release
> waits for code signing (see below); until then, build it yourself or use the CI build.

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
- **Safe restore:** preview what's in a backup, then restore it. Warden checks the backup first,
  makes a safety backup of the world as it is now, keeps the old folders aside, and rolls back if
  anything goes wrong.
- **Runs the server for you, or works alongside what you have:** Warden can start the server
  itself and restart it after a crash, or work with an existing service, scheduled task or
  watchdog script.
- **No sign-in on the server, accounts for remote admins:** at the machine you're the owner; for
  others you create moderator, admin or owner accounts that sign in over Tailscale. Every action
  is in the audit log.

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

See [docs/design.md](docs/design.md) for the plan and what comes next (settings editor for the
game itself, logs, notifications, PalDefender, a map, and mod management).

## Signing and privacy

Windows builds are being set up for free open-source code signing through SignPath Foundation;
see [the code signing policy](docs/code-signing-policy.md). Warden sends nothing anywhere unless
you configure it to.

Palworld is a trademark of Pocketpair, Inc. Palsforge Warden is an unofficial fan project and is
not affiliated with or endorsed by Pocketpair.

## Licence

[MIT](LICENSE)
