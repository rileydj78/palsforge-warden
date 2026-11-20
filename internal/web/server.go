// Package web serves Warden's panel and its JSON API.
//
// It listens on 127.0.0.1 only: local use is the warden.exe window, remote use is through
// `tailscale serve`, which proxies the admin's tailnet to this loopback address.
//
// Every /api route except sign-in and first-run setup needs a session; every change (POST)
// also needs the session's CSRF token in X-CSRF and, when the browser sends one, an Origin that
// matches the Host. Every change is written to the audit log.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/monitor"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/store"
)

//go:embed static
var staticFiles embed.FS

const cookieName = "warden_session"

// Server is the panel.
type Server struct {
	Game    *palworld.Client
	Store   *store.Store
	Auth    *auth.Auth
	Version string
	mu      sync.Mutex
	last    *Status
	lastAt  time.Time
}

// PlayerView is what the panel shows about a player. No IP addresses: the panel never needs them.
type PlayerView struct {
	Name     string  `json:"name"`
	Level    int     `json:"level"`
	Ping     float64 `json:"ping"`
	Platform string  `json:"platform"`
	UserID   string  `json:"userId"`
}

// Status is GET /api/status.
type Status struct {
	Online  bool              `json:"online"`
	Error   string            `json:"error,omitempty"`
	Server  *palworld.Info    `json:"server,omitempty"`
	Metrics *palworld.Metrics `json:"metrics,omitempty"`
	Players []PlayerView      `json:"players"`
	Warden  string            `json:"warden"`
	Checked time.Time         `json:"checked"`
}

func (s *Server) status(ctx context.Context, fresh bool) *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !fresh && s.last != nil && time.Since(s.lastAt) < 3*time.Second { // many tabs, one question
		return s.last
	}
	st := &Status{Warden: s.Version, Checked: time.Now().UTC(), Players: []PlayerView{}}
	info, err := s.Game.Info(ctx)
	if err != nil {
		st.Error = err.Error()
	} else {
		st.Online, st.Server = true, info
		if m, err := s.Game.Metrics(ctx); err == nil {
			st.Metrics = m
		}
		if ps, err := s.Game.Players(ctx); err == nil {
			for _, p := range ps {
				st.Players = append(st.Players, PlayerView{Name: p.Name, Level: p.Level, Ping: p.Ping, Platform: monitor.Platform(p.UserID), UserID: p.UserID})
			}
		}
	}
	s.last, s.lastAt = st, time.Now()
	return st
}

func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; font-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'")
		h.Set("X-Frame-Options", "DENY")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func readJSON(r *http.Request, v any) error {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		return errors.New("expected JSON")
	}
	return json.NewDecoder(http.MaxBytesReader(nil, r.Body, 16<<10)).Decode(v)
}

func sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true // the CSRF token still has to match
	}
	return o == "http://"+r.Host || o == "https://"+r.Host
}

func (s *Server) session(r *http.Request) *auth.Session {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	return s.Auth.Session(c.Value)
}

// guard wraps a route: it needs a session with at least role min; POSTs also need CSRF.
func (s *Server) guard(min string, h func(http.ResponseWriter, *http.Request, *auth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ses := s.session(r)
		if ses == nil {
			fail(w, http.StatusUnauthorized, "please sign in")
			return
		}
		if r.Method == http.MethodPost && (!sameOrigin(r) || r.Header.Get("X-CSRF") == "" || r.Header.Get("X-CSRF") != ses.CSRF) {
			fail(w, http.StatusForbidden, "this request didn't come from the panel; reload the page")
			return
		}
		if !auth.Allows(ses.User.Role, min) {
			fail(w, http.StatusForbidden, "your role can't do that")
			return
		}
		h(w, r, ses)
	}
}

func cleanText(s string, max int) string {
	s = strings.TrimSpace(strings.Map(func(r rune) rune {
		if r < 32 {
			return -1
		}
		return r
	}, s))
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max])
	}
	return s
}

