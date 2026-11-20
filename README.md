# Palsforge Warden

A free, open-source admin tool for **Palworld dedicated servers**, from the people behind the
[Palsforge](https://palsforge.com) community server.

> **Status: design stage.** Nothing to install yet. See [docs/design.md](docs/design.md) for the
> plan and [docs/research.md](docs/research.md) for what existing tools do.

## What it will do

One install on the machine that runs your Palworld server. Use it **locally** in its own window on
that machine, or **remotely** as a web page over **Tailscale** (nothing is opened to the internet):

- **See the server at a glance:** online or not, players, FPS, uptime, history.
- **Look after players:** kick, ban, unban, an allow-list, announcements.
- **Never lose the world:** scheduled saves and restarts with in-game warnings, complete backups
  (saves, settings, mods, logs and the server itself) with retention, copied to a second place and
  checked.
- **Change settings safely:** every `PalWorldSettings.ini` option explained, validated, applied on
  the next restart.
- **Stay up to date:** game updates through SteamCMD, as part of the nightly restart.
- **Know who did what:** an audit log of every admin action.

## Principles

- **Free and open:** MIT licence, no accounts, no telemetry.
- **Safe by default:** the panel is for you and your admins only. It never asks you to expose
  Palworld's own admin API to the internet, and it keeps that password on the server.
- **Works on the hardware you have:** Windows and Linux servers, modest machines.

Palworld is a trademark of Pocketpair, Inc. Palsforge Warden is an unofficial fan project and is
not affiliated with or endorsed by Pocketpair.

## Licence

[MIT](LICENSE)

## Signing and privacy

Windows builds are being set up for free open-source code signing through SignPath Foundation;
see [the code signing policy](docs/code-signing-policy.md). Warden sends nothing anywhere unless
you configure it to (no telemetry, no update check).
