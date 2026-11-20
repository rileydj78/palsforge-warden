# Running Warden in Docker

Warden runs as a **sidecar** next to a Palworld server container. Tested on Linux (20 Nov 2026)
with [thijsvanloef/palworld-server-docker](https://github.com/thijsvanloef/palworld-server-docker):
status, backups, a settings change with a restart, and a full restore, with the server back
online afterwards.

Three things make it work; each one fixed a real failure in testing:

1. **A small `net` container holds the shared network.** Warden reaches the game's REST API on
   127.0.0.1, so they share a network. If Warden joins the *game's* network directly, Docker gives
   the game a new network every time it restarts it, and Warden is left talking to nothing.
2. **A stop file.** After Warden shuts the game down, Docker's restart policy would start it again
   at once, leaving no moment to write settings or restore a backup. The game's entrypoint waits
   while Warden's stop file exists.
3. **`DISABLE_GENERATE_SETTINGS=true`**, so the image doesn't rewrite PalWorldSettings.ini from its
   environment variables on every start (that would undo every change made in Warden).

```yaml
services:
  net:                                  # never restarts: the network both containers share
    image: alpine:3.22
    command: ["sleep", "infinity"]
    restart: unless-stopped
    init: true
    ports: ["8211:8211/udp"]            # the game port, for players

  palworld:
    image: thijsvanloef/palworld-server-docker:latest
    restart: unless-stopped
    network_mode: service:net
    entrypoint: ["/bin/bash", "-c", "while [ -f /palworld/WARDEN_STOP ]; do sleep 2; done; exec /home/steam/server/init.sh"]
    environment:
      PUID: "1000"
      PGID: "1000"
      PORT: "8211"
      REST_API_ENABLED: "true"
      REST_API_PORT: "8212"
      RCON_ENABLED: "false"             # Pocketpair has deprecated RCON; Warden uses REST only
      ADMIN_PASSWORD: "change-me"
      DISABLE_GENERATE_SETTINGS: "true" # set this after the first start, once the file exists
    volumes: ["palworld:/palworld"]

  warden:
    build: https://github.com/rileydj78/palsforge-warden.git
    restart: unless-stopped
    network_mode: service:net
    command: ["-server", "/palworld"]
    volumes: ["palworld:/palworld", "warden:/data"]

  tailscale:                            # remote admins reach the administration port here
    image: tailscale/tailscale:latest
    restart: unless-stopped
    network_mode: service:net
    environment: { TS_AUTHKEY: "tskey-...", TS_HOSTNAME: "warden", TS_SERVE_CONFIG: /serve.json }
    volumes: ["./serve.json:/serve.json:ro", "ts:/var/lib/tailscale"]

volumes: { palworld: {}, warden: {}, ts: {} }
```

**First start:** start `palworld` once without `DISABLE_GENERATE_SETTINGS` so the image writes the
settings file (with the REST API on and your admin password), then add the line and restart it.
Before starting `warden` the first time, give it these settings (in the `warden` volume, as
`warden.json`) so it doesn't try to run the game itself:

```json
{"serverDir": "/palworld",
 "control": {"mode": "external", "startCommand": ["true"], "stopMarker": "WARDEN_STOP"}}
```

`startCommand` is `true` because Docker starts the game; Warden only removes the stop file.

**Remote admins:** `serve.json` publishes the administration port (sign-in required) on your
tailnet only:

```json
{ "TCP": { "443": { "HTTPS": true } },
  "Web": { "${TS_CERT_DOMAIN}:443": { "Handlers": { "/": { "Proxy": "http://127.0.0.1:8283" } } } } }
```

Make accounts with `docker compose exec -T warden warden adduser moss owner -data /data` (then
type the password). Never publish 8282 (the local, no-sign-in port), 8283 or 8212 with `ports:`.

**Limits in Docker:** the "Can players get in?" check can't read the host's firewall from inside
a container, and Warden can't pin CPU cores (use the compose file's `cpus:` instead).
