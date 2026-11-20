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
		out := map[string]any{"host": c.Host.State(), "build": c.Host.BuildID(), "routineRunning": busy, "memory": c.Host.Memory(), "machine": host.MachineLoad(), "health": c.Cfg.Health, "proof": nightly.LastProof(c.Runner.Backups.DataDir),
			"nightly": c.Cfg.Nightly, "update": c.Cfg.Update.Enabled, "offsite": len(c.Cfg.Backups.Offsite)}
		if c.Cfg.Nightly.Enabled {
			out["nextRun"] = nightly.NextRun(c.Cfg.Nightly.Time, time.Now())
		}
		if len(runs) > 0 {
			out["lastRun"] = runs[0]
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
		writeJSON(w, 200, places)
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
	mux.HandleFunc("GET /api/backups/download", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
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
	mux.HandleFunc("GET /api/backups/preview", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
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
		if err == nil && len(c.Cfg.Backups.Offsite) > 0 {
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
		resp := map[string]any{"settings": cfg, "path": config.ConfigPath(s.Settings.DataDir)}
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
	view := func(w http.ResponseWriter) {
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
		writeJSON(w, 200, map[string]any{"catalog": gamesettings.Catalog, "values": values, "secretsSet": secrets, "unknown": unknown, "path": path, "pending": pending, "presets": gamesettings.Presets})
	}
	mux.HandleFunc("GET /api/game-settings", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) { view(w) }))
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
			s.Store.AddEvent(time.Now(), "settings", fmt.Sprintf("%s %v", sum, err))
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
		if err == nil {
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
		writeJSON(w, 200, gs)
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
		if readJSON(r, &in) != nil || in.UserID == "" || !idRe.MatchString(in.ID) {
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
		s.Store.Audit(ses.User.Name, "give", in.Name+" ("+in.UserID+")", what+"; "+cleanText(in.Reason, 120), err == nil)
		if err != nil {
			fail(w, 502, err.Error())
			return
		}
		writeJSON(w, 200, map[string]any{"ok": true, "gave": what})
	}))
}

var idRe = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
