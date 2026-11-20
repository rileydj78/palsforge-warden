package web

import (
	"context"
	"net/http"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/backup"
	"github.com/rileydj78/palsforge-warden/internal/config"
	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/nightly"
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
		out := map[string]any{"host": c.Host.State(), "build": c.Host.BuildID(), "routineRunning": busy,
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
			a, err := backup.List(d)
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
			if busy, _ := c.Runner.Busy(); busy {
				fail(w, 409, "the nightly routine is running; wait for it to finish")
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			err := f(ctx)
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
	mux.HandleFunc("POST /api/backups/now", s.guard(auth.Admin, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		if busy, _ := c.Runner.Busy(); busy {
			fail(w, 409, "the nightly routine is running")
			return
		}
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
