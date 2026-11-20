# Running Warden in Docker

Warden can run as a **sidecar** next to a Palworld server container (for example
[thijsvanloef/palworld-server-docker](https://github.com/thijsvanloef/palworld-server-docker)).
The Palworld container keeps running the game; Warden works in **"another program runs it"** mode.

What Warden needs:

- **The server folder**, mounted read-write (settings, saves, mods, backups): mount the same
  volume the Palworld container uses at `/palworld`.
- **The REST API** of the game: share the network namespace with the Palworld container
  (`network_mode: service:palworld`) so `127.0.0.1:8212` works and the REST API never leaves
  the pair.
- **A way to stop and start the game.** Use Docker's restart policy: Warden's stop file
  (`stopMarker`) tells your container's entry script to exit, and `restart: unless-stopped` brings
  it back. With images that don't support a stop file, set the start command to a small script
  that calls the image's own restart hook.
- **A place for Warden's data**: a `/data` volume (database, logs, `warden.json`).

```yaml
services:
  palworld:
    image: thijsvanloef/palworld-server-docker:latest
    restart: unless-stopped
    ports: ["8211:8211/udp", "27015:27015/udp"]
    environment:
      REST_API_ENABLED: "true"
      ADMIN_PASSWORD: "change-me"
    volumes: ["palworld:/palworld"]

  warden:
    build: https://github.com/rileydj78/palsforge-warden.git
    restart: unless-stopped
    network_mode: service:palworld   # 127.0.0.1:8212 is the game's REST API
    volumes: ["palworld:/palworld", "warden:/data"]

  tailscale:                          # remote admins reach the administration port here
    image: tailscale/tailscale:latest
    network_mode: service:palworld
    environment: { TS_AUTHKEY: "tskey-...", TS_HOSTNAME: "warden", TS_SERVE_CONFIG: /serve.json }
    volumes: ["./serve.json:/serve.json:ro", "ts:/var/lib/tailscale"]

volumes: { palworld: {}, warden: {}, ts: {} }
```

`serve.json` publishes the administration port (sign-in required) on your tailnet only:

```json
{ "TCP": { "443": { "HTTPS": true } },
  "Web": { "${TS_CERT_DOMAIN}:443": { "Handlers": { "/": { "Proxy": "http://127.0.0.1:8283" } } } } }
```

Never publish port 8282 (the local, no-sign-in port) or 8283 with `ports:`.

The first time, open a shell in the container (`docker compose exec warden sh`) or edit
`warden.json` in the `warden` volume: set `control.mode` to `external`, and the start command and
stop file your image uses. Then make the remote admins' accounts:

```
docker compose exec -T warden warden adduser moss owner -data /data   # then type the password
```

Not tested on a live server yet.
