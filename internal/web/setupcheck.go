package web

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/paldefender"
)

// Check is one line of the setup check.
type Check struct {
	Area   string `json:"area"`
	OK     bool   `json:"ok"`
	Warn   bool   `json:"warn"` // not wrong, but worth a look
	Detail string `json:"detail"`
}

func (s *Server) setupCheckRoutes(mux *http.ServeMux) {
	c := s.Control
	if c == nil {
		return
	}
	mux.HandleFunc("GET /api/setup-check", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		var out []Check
		add := func(area string, ok, warn bool, f string, a ...any) {
			out = append(out, Check{area, ok, warn, fmt.Sprintf(f, a...)})
		}
		if _, err := s.Game.Info(ctx); err == nil {
			add("Game connection", true, false, "Palworld's REST API answers Warden.")
		} else {
			add("Game connection", false, false, "Palworld's REST API doesn't answer: %v", err)
		}
		ini, _ := config.ReadSettings(config.SettingsPath(c.Cfg.ServerDir))
		if ini != nil && len(ini.AdminPassword) < 12 {
			add("Admin password", false, false, "The game's AdminPassword is short (%d characters). Anyone who reaches the admin API with it controls the server: use 16 or more random characters.", len(ini.AdminPassword))
		} else if ini != nil {
			add("Admin password", true, false, "Long enough.")
		}
		open := host.OpenPorts()
		exposed := []string{}
		rest, rcon := 8212, 25575
		if ini != nil {
			rest = ini.RESTPort
			if p, err := strconv.Atoi(ini.All["RCONPort"]); err == nil && p > 0 {
				rcon = p
			}
		}
		for port, name := range map[int]string{rest: fmt.Sprintf("the game's REST API (%d)", rest), rcon: fmt.Sprintf("RCON (%d)", rcon), 17993: "PalDefender's API (17993)"} {
			if open[port] {
				exposed = append(exposed, name)
			}
		}
		if len(exposed) > 0 {
			verb, them := "listen", "them"
			if len(exposed) == 1 {
				verb, them = "listens", "it"
			}
			add("Exposure", false, true, "%s %s beyond this machine. Make sure the firewall blocks %s from outside: anyone who reaches it can control the server.", strings.Join(exposed, ", "), verb, them)
		} else {
			add("Exposure", true, false, "The admin APIs only listen on this machine.")
		}
		if paldefender.Installed(c.Cfg.ServerDir) {
			if s.PalDef != nil {
				add("PalDefender", true, false, "Connected: guilds, exact map positions, messages and IP bans are on.")
			} else {
				add("PalDefender", false, true, "Installed but not connected. Connect it on this page for guilds, map positions, messages and IP bans.")
			}
		}
		if len(c.Cfg.Backups.Offsite)+len(c.Cfg.Backups.SFTP)+len(c.Cfg.Backups.S3) == 0 {
			add("Backups", false, false, "Backups are only on this machine. Add a second place in Settings so a dead disk can't take them.")
		} else {
			add("Backups", true, false, "Copied to %d second place(s) and checked.", len(c.Cfg.Backups.Offsite)+len(c.Cfg.Backups.SFTP)+len(c.Cfg.Backups.S3))
		}
		if list, _ := backup.ListTag(c.Runner.Backups.LocalDir(), c.Cfg.Backups.Tag); len(list) == 0 {
			add("Last backup", false, true, "No backup yet.")
		} else if age := time.Since(list[0].When); age > 36*time.Hour {
			add("Last backup", false, true, "The newest backup is %d hours old.", int(age.Hours()))
		} else {
			add("Last backup", true, false, "%s.", list[0].Name)
		}
		if c.Cfg.Nightly.Enabled {
			add("Nightly routine", true, false, "On, at %s.", c.Cfg.Nightly.Time)
		} else {
			add("Nightly routine", false, true, "Off. Turn it on in Settings (unless another program already restarts and backs up the server).")
		}
		if c.Cfg.Update.Enabled {
			if _, err := os.Stat(c.Cfg.Update.SteamCMD); c.Cfg.Update.SteamCMD == "" || err != nil {
				add("Game updates", false, false, "Updates are on but SteamCMD isn't found: set its path in Settings.")
			} else {
				add("Game updates", true, false, "Through SteamCMD in the nightly routine.")
			}
		} else {
			add("Game updates", false, true, "Off: the game won't update itself.")
		}
		if c.Cfg.Notify.DiscordWebhook == "" && c.Cfg.Notify.NtfyURL == "" {
			add("Notifications", false, true, "Nobody is told when something goes wrong. Add a Discord webhook or ntfy in Settings.")
		} else {
			add("Notifications", true, false, "On.")
		}
		ts := "tailscale"
		if runtime.GOOS == "windows" {
			ts = `C:\Program Files\Tailscale\tailscale.exe`
		}
		if outb, err := exec.CommandContext(ctx, ts, "serve", "status").CombinedOutput(); err != nil {
			add("Remote access", false, true, "Tailscale isn't installed, so other admins can't reach Warden. Install Tailscale, then run: warden remote on")
		} else if strings.Contains(string(outb), "8283") {
			add("Remote access", true, false, "Remote admins can sign in over your Tailscale network.")
		} else {
			add("Remote access", false, true, "Tailscale is installed but Warden isn't shared. Run: warden remote on")
		}
		writeJSON(w, 200, out)
	}))
}
