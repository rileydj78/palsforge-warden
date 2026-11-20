package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/gamesettings"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/logs"
	"github.com/rileydj78/palsforge-warden/internal/mods"
	"github.com/rileydj78/palsforge-warden/internal/nightly"
	"github.com/rileydj78/palsforge-warden/internal/notify"
	"github.com/rileydj78/palsforge-warden/internal/paldefender"
	"github.com/rileydj78/palsforge-warden/internal/worlds"
)

// Control is the server-control side of the panel (nil when Warden only watches).
type Control struct {
	Cfg    config.Warden
	Host   *host.Host
	Runner *nightly.Runner
}

func (s *Server) controlRoutes(mux *http.ServeMux) {
	c := s.Control
	if c == nil {
		return
	}
	mux.HandleFunc("GET /api/server", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		busy, _ := c.Runner.Busy()
		runs, _ := s.Store.Runs(1)
		out := map[string]any{"host": c.Host.State(), "build": c.Host.BuildID(), "routineRunning": busy, "memory": c.Host.Memory(), "machine": host.MachineLoad(), "disk": host.DiskOf(c.Cfg.ServerDir), "health": c.Cfg.Health, "proof": nightly.LastProof(c.Runner.Backups.DataDir),
			"nightly": c.Cfg.Nightly, "update": c.Cfg.Update.Enabled, "offsite": len(c.Cfg.Backups.Offsite) + len(c.Cfg.Backups.Remotes())}
		if c.Cfg.Nightly.Enabled {
			out["nextRun"] = nightly.NextRun(c.Cfg.Nightly.Time, time.Now())
		}
		if len(runs) > 0 {
			out["lastRun"] = runs[0]
		}
		if list, err := backup.ListTag(c.Runner.Backups.LocalDir(), c.Cfg.Backups.Tag); err == nil {
			for _, a := range list { // newest first: the newest world backup, however it was made
				if a.Kind == backup.Data {
					out["lastBackup"] = map[string]any{"name": a.Name, "when": a.When}
					break
				}
			}
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("GET /api/runs", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		runs, err := s.Store.Runs(60)
		if err != nil {
			fail(w, 500, "unavailable")
			return
		}
		writeJSON(w, 200, runs)
	}))
	mux.HandleFunc("GET /api/events", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		ev, err := s.Store.Events(200)
		if err != nil {
			fail(w, 500, "unavailable")
			return
		}
		writeJSON(w, 200, ev)
	}))
	mux.HandleFunc("GET /api/backups", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		type place struct {
			Dir      string           `json:"dir"`
			Archives []backup.Archive `json:"archives"`
			Error    string           `json:"error,omitempty"`
		}
		var places []place
		for _, d := range append([]string{c.Runner.Backups.LocalDir()}, c.Cfg.Backups.Offsite...) {
			a, err := backup.ListTag(d, c.Cfg.Backups.Tag)
			p := place{Dir: d, Archives: a}
			if p.Archives == nil {
				p.Archives = []backup.Archive{}
			}
			if err != nil {
				p.Error = "not reachable: " + err.Error()
			}
			places = append(places, p)
		}
		// Places on other machines and in cloud storage: each with its last copy.
		st := nightly.RemoteStatuses(c.Runner.Backups.DataDir)
		var remotes []nightly.RemoteStatus
		for _, t := range c.Cfg.Backups.Remotes() {
			r, ok := st[t.Name()]
			if !ok {
				r = nightly.RemoteStatus{Name: t.Name()}
			}
			remotes = append(remotes, r)
		}
		writeJSON(w, 200, map[string]any{"places": places, "remotes": remotes})
	}))

	// Changes. Each is audited; long ones run in the background and land in the run history.
	do := func(path, min, what string, f func(ctx context.Context) error) {
		mux.HandleFunc("POST "+path, s.guard(min, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
			end, err := c.Runner.Begin(what)
			if err != nil {
				fail(w, 409, err.Error())
				return
			}
			defer end()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			err = f(ctx)
			s.Store.Audit(ses.User.Name, what, "", "", err == nil)
			if err != nil {
				fail(w, 502, err.Error())
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
		}))
	}
	do("/api/server/start", auth.Admin, "start server", c.Host.Start)
	do("/api/server/stop", auth.Admin, "stop server", func(ctx context.Context) error {
		c.Host.PlanStop(ctx)
		if err := s.Game.Shutdown(ctx, 10, "The server is stopping for maintenance."); err != nil || !c.Host.WaitStopped(ctx, 2*time.Minute) {
			return c.Host.ForceStop(ctx)
		}
		return nil
	})
	mux.HandleFunc("GET /api/worlds", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		ws, err := worlds.List(c.Cfg.ServerDir, config.SettingsPath(c.Cfg.ServerDir), c.Runner.Backups.DataDir)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"worlds": ws, "pending": worlds.Pending(c.Runner.Backups.DataDir)})
	}))
	mux.HandleFunc("POST /api/worlds/switch", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			ID      string `json:"id"` // a world ID, or "new" for a fresh world
			Minutes int    `json:"minutes"`
		}
		if readJSON(r, &in) != nil || in.Minutes < 0 || in.Minutes > 60 {
			fail(w, 400, "bad request")
			return
		}
		if busy, what := c.Runner.Busy(); busy {
			fail(w, 409, "busy with "+what)
			return
		}
		var warn []int
		for _, m := range []int{15, 5, 1} {
			if m <= in.Minutes {
				warn = append(warn, m)
			}
		}
		s.Store.Audit(ses.User.Name, "switch world", in.ID, fmt.Sprintf("warning %d min", in.Minutes), true)
		go c.Runner.SwitchWorld(context.Background(), in.ID, warn)
		writeJSON(w, 202, map[string]bool{"started": true})
	}))
	mux.HandleFunc("POST /api/worlds/import", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		r.Body = http.MaxBytesReader(w, r.Body, worlds.MaxImport)
		file, hdr, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "choose the world's .zip file (up to 2 GB)")
			return
		}
		defer file.Close()
		tmp, err := os.CreateTemp(c.Runner.Backups.DataDir, "world-*.zip")
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, file)
		tmp.Close()
		if err != nil {
			fail(w, 400, "the upload didn't finish")
			return
		}
		id, notes, err := worlds.Import(c.Cfg.ServerDir, tmp.Name())
		s.Store.Audit(ses.User.Name, "import world", hdr.Filename, id, err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		worlds.SetLabel(c.Runner.Backups.DataDir, id, cleanText(strings.TrimSuffix(hdr.Filename, ".zip"), 40))
		writeJSON(w, 200, map[string]any{"id": id, "notes": notes})
	}))
	mux.HandleFunc("POST /api/worlds/label", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ ID, Label string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := worlds.SetLabel(c.Runner.Backups.DataDir, in.ID, cleanText(in.Label, 40))
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/engine", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		p := gamesettings.EnginePath(config.SettingsPath(c.Cfg.ServerDir))
		cur, _ := os.ReadFile(p)
		pend, waiting := gamesettings.PendingEngine(c.Runner.Backups.DataDir)
		writeJSON(w, 200, map[string]any{"path": p, "text": string(cur), "pending": pend, "waiting": waiting})
	}))
	mux.HandleFunc("POST /api/engine", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Text string `json:"text"`
		}
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := gamesettings.QueueEngine(c.Runner.Backups.DataDir, in.Text)
		s.Store.Audit(ses.User.Name, "change Engine.ini", "", "written at the next stop", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/connection", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		port := nightly.GamePort(c.Cfg.Control.Args)
		f := c.Host.Connection(port)
		if c.Cfg.Control.Mode != "supervise" { // another program starts it: its command line says
			if p := nightly.GamePort(strings.Fields(f.CommandLine)); p != port {
				port = p
				f = c.Host.Connection(port)
			}
		}
		type item struct {
			State, Area, Text string
		}
		var out []item
		add := func(state, area, text string, a ...any) {
			out = append(out, item{state, area, fmt.Sprintf(text, a...)})
		}
		if f.Listening {
			add("ok", "Game port", "The game is listening on UDP %d.", port)
		} else {
			add("bad", "Game port", "Nothing is listening on UDP %d: the server isn't running, or it uses another port.", port)
		}
		switch {
		case !f.Known:
			add("note", "Firewall", "Warden can't read this machine's firewall; make sure it lets UDP %d in.", port)
		case !f.FirewallOn:
			add("ok", "Firewall", "Windows Firewall is off on this machine.")
		case f.Allowed:
			add("ok", "Firewall", "Windows Firewall lets UDP %d in.", port)
		default:
			add("bad", "Firewall", "Windows Firewall has no rule letting players in. In an Administrator PowerShell: New-NetFirewallRule -DisplayName \"Palworld %d\" -Direction Inbound -Protocol UDP -LocalPort %d -Action Allow", port, port)
		}
		lc := strings.ToLower(f.CommandLine)
		switch {
		case f.CommandLine == "":
			add("note", "Server list", "Couldn't read the server's launch options. It shows in the community server list only with -publiclobby, and console players can only join from that list.")
		case strings.Contains(lc, "-publiclobby"):
			add("ok", "Server list", "Launched with -publiclobby: it appears in the community server list (console players join from there).")
		default:
			add("warn", "Server list", "Not in the community server list (no -publiclobby). Fine for a private server, but console players can only join from that list.")
		}
		if ini, err := config.ReadSettings(config.SettingsPath(c.Cfg.ServerDir)); err == nil {
			cp := ini.All["CrossplayPlatforms"]
			switch {
			case cp == "":
				add("note", "Crossplay", "CrossplayPlatforms isn't set: the game's default applies.")
			case strings.Contains(cp, "Xbox") && strings.Contains(cp, "PS5"):
				add("ok", "Crossplay", "Open to %s.", strings.Trim(cp, "()"))
			default:
				add("warn", "Crossplay", "Only %s can join (CrossplayPlatforms). Add Xbox, PS5 and Mac in Game settings to let console and Mac players in.", strings.Trim(cp, "()"))
			}
		}
		lan := "this machine's address on your network"
		if len(f.LANAddrs) > 0 {
			lan = strings.Join(f.LANAddrs, " or ")
		}
		add("note", "Router", "From the internet, players need your router to forward UDP %d to %s. If they still can't connect, your provider may use CGNAT (the router's internet address starts with 100.64-100.127, or differs from what whatismyip.com shows): forwarding can't work then; ask for a public IP or use a VPS.", port, lan)
		writeJSON(w, 200, map[string]any{"port": port, "checks": out})
	}))
	mux.HandleFunc("POST /api/update/check", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		latest, err := c.Host.LatestBuild(r.Context())
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"installed": c.Host.BuildID(), "latest": latest, "allowed": c.Host.AllowedBuild(), "hold": c.Cfg.Update.HoldForMods})
	}))
	mux.HandleFunc("POST /api/update/allow", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Build string `json:"build"`
		}
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := c.Host.AllowBuild(in.Build)
		s.Store.Audit(ses.User.Name, "allow game update", in.Build, "", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/server/restart", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Minutes int `json:"minutes"`
		}
		if readJSON(r, &in) != nil || in.Minutes < 0 || in.Minutes > 60 {
			fail(w, 400, "warn 0 to 60 minutes ahead")
			return
		}
		if busy, what := c.Runner.Busy(); busy {
			fail(w, 409, "busy with "+what)
			return
		}
		var warn []int
		for _, m := range []int{30, 15, 10, 5, 1} {
			if m <= in.Minutes {
				warn = append(warn, m)
			}
		}
		if in.Minutes > 0 && (len(warn) == 0 || warn[0] != in.Minutes) {
			warn = append([]int{in.Minutes}, warn...)
		}
		s.Store.Audit(ses.User.Name, "restart server", "", fmt.Sprintf("warning %d min", in.Minutes), true)
		go c.Runner.Restart(context.Background(), "restart by "+ses.User.Name, warn)
		writeJSON(w, 202, map[string]bool{"started": true})
	}))
	mux.HandleFunc("POST /api/routine/run", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Warn   bool `json:"warn"`
			Update bool `json:"update"`
		}
		readJSON(r, &in)
		if busy, _ := c.Runner.Busy(); busy {
			fail(w, 409, "a run is already in progress")
			return
		}
		warn := []int{}
		if in.Warn {
			warn = c.Cfg.Nightly.WarnMinutes
		}
		s.Store.Audit(ses.User.Name, "run routine now", "", map[bool]string{true: "with warnings", false: "no warnings"}[in.Warn], true)
		go c.Runner.RunNow(context.Background(), "manual", warn, in.Update && c.Cfg.Update.Enabled)
		writeJSON(w, 202, map[string]bool{"started": true})
	}))
	mux.HandleFunc("GET /api/logs", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		srcs := logs.Sources(c.Cfg.ServerDir, c.Runner.Backups.DataDir)
		id := r.URL.Query().Get("source")
		if id == "" {
			writeJSON(w, 200, map[string]any{"sources": srcs})
			return
		}
		for _, src := range srcs {
			if src.ID == id {
				n, _ := strconv.Atoi(r.URL.Query().Get("lines"))
				if n <= 0 || n > 2000 {
					n = 300
				}
				q := r.URL.Query().Get("q")
				if len(q) > 100 {
					q = q[:100]
				}
				lines, err := logs.Tail(src, n, q)
				if err != nil {
					fail(w, 500, "can't read that log: "+err.Error())
					return
				}
				writeJSON(w, 200, map[string]any{"sources": srcs, "lines": lines})
				return
			}
		}
		fail(w, 404, "no such log")
	}))
	mux.HandleFunc("POST /api/backups/unpin", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Place int    `json:"place"`
			Name  string `json:"name"`
		}
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		dirs := append([]string{c.Runner.Backups.LocalDir()}, c.Cfg.Backups.Offsite...)
		if in.Place < 0 || in.Place >= len(dirs) {
			fail(w, 400, "no such place")
			return
		}
		err := backup.Unpin(dirs[in.Place], in.Name)
		s.Store.Audit(ses.User.Name, "stop keeping backup", in.Name, "", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/backups/download", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		place, _ := strconv.Atoi(r.URL.Query().Get("place"))
		a, err := c.Runner.FindArchive(place, r.URL.Query().Get("name"))
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		f, err := os.Open(a.Path)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		defer f.Close()
		s.Store.Audit(ses.User.Name, "download backup", a.Name, "", true)
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="`+a.Name+`"`)
		w.Header().Set("X-Checksum-Sha256", a.SHA256)
		http.ServeContent(w, r, a.Name, a.When, f)
	}))
	mux.HandleFunc("POST /api/players/note", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ UserID, Note string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		note := cleanText(in.Note, 300)
		err := s.Store.SetNote(in.UserID, note)
		s.Store.Audit(ses.User.Name, "note", in.UserID, note, err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/backups/preview", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		place, _ := strconv.Atoi(r.URL.Query().Get("place"))
		a, err := c.Runner.FindArchive(place, r.URL.Query().Get("name"))
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		files, err := backup.Contents(a.Path)
		if err != nil {
			fail(w, 500, "can't read that backup: "+err.Error())
			return
		}
		worlds, mods, settings, total := map[string]bool{}, 0, false, int64(0)
		for _, f := range files {
			total += f.Size
			parts := strings.Split(f.Name, "/")
			if len(parts) > 4 && parts[2] == "SaveGames" {
				worlds[parts[4]] = true
			}
			if strings.HasSuffix(f.Name, "PalWorldSettings.ini") {
				settings = true
			}
			if strings.Contains(f.Name, "~mods/") || strings.Contains(f.Name, "ue4ss/Mods/") {
				mods++
			}
		}
		ws := []string{}
		for k := range worlds {
			ws = append(ws, k)
		}
		writeJSON(w, 200, map[string]any{"archive": a, "files": len(files), "bytes": total, "worlds": ws, "settings": settings, "modFiles": mods})
	}))
	mux.HandleFunc("POST /api/backups/restore", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			Place int    `json:"place"`
			Name  string `json:"name"`
		}
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		a, err := c.Runner.FindArchive(in.Place, in.Name)
		if err != nil {
			fail(w, 404, err.Error())
			return
		}
		if busy, _ := c.Runner.Busy(); busy {
			fail(w, 409, "another run is in progress")
			return
		}
		s.Store.Audit(ses.User.Name, "restore backup", a.Name, "", true)
		go c.Runner.Restore(context.Background(), a, ses.User.Name)
		writeJSON(w, 202, map[string]bool{"started": true})
	}))
	mux.HandleFunc("POST /api/backups/now", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		end, err := c.Runner.Begin("a backup")
		if err != nil {
			fail(w, 409, err.Error())
			return
		}
		defer end()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		s.Game.Save(ctx) // a live backup: save the world first (best effort; the server may be off)
		b := *c.Runner.Backups
		b.Cfg.Backups.Image = "off"
		names, err := b.MakeAll(ctx)
		if err == nil && len(c.Cfg.Backups.Offsite)+len(c.Cfg.Backups.Remotes()) > 0 {
			err = b.CopyAndVerify(ctx, names)
		}
		s.Store.Audit(ses.User.Name, "back up now", "", "", err == nil)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "made": names})
	}))
}

// Settings is the owner's view of warden.json. Saving checks it, writes it and restarts Warden
// (Restart is set by main) so every part picks the new values up the same way as at startup.
type Settings struct {
	DataDir string
	Restart func()
}

func (s *Server) settingsRoutes(mux *http.ServeMux) {
	if s.Settings == nil {
		return
	}
	mux.HandleFunc("GET /api/settings", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		cfg, _, err := config.Load(s.Settings.DataDir, "")
		resp := map[string]any{"settings": cfg.Masked(), "path": config.ConfigPath(s.Settings.DataDir)}
		if s.Bot != nil {
			resp["discordBot"] = s.Bot.State()
		}
		if err != nil {
			resp["error"] = err.Error()
		} else if err := cfg.Check(); err != nil {
			resp["error"] = err.Error()
		}
		writeJSON(w, 200, resp)
	}))
	mux.HandleFunc("POST /api/notify/test", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in notify.Config
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		if saved, _, err := config.Load(s.Settings.DataDir, ""); err == nil { // placeholders mean the saved values
			if in.DiscordWebhook == config.Unchanged {
				in.DiscordWebhook = saved.Notify.DiscordWebhook
			}
			if in.NtfyToken == config.Unchanged {
				in.NtfyToken = ""
				if in.NtfyURL == saved.Notify.NtfyURL { // the saved token only goes to the saved address
					in.NtfyToken = saved.Notify.NtfyToken
				}
			}
		}
		if err := in.Check(); err != nil {
			fail(w, 400, err.Error())
			return
		}
		n := notify.New(in, "Warden")
		if !n.Enabled() {
			fail(w, 400, "set a Discord webhook or an ntfy address first")
			return
		}
		n.Send(r.Context(), notify.Info, "", "Test message", "If you can read this, Warden's notifications reach you.")
		s.Store.Audit(ses.User.Name, "send test notification", "", "", true)
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/settings", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var cfg config.Warden
		if readJSON(r, &cfg) != nil {
			fail(w, 400, "bad request")
			return
		}
		if saved, _, err := config.Load(s.Settings.DataDir, ""); err == nil {
			cfg.KeepSecrets(saved)
		}
		if err := cfg.Check(); err != nil {
			fail(w, 400, err.Error())
			return
		}
		if busy := s.Control != nil && func() bool { b, _ := s.Control.Runner.Busy(); return b }(); busy {
			fail(w, 409, "the nightly routine is running; save after it finishes")
			return
		}
		err := config.Save(s.Settings.DataDir, cfg)
		s.Store.Audit(ses.User.Name, "change Warden settings", "", "", err == nil)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true, "restarting": s.Settings.Restart != nil})
		if s.Settings.Restart != nil {
			go func() { time.Sleep(500 * time.Millisecond); s.Settings.Restart() }()
		}
	}))
}

// ---- the game's own settings (PalWorldSettings.ini)

func (s *Server) gameSettingsRoutes(mux *http.ServeMux) {
	c := s.Control
	if c == nil {
		return
	}
	path := config.SettingsPath(c.Cfg.ServerDir)
	view := func(w http.ResponseWriter, r *http.Request) {
		f, err := gamesettings.Read(path)
		if err != nil {
			fail(w, 500, "can't read the game's settings: "+err.Error())
			return
		}
		values, secrets := map[string]string{}, map[string]bool{}
		for k, v := range f.Values {
			if o, ok := gamesettings.Lookup(k); ok && o.Type == "secret" {
				secrets[k] = v != "" // never send a password to the browser
				continue
			}
			values[k] = v
		}
		var unknown []string
		for _, p := range f.Pairs {
			if _, ok := gamesettings.Lookup(p.Key); !ok {
				unknown = append(unknown, p.Key)
			}
		}
		pending := gamesettings.LoadPending(c.Runner.Backups.DataDir)
		for k, v := range pending { // show what the server will have after the next stop
			if o, ok := gamesettings.Lookup(k); ok && o.Type == "secret" {
				pending[k] = "(changed)"
				continue
			}
			values[k] = v
		}
		// Is the file what the server is actually running with? (A world's WorldOption.sav wins
		// over the file for almost everything.)
		var notInEffect []map[string]string
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second) // a frozen game can't slow the page much
		if running, err := s.Game.Settings(ctx); err == nil {
			for k, fv := range f.Values {
				o, ok := gamesettings.Lookup(k)
				rv, has := running[k]
				if !ok || !has || o.Type == "secret" {
					continue
				}
				if _, waiting := pending[k]; waiting {
					continue
				}
				if !gamesettings.Same(k, fmt.Sprint(rv), fv) {
					notInEffect = append(notInEffect, map[string]string{"key": k, "label": o.Label, "file": fv, "running": fmt.Sprint(rv)})
				}
			}
		}
		cancel()
		sort.Slice(notInEffect, func(i, j int) bool { return notInEffect[i]["label"] < notInEffect[j]["label"] })
		writeJSON(w, 200, map[string]any{"catalog": gamesettings.Catalog, "values": values, "secretsSet": secrets, "unknown": unknown, "path": path, "pending": pending, "presets": gamesettings.Presets,
			"notInEffect": notInEffect, "fileProblems": gamesettings.CheckFile(path), "worldOption": worlds.OptionFile(c.Cfg.ServerDir, path) != "", "worldOptionQueued": worlds.SetAsideOptionQueued(c.Runner.Backups.DataDir)})
	}
	mux.HandleFunc("POST /api/game-settings/world-option", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		if worlds.OptionFile(c.Cfg.ServerDir, path) == "" {
			fail(w, 400, "the running world has no WorldOption.sav")
			return
		}
		err := worlds.QueueSetAsideOption(c.Runner.Backups.DataDir)
		s.Store.Audit(ses.User.Name, "set aside WorldOption.sav", "", "at the next stop", err == nil)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/game-settings", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) { view(w, r) }))
	plan := func(r *http.Request) (*gamesettings.File, []gamesettings.Change, string, error) {
		var in struct {
			Values  map[string]string `json:"values"`
			Restart string            `json:"restart"`
		}
		if err := readJSON(r, &in); err != nil {
			return nil, nil, "", err
		}
		f, err := gamesettings.Read(path)
		if err != nil {
			return nil, nil, "", err
		}
		ch, err := f.Plan(in.Values)
		return f, ch, in.Restart, err
	}
	mask := func(ch []gamesettings.Change) []gamesettings.Change {
		out := append(make([]gamesettings.Change, 0, len(ch)), ch...) // [] not null for the panel
		for n, c := range out {
			if o, ok := gamesettings.Lookup(c.Key); ok && o.Type == "secret" {
				out[n].Old, out[n].New = "(hidden)", "(changed)"
				if c.New == "" {
					out[n].New = "(removed)"
				}
			}
		}
		return out
	}
	mux.HandleFunc("POST /api/game-settings/plan", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		_, ch, _, err := plan(r)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"changes": mask(ch)})
	}))
	mux.HandleFunc("POST /api/game-settings/apply", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		f, ch, restart, err := plan(r)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		if len(ch) == 0 {
			fail(w, 400, "nothing changed")
			return
		}
		_ = f
		var names []string
		vals := map[string]string{}
		for _, x := range ch {
			vals[x.Key] = x.New
		}
		for _, x := range mask(ch) {
			names = append(names, x.Label+": "+x.Old+" → "+x.New)
		}
		s.Store.Audit(ses.User.Name, "change game settings", restart, strings.Join(names, "; "), true)
		// Palworld rewrites its settings file when it shuts down, so changes are written only while
		// it's stopped: now if it already is, otherwise by the restart below or the nightly routine.
		if err := gamesettings.AddPending(c.Runner.Backups.DataDir, vals); err != nil {
			fail(w, 500, err.Error())
			return
		}
		if !c.Host.Running() {
			end, err := c.Runner.Begin("a settings change")
			if err != nil {
				writeJSON(w, 200, map[string]any{"ok": true, "changes": mask(ch), "when": "next stop"})
				return
			}
			sum, err := c.Runner.WhileStopped(r.Context())
			end()
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"ok": true, "changes": mask(ch), "when": "now", "done": sum})
			return
		}
		if restart != "now" {
			writeJSON(w, 200, map[string]any{"ok": true, "changes": mask(ch), "when": "next nightly stop"})
			return
		}
		end, err := c.Runner.Begin("a settings change")
		if err != nil {
			writeJSON(w, 200, map[string]any{"ok": true, "changes": mask(ch), "when": "next stop", "note": err.Error()})
			return
		}
		go func() { // stop with a minute's warning, write the settings, start
			defer end()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			s.Game.Announce(ctx, "The server restarts in 1 minute to apply new settings.")
			c.Host.PlanStop(ctx) // on purpose: not a crash
			if err := s.Game.Shutdown(ctx, 60, "Restarting to apply new settings."); err != nil || !c.Host.WaitStopped(ctx, 4*time.Minute) {
				c.Host.ForceStop(ctx)
				c.Host.WaitStopped(ctx, time.Minute)
			}
			time.Sleep(3 * time.Second) // let the game finish writing its own copy first
			sum, err := c.Runner.WhileStopped(ctx)
			if err != nil {
				sum = strings.TrimPrefix(sum+"; "+err.Error(), "; ")
			}
			if sum != "" {
				s.Store.AddEvent(time.Now(), "settings", sum)
			}
			c.Host.Start(ctx)
		}()
		writeJSON(w, 200, map[string]any{"ok": true, "changes": mask(ch), "when": "restarting now"})
	}))
}

// ---- mods

func (s *Server) modRoutes(mux *http.ServeMux) {
	c := s.Control
	if c == nil || c.Runner.Mods == nil {
		return
	}
	m := c.Runner.Mods
	mux.HandleFunc("GET /api/mods", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		list, probs, err := m.List()
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		if list == nil {
			list = []mods.Mod{}
		}
		if probs == nil {
			probs = []mods.Problem{}
		}
		writeJSON(w, 200, map[string]any{"mods": list, "problems": probs, "running": c.Host.Running()})
	}))
	mux.HandleFunc("POST /api/mods/toggle", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ ID, Op string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := m.Queue(in.ID, in.Op)
		s.Store.Audit(ses.User.Name, in.Op+" mod", in.ID, "queued", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		if !c.Host.Running() { // stopped: apply straight away
			end, err := c.Runner.Begin("a mod change")
			if err != nil {
				writeJSON(w, 200, map[string]any{"queued": true, "note": err.Error()})
				return
			}
			done, err := m.ApplyPending()
			end()
			if err != nil {
				fail(w, 500, err.Error())
				return
			}
			writeJSON(w, 200, map[string]any{"applied": done})
			return
		}
		writeJSON(w, 200, map[string]any{"queued": true})
	}))
	mux.HandleFunc("POST /api/mods/install", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		r.Body = http.MaxBytesReader(w, r.Body, mods.MaxZip)
		file, hdr, err := r.FormFile("file")
		if err != nil {
			fail(w, 400, "choose a mod's .zip file (up to 1 GB)")
			return
		}
		defer file.Close()
		tmp, err := os.CreateTemp(m.DataDir, "upload-*.zip")
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		defer os.Remove(tmp.Name())
		_, err = io.Copy(tmp, file)
		tmp.Close()
		if err != nil {
			fail(w, 400, "the upload didn't finish")
			return
		}
		id, err := m.Install(tmp.Name(), hdr.Filename)
		s.Store.Audit(ses.User.Name, "install mod", hdr.Filename, id, err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		if !c.Host.Running() {
			if end, err := c.Runner.Begin("a mod change"); err == nil {
				m.ApplyPending()
				end()
				writeJSON(w, 200, map[string]any{"id": id, "applied": true})
				return
			}
		}
		writeJSON(w, 200, map[string]any{"id": id, "queued": true})
	}))
	mux.HandleFunc("POST /api/mods/apply-now", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		end, err := c.Runner.Begin("a mod change")
		if err != nil {
			fail(w, 409, err.Error())
			return
		}
		s.Store.Audit(ses.User.Name, "apply mod changes now", "", "restart", true)
		go func() {
			defer end()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()
			s.Game.Announce(ctx, "The server restarts in 1 minute to change mods.")
			c.Host.PlanStop(ctx)
			if err := s.Game.Shutdown(ctx, 60, "Restarting to change mods."); err != nil || !c.Host.WaitStopped(ctx, 4*time.Minute) {
				c.Host.ForceStop(ctx)
				c.Host.WaitStopped(ctx, time.Minute)
			}
			done, err := m.ApplyPending()
			s.Store.AddEvent(time.Now(), "mods", fmt.Sprintf("mod changes: %s %v", strings.Join(done, ", "), err))
			c.Host.Start(ctx)
		}()
		writeJSON(w, 202, map[string]bool{"started": true})
	}))
}

// ---- PalDefender

func (s *Server) palDefRoutes(mux *http.ServeMux) {
	c := s.Control
	if c == nil {
		return
	}
	mux.HandleFunc("GET /api/paldefender", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		out := map[string]any{"installed": paldefender.Installed(c.Cfg.ServerDir), "connected": s.PalDef != nil}
		if s.PalDef != nil {
			ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
			defer cancel()
			if v, err := s.PalDef.Version(ctx); err != nil {
				out["error"] = err.Error()
			} else {
				out["version"] = v.VersionLong
			}
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("POST /api/paldefender/connect", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		if s.Settings == nil {
			fail(w, 500, "settings unavailable")
			return
		}
		if busy, what := c.Runner.Busy(); busy {
			fail(w, 409, "busy: "+what+" is in progress; Warden can't restart until it's done")
			return
		}
		pd, err := paldefender.Connect(c.Cfg.ServerDir)
		if err == nil {
			cfg, _, lerr := config.Load(s.Settings.DataDir, "")
			if err = lerr; err == nil {
				cfg.PalDef = pd
				err = config.Save(s.Settings.DataDir, cfg)
			}
		}
		s.Store.Audit(ses.User.Name, "connect PalDefender", "", "REST API on (127.0.0.1), Warden token created", err == nil)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
		if s.Settings.Restart != nil {
			go func() { time.Sleep(500 * time.Millisecond); s.Settings.Restart() }()
		}
	}))
}

// ---- every ban in one place: Warden's, Palworld's own banlist.txt, PalDefender's (users and IPs)

func (s *Server) banRoutes(mux *http.ServeMux) {
	c := s.Control
	if c == nil {
		return
	}
	mux.HandleFunc("GET /api/bans", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		type ban struct {
			Source string `json:"source"`
			ID     string `json:"id"`
			Name   string `json:"name,omitempty"`
			Reason string `json:"reason,omitempty"`
			IP     bool   `json:"ip,omitempty"`
		}
		out := []ban{}
		names := map[string]string{}
		if roster, err := s.Store.Roster(); err == nil {
			for _, p := range roster {
				names[p.UserID] = p.Name
			}
		}
		// Palworld's own list: one user id per line.
		if b, err := os.ReadFile(filepath.Join(c.Cfg.ServerDir, "Pal", "Saved", "SaveGames", "banlist.txt")); err == nil {
			for _, line := range strings.Split(string(b), "\n") {
				if id := strings.TrimSpace(line); id != "" {
					out = append(out, ban{Source: "palworld", ID: id, Name: names[id]})
				}
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		users, ips, _ := paldefender.Banlist(ctx, s.PalDef, c.Cfg.ServerDir)
		pick := func(e map[string]any, keys ...string) string {
			for _, k := range keys {
				if v, ok := e[k]; ok && v != nil {
					return fmt.Sprint(v)
				}
			}
			return ""
		}
		for _, e := range users {
			id := pick(e, "UserId", "UserID", "userId", "SteamId", "Id")
			out = append(out, ban{Source: "paldefender", ID: id, Name: pick(e, "Name", "PlayerName"), Reason: pick(e, "Reason", "reason")})
		}
		for range ips { // IP bans: counted, never shown (the panel never shows IP addresses)
			out = append(out, ban{Source: "paldefender", ID: "(an IP address)", IP: true})
		}
		writeJSON(w, 200, out)
	}))
	mux.HandleFunc("POST /api/bans/lift", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Source, ID string }
		if readJSON(r, &in) != nil || in.ID == "" || len(in.ID) > 100 {
			fail(w, 400, "bad request")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var err error
		switch in.Source {
		case "palworld":
			err = s.Game.Unban(ctx, in.ID)
		case "paldefender":
			if s.PalDef == nil {
				err = errors.New("lifting a PalDefender ban needs PalDefender connected (Server page)")
			} else {
				err = s.PalDef.Unban(ctx, in.ID)
			}
		default:
			err = errors.New("unknown ban list")
		}
		if err == nil && !s.stillBanned(in.ID, c.Cfg.ServerDir) { // another list may still hold them
			s.Store.SetBanned(in.ID, false)
		}
		s.Store.Audit(ses.User.Name, "unban", in.ID, in.Source, err == nil)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
}

// ---- PalDefender's view of guilds and players, and game-master actions (audited)

func (s *Server) palDefDataRoutes(mux *http.ServeMux) {
	need := func(w http.ResponseWriter) bool {
		if s.PalDef == nil {
			fail(w, 409, "this needs PalDefender connected (Server page)")
			return false
		}
		return true
	}
	mux.HandleFunc("GET /api/guilds", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		if !need(w) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()
		gs, err := s.PalDef.Guilds(ctx)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		// When each guild was last played: the latest "last seen" of its members (PalDefender
		// links its PlayerUIDs to user IDs; Warden's roster knows when each was last online).
		uid := map[string]string{}
		if ps, err := s.PalDef.Players(ctx); err == nil {
			for _, p := range ps {
				uid[p.PlayerUID] = p.UserID
			}
		}
		seen := map[string]int64{}
		if ro, err := s.Store.Roster(); err == nil {
			for _, p := range ro {
				seen[p.UserID] = p.LastSeen
			}
		}
		type guildView struct {
			paldefender.Guild
			LastSeen int64 `json:"lastSeen"` // unix seconds; 0 = unknown
		}
		out := make([]guildView, 0, len(gs))
		for _, g := range gs {
			v := guildView{Guild: g}
			for _, m := range g.Members {
				v.LastSeen = max(v.LastSeen, seen[uid[m]])
			}
			v.Members = nil // IDs stay on the server
			out = append(out, v)
		}
		writeJSON(w, 200, out)
	}))
	// Deleting a base: owner only, a fresh backup first (a restore brings it back), audited.
	mux.HandleFunc("POST /api/guilds/delete-base", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		if !need(w) {
			return
		}
		var in struct {
			Camp  string `json:"camp"`
			Guild string `json:"guild"`
		}
		if readJSON(r, &in) != nil || in.Camp == "" || len(in.Camp) > 40 {
			fail(w, 400, "which base?")
			return
		}
		in.Guild = cleanText(in.Guild, 80)
		c := s.Control
		if c == nil || c.Runner == nil || c.Runner.Backups == nil {
			fail(w, 409, "this needs Warden's backups set up, so the base can be brought back")
			return
		}
		end, err := c.Runner.Begin("deleting a base")
		if err != nil {
			fail(w, 409, err.Error())
			return
		}
		defer end()
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		s.Game.Save(ctx)
		b := *c.Runner.Backups
		b.Cfg.Backups.Image = "off"
		names, err := b.MakeAll(ctx)
		if err != nil {
			s.Store.Audit(ses.User.Name, "delete base", in.Guild, "backup failed, base kept", false)
			fail(w, 502, "the backup before deleting failed, so the base was kept: "+err.Error())
			return
		}
		dctx, dcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer dcancel()
		res, err := s.PalDef.DeleteBase(dctx, in.Camp)
		detail := "backup " + strings.Join(names, ", ")
		if res != nil {
			detail = fmt.Sprintf("%d buildings, %d Pals, %d items; %s", res.Deleted.Buildings, res.Deleted.Pals, res.Deleted.ItemCount, detail)
		}
		s.Store.Audit(ses.User.Name, "delete base", in.Guild+" ("+in.Camp+")", detail, err == nil)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "backup": names, "deleted": res.Deleted})
	}))
	mux.HandleFunc("GET /api/players/detail", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		if !need(w) {
			return
		}
		id := r.URL.Query().Get("userId")
		if id == "" || len(id) > 100 {
			fail(w, 400, "which player?")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		writeJSON(w, 200, s.PalDef.Detail(ctx, id))
	}))
	mux.HandleFunc("POST /api/action/give", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			UserID, Name, Kind, ID, Reason string
			Count, Level                   int
		}
		if readJSON(r, &in) != nil || in.UserID == "" || len(in.UserID) > 100 || !idRe.MatchString(in.ID) {
			fail(w, 400, "say which player and which item or Pal (its game ID, e.g. Money or Anubis)")
			return
		}
		if !need(w) {
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		var err error
		var what string
		switch in.Kind {
		case "item":
			if in.Count < 1 || in.Count > 9999 {
				fail(w, 400, "count must be 1 to 9999")
				return
			}
			what = fmt.Sprintf("%d × %s", in.Count, in.ID)
			err = s.PalDef.GiveItems(ctx, in.UserID, in.ID, in.Count)
		case "pal":
			if in.Level < 1 || in.Level > 80 {
				fail(w, 400, "level must be 1 to 80")
				return
			}
			what = fmt.Sprintf("%s level %d", in.ID, in.Level)
			err = s.PalDef.GivePal(ctx, in.UserID, in.ID, in.Level)
		default:
			fail(w, 400, "give an item or a Pal")
			return
		}
		s.Store.Audit(ses.User.Name, "give", cleanText(in.Name, 64)+" ("+cleanText(in.UserID, 100)+")", what+"; "+cleanText(in.Reason, 120), err == nil)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "gave": what})
	}))
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)

// stillBanned says whether the game's own list or PalDefender's still bans a user id.
func (s *Server) stillBanned(id, serverDir string) bool {
	if b, err := os.ReadFile(filepath.Join(serverDir, "Pal", "Saved", "SaveGames", "banlist.txt")); err == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(line) == id {
				return true
			}
		}
	}
	users, _, _ := paldefender.Banlist(context.Background(), nil, serverDir)
	for _, e := range users {
		for _, k := range []string{"UserId", "UserID", "userId", "SteamId", "Id"} {
			if fmt.Sprint(e[k]) == id {
				return true
			}
		}
	}
	return false
}

// ---- the allow-list

func (s *Server) allowRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/allow", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		l, err := s.Store.AllowList()
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		on := s.Control != nil && s.Control.Cfg.AllowList.On
		writeJSON(w, 200, map[string]any{"on": on, "players": l})
	}))
	mux.HandleFunc("POST /api/allow", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct {
			UserID string `json:"userId"`
			On     bool   `json:"on"`
		}
		if readJSON(r, &in) != nil || !idRe2.MatchString(in.UserID) {
			fail(w, 400, "a player ID looks like steam_76561198000000000 (it's on the Players page)")
			return
		}
		err := s.Store.Allow(in.UserID, ses.User.Name, in.On)
		s.Store.Audit(ses.User.Name, map[bool]string{true: "allow", false: "disallow"}[in.On], in.UserID, "", err == nil)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
}

var idRe2 = regexp.MustCompile(`^[A-Za-z0-9_-]{3,100}$`)