// Handler is the panel's HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServerFS(sub))

	mux.HandleFunc("GET /api/me", func(w http.ResponseWriter, r *http.Request) {
		if ses := s.session(r); ses != nil {
			writeJSON(w, 200, map[string]any{"user": ses.User, "csrf": ses.CSRF})
			return
		}
		writeJSON(w, 200, map[string]any{"user": nil, "setupNeeded": s.Auth.SetupCode != ""})
	})
	mux.HandleFunc("POST /api/setup", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Code, Name, Password string }
		if !sameOrigin(r) || readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		if err := s.Auth.CreateOwner(strings.TrimSpace(in.Code), strings.TrimSpace(in.Name), in.Password); err != nil {
			fail(w, 400, err.Error())
			return
		}
		s.Store.Audit(in.Name, "setup", "", "created the owner account", true)
		s.signIn(w, r, in.Name, in.Password)
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Password string }
		if !sameOrigin(r) || readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		s.signIn(w, r, strings.TrimSpace(in.Name), in.Password)
	})
	mux.HandleFunc("POST /api/logout", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		c, _ := r.Cookie(cookieName)
		s.Auth.Logout(c.Value)
		http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))

	mux.HandleFunc("GET /api/status", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		writeJSON(w, 200, s.status(r.Context(), false))
	}))
	mux.HandleFunc("GET /api/history", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		hours := 24
		if r.URL.Query().Get("range") == "week" {
			hours = 24 * 7
		}
		h, err := s.Store.History(time.Now().Add(-time.Duration(hours) * time.Hour))
		if err != nil {
			fail(w, 500, "history unavailable")
			return
		}
		writeJSON(w, 200, h)
	}))
	mux.HandleFunc("GET /api/roster", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		p, err := s.Store.Roster()
		if err != nil {
			fail(w, 500, "roster unavailable")
			return
		}
		writeJSON(w, 200, p)
	}))
	mux.HandleFunc("GET /api/audit", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		a, err := s.Store.RecentAudit(200)
		if err != nil {
			fail(w, 500, "activity unavailable")
			return
		}
		writeJSON(w, 200, a)
	}))
	mux.HandleFunc("GET /api/users", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		u, err := s.Auth.Users()
		if err != nil {
			fail(w, 500, "unavailable")
			return
		}
		writeJSON(w, 200, u)
	}))
	mux.HandleFunc("POST /api/users", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Name, Password, Role string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := s.Auth.AddUser(strings.TrimSpace(in.Name), in.Password, in.Role)
		s.Store.Audit(ses.User.Name, "add admin", in.Name, in.Role, err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))

	// Actions on the game. Each is audited; the message to players is cleaned and capped.
	type actIn struct {
		UserID  string `json:"userId"`
		Name    string `json:"name"`
		Message string `json:"message"`
		Seconds int    `json:"seconds"`
	}
	act := func(min, name string, do func(context.Context, actIn) error, target func(actIn) string) {
		mux.HandleFunc("POST /api/action/"+name, s.guard(min, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
			var in actIn
			if readJSON(r, &in) != nil {
				fail(w, 400, "bad request")
				return
			}
			in.Message = cleanText(in.Message, 200)
			ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
			defer cancel()
			err := do(ctx, in)
			s.Store.Audit(ses.User.Name, name, target(in), in.Message, err == nil)
			if err != nil {
				fail(w, 502, err.Error())
				return
			}
			s.status(ctx, true)
			writeJSON(w, 200, map[string]bool{"ok": true})
		}))
	}
	who := func(in actIn) string {
		if in.Name != "" {
			return in.Name + " (" + in.UserID + ")"
		}
		return in.UserID
	}
	none := func(actIn) string { return "" }
	needID := func(in actIn) error {
		if in.UserID == "" || len(in.UserID) > 100 {
			return errors.New("which player?")
		}
		return nil
	}
	act(auth.Moderator, "announce", func(ctx context.Context, in actIn) error {
		if in.Message == "" {
			return errors.New("write a message first")
		}
		return s.Game.Announce(ctx, in.Message)
	}, none)
	act(auth.Moderator, "kick", func(ctx context.Context, in actIn) error {
		if err := needID(in); err != nil {
			return err
		}
		return s.Game.Kick(ctx, in.UserID, in.Message)
	}, who)
	act(auth.Admin, "ban", func(ctx context.Context, in actIn) error {
		if err := needID(in); err != nil {
			return err
		}
		err := s.Game.Ban(ctx, in.UserID, in.Message)
		if err == nil {
			s.Store.SetBanned(in.UserID, true)
		}
		return err
	}, who)
	act(auth.Admin, "unban", func(ctx context.Context, in actIn) error {
		if err := needID(in); err != nil {
			return err
		}
		err := s.Game.Unban(ctx, in.UserID)
		if err == nil {
			s.Store.SetBanned(in.UserID, false)
		}
		return err
	}, who)
	act(auth.Admin, "save", func(ctx context.Context, _ actIn) error { return s.Game.Save(ctx) }, none)
	act(auth.Admin, "shutdown", func(ctx context.Context, in actIn) error {
		if in.Seconds < 10 || in.Seconds > 3600 {
			return errors.New("choose a countdown between 10 seconds and an hour")
		}
		if in.Message == "" {
			in.Message = "The server is shutting down."
		}
		return s.Game.Shutdown(ctx, in.Seconds, in.Message)
	}, none)
	return secure(mux)
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request, name, pw string) {
	tok, ses, err := s.Auth.Login(name, pw)
	if err != nil {
		s.Store.Audit(name, "sign in", "", "failed", false)
		fail(w, 401, err.Error())
		return
	}
	secureCookie := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", HttpOnly: true, Secure: secureCookie,
		SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionLife / time.Second)})
	s.Store.Audit(name, "sign in", "", "", true)
	writeJSON(w, 200, map[string]any{"user": ses.User, "csrf": ses.CSRF})
}

// Listen serves on addr, which must be a loopback address.
func (s *Server) Listen(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		log.Fatalf("refusing to listen on %s: Warden listens on 127.0.0.1 only (use tailscale serve for remote access)", addr)
	}
	srv := &http.Server{Addr: addr, Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}
	return srv.ListenAndServe()
}
