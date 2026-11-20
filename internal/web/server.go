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
	"fmt"
	"io/fs"
	"math"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/rileydj78/palsforge-warden/internal/auth"
	"github.com/rileydj78/palsforge-warden/internal/instances"
	"github.com/rileydj78/palsforge-warden/internal/monitor"
	"github.com/rileydj78/palsforge-warden/internal/paldefender"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/store"
)

//go:embed static
var staticFiles embed.FS

// cookieName is per instance: browsers share cookies across ports, so several Wardens on one
// machine (one per server) would otherwise sign each other out.
func (s *Server) cookieName() string {
	if s.Instances != nil && s.Instances.Self != "" {
		return "warden_session_" + strings.ReplaceAll(s.Instances.Self, "-", "_")
	}
	return "warden_session"
}

// Server is the panel.
type Server struct {
	Game     *palworld.Client
	Store    *store.Store
	Auth     *auth.Auth
	Version  string
	Control  *Control
	Settings *Settings
	PalDef   *paldefender.Client // nil unless PalDefender is connected
	Public   *Public             // the read-only status page; nil when it's off
	// Instances: the other Wardens on this machine (one per game server); nil = don't list.
	Instances *InstancesInfo
	mu        sync.Mutex
	local     *auth.Session // the local port's session: the owner, at the server
	last      *Status
	lastAt    time.Time
}

// PlayerView is what the panel shows about a player. No IP addresses: the panel never needs them.
type PlayerView struct {
	Name     string  `json:"name"`
	Level    int     `json:"level"`
	Ping     float64 `json:"ping"`
	Platform string  `json:"platform"`
	UserID   string  `json:"userId"`
	Guild    string  `json:"guild,omitempty"` // from PalDefender
	MapX     int     `json:"mapX"`            // the coordinates players see on their in-game map
	MapY     int     `json:"mapY"`
}

// mapCoords turns REST world positions into in-game map coordinates (the transform community
// tools use: world units are 1/462.96 of a map unit, with the origin shifted).
func mapCoords(x, y float64) (int, int) {
	return int(math.Round((y - 157664.55791065) / 462.962962963)), int(math.Round((x + 123467.1611767) / 462.962962963))
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
				mx, my := mapCoords(p.X, p.Y)
				st.Players = append(st.Players, PlayerView{Name: p.Name, Level: p.Level, Ping: p.Ping, Platform: monitor.Platform(p.UserID), UserID: p.UserID, MapX: mx, MapY: my})
			}
		}
	}
	if s.PalDef != nil && len(st.Players) > 0 { // PalDefender knows guilds and exact map positions
		pctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if pd, err := s.PalDef.Players(pctx); err == nil {
			by := map[string]paldefender.Player{}
			for _, p := range pd {
				by[p.UserID] = p
			}
			for i, p := range st.Players {
				if d, ok := by[p.UserID]; ok {
					st.Players[i].Guild = d.GuildName
					if d.MapLocation.X != 0 || d.MapLocation.Y != 0 {
						st.Players[i].MapX, st.Players[i].MapY = int(math.Round(d.MapLocation.X)), int(math.Round(d.MapLocation.Y))
					}
				}
			}
		}
		cancel()
	}
	for _, p := range st.Players {
		if p.MapX != 0 || p.MapY != 0 {
			s.Store.SetPosition(p.UserID, p.MapX, p.MapY)
		}
	}
	s.last, s.lastAt = st, time.Now()
	return st
}

func secure(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !hostOK(r.Host) {
			http.Error(w, "Open Warden at http://127.0.0.1 or through your Tailscale name.", http.StatusMisdirectedRequest)
			return
		}
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

type ctxKey int

const localKey ctxKey = 1

// onLocalPort says the request came in on the local port (this machine, no sign-in).
func onLocalPort(r *http.Request) bool { v, _ := r.Context().Value(localKey).(bool); return v }

func (s *Server) localSession() *auth.Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.local == nil {
		s.local = &auth.Session{User: auth.User{Name: "owner (on the server)", Role: auth.Owner}, CSRF: auth.NewToken(24)}
	}
	return s.local
}

func (s *Server) session(r *http.Request) *auth.Session {
	if onLocalPort(r) { // whoever is at the server machine is its rightful operator
		return s.localSession()
	}
	c, err := r.Cookie(s.cookieName())
	if err != nil {
		return nil
	}
	return s.Auth.Session(c.Value)
}

// guard wraps a route: it needs a session with at least role min; POSTs also need CSRF.
func (s *Server) guard(min string, h func(http.ResponseWriter, *http.Request, *auth.Session)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok && !onLocalPort(r) {
			// An API key (bots, websites): no cookie, so no CSRF; never the owner's routes or an
			// account's own pages.
			ses := s.Auth.KeySession(strings.TrimSpace(bearer))
			if ses == nil {
				fail(w, http.StatusUnauthorized, "that API key isn't valid (revoked or mistyped)")
				return
			}
			if min == auth.Owner || strings.HasPrefix(r.URL.Path, "/api/me") || !auth.Allows(ses.User.Role, min) {
				fail(w, http.StatusForbidden, "an API key can't do that")
				return
			}
			h(w, r, ses)
			return
		}
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
			writeJSON(w, 200, map[string]any{"user": ses.User, "csrf": ses.CSRF, "local": onLocalPort(r)})
			return
		}
		n, _ := s.Auth.UserCount()
		writeJSON(w, 200, map[string]any{"user": nil, "accounts": n})
	})
	mux.HandleFunc("POST /api/login", func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Name, Password, Code string }
		if !sameOrigin(r) || readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		s.signIn(w, r, strings.TrimSpace(in.Name), in.Password, strings.TrimSpace(in.Code))
	})
	// Two-step sign-in, for remote accounts (the server machine itself never signs in).
	mux.HandleFunc("POST /api/me/two-step/start", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		if onLocalPort(r) {
			fail(w, 400, "on the server machine there's no account to protect; set this up when signed in remotely")
			return
		}
		key, link, err := s.Auth.StartTOTP(ses.User.Name, "Palsforge Warden")
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"key": key, "link": link})
	}))
	mux.HandleFunc("POST /api/me/two-step/confirm", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Code string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := s.Auth.ConfirmTOTP(ses.User.Name, in.Code)
		s.Store.Audit(ses.User.Name, "turn on two-step sign-in", ses.User.Name, "", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/users/two-step-off", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Name string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := s.Auth.DisableTOTP(in.Name)
		s.Store.Audit(ses.User.Name, "turn off two-step sign-in", in.Name, "", err == nil)
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("POST /api/logout", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		if c, err := r.Cookie(s.cookieName()); err == nil {
			s.Auth.Logout(c.Value)
		}
		http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
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
	mux.HandleFunc("GET /api/positions", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		p, err := s.Store.LastPositions()
		if err != nil {
			fail(w, 500, "positions unavailable")
			return
		}
		writeJSON(w, 200, p)
	}))
	mux.HandleFunc("GET /api/keys", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		k, err := s.Auth.Keys()
		if err != nil {
			fail(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, k)
	}))
	mux.HandleFunc("POST /api/keys", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Name, Role string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		tok, err := s.Auth.NewKey(strings.TrimSpace(in.Name), in.Role, ses.User.Name)
		s.Store.Audit(ses.User.Name, "make API key", in.Name, in.Role, err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]string{"key": tok})
	}))
	mux.HandleFunc("POST /api/keys/revoke", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Name string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := s.Auth.RevokeKey(in.Name)
		s.Store.Audit(ses.User.Name, "revoke API key", in.Name, "", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
	}))
	mux.HandleFunc("GET /api/instances", s.guard(auth.Moderator, func(w http.ResponseWriter, r *http.Request, _ *auth.Session) {
		out := []map[string]any{}
		if s.Instances != nil {
			local := onLocalPort(r)
			for _, e := range instances.List(s.Instances.MainData) {
				m := map[string]any{"name": e.Name, "serverName": e.ServerName, "self": e.Name == s.Instances.Self}
				switch {
				case local: // on the machine: each one's own no-sign-in panel
					m["url"] = "http://" + e.Local + "/"
				case e.Remote != "": // remotely: each one's own sign-in address
					m["url"] = e.Remote
				}
				out = append(out, m)
			}
		}
		writeJSON(w, 200, out)
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
	mux.HandleFunc("POST /api/users/remove", s.guard(auth.Owner, func(w http.ResponseWriter, r *http.Request, ses *auth.Session) {
		var in struct{ Name string }
		if readJSON(r, &in) != nil {
			fail(w, 400, "bad request")
			return
		}
		err := s.Auth.RemoveUser(in.Name)
		s.Store.Audit(ses.User.Name, "remove account", in.Name, "", err == nil)
		if err != nil {
			fail(w, 400, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"ok": true})
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
		s.Store.Audit(ses.User.Name, "add account", in.Name, in.Role, err == nil)
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
		IP      bool   `json:"ip"` // ban: the player's IP address too (needs PalDefender)
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
		var err error
		if in.IP {
			if s.PalDef == nil {
				return errors.New("banning an IP address needs PalDefender connected")
			}
			err = s.PalDef.Ban(ctx, in.UserID, in.Message, true)
		} else {
			err = s.Game.Ban(ctx, in.UserID, in.Message)
		}
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
	act(auth.Moderator, "message", func(ctx context.Context, in actIn) error {
		if err := needID(in); err != nil {
			return err
		}
		if s.PalDef == nil {
			return errors.New("private messages need PalDefender connected")
		}
		if in.Message == "" {
			return errors.New("write a message first")
		}
		return s.PalDef.Message(ctx, in.UserID, in.Message)
	}, who)
	act(auth.Admin, "save", func(ctx context.Context, _ actIn) error { return s.Game.Save(ctx) }, none)
	act(auth.Admin, "shutdown", func(ctx context.Context, in actIn) error {
		if in.Seconds < 10 || in.Seconds > 3600 {
			return errors.New("choose a countdown between 10 seconds and an hour")
		}
		if in.Message == "" {
			in.Message = "The server is shutting down."
		}
		if s.Control != nil {
			s.Control.Host.PlanStop(ctx) // on purpose: not a crash, and it stays off
		}
		return s.Game.Shutdown(ctx, in.Seconds, in.Message)
	}, none)
	s.controlRoutes(mux)
	s.settingsRoutes(mux)
	s.gameSettingsRoutes(mux)
	s.modRoutes(mux)
	s.palDefRoutes(mux)
	s.banRoutes(mux)
	s.palDefDataRoutes(mux)
	s.setupCheckRoutes(mux)
	s.allowRoutes(mux)
	return secure(mux)
}

func (s *Server) signIn(w http.ResponseWriter, r *http.Request, name, pw, code string) {
	tok, ses, err := s.Auth.LoginFrom(name, pw, clientAddr(r), code)
	if errors.Is(err, auth.ErrNeedCode) {
		writeJSON(w, 401, map[string]any{"error": err.Error(), "needCode": true})
		return
	}
	if err != nil {
		s.Store.Audit(name, "sign in", "", "failed", false)
		fail(w, 401, err.Error())
		return
	}
	secureCookie := r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: tok, Path: "/", HttpOnly: true, Secure: secureCookie,
		SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionLife / time.Second)})
	s.Store.Audit(name, "sign in", "", "", true)
	writeJSON(w, 200, map[string]any{"user": ses.User, "csrf": ses.CSRF})
}

// clientAddr is who's asking: the tailnet address tailscale serve passes on, or the direct one.
func clientAddr(r *http.Request) string {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil && ip.IsLoopback() {
		if f := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); f != "" {
			return f
		}
	}
	return host
}

// isLocal says the request comes from the server machine itself: a direct loopback connection,
// addressed to a loopback name, not forwarded by tailscale serve or any other proxy.
func isLocal(r *http.Request) bool {
	host, _, _ := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return false
	}
	for _, h := range []string{"X-Forwarded-For", "X-Forwarded-Host", "Forwarded", "Tailscale-User-Login", "X-Real-Ip"} {
		if r.Header.Get(h) != "" {
			return false
		}
	}
	h := r.Host
	if hh, _, err := net.SplitHostPort(h); err == nil {
		h = hh
	}
	h = strings.Trim(strings.ToLower(h), "[]")
	if h == "localhost" {
		return true
	}
	hip := net.ParseIP(h)
	return hip != nil && hip.IsLoopback()
}

// hostOK refuses requests addressed to any other name (DNS rebinding): only this machine and
// its own Tailscale name may be used.
func hostOK(h string) bool {
	host := h
	if hh, _, err := net.SplitHostPort(h); err == nil {
		host = hh
	}
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "localhost" || strings.HasSuffix(host, ".ts.net") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// localOnly wraps the local port: requests must come straight from this machine (not forwarded by
// tailscale serve or any other proxy), and then they act as the owner without signing in.
func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isLocal(r) {
			http.Error(w, "This port is for the server machine only. Remote admins use the administration port (warden remote on).", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), localKey, true)))
	})
}

func listen(addr string) (net.Listener, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		return nil, fmt.Errorf("refusing to listen on %s: Warden listens on 127.0.0.1 only (remote access goes through tailscale serve)", addr)
	}
	var ln net.Listener
	for i := 0; ; i++ { // after a restart the old process may hold the port for a moment
		if ln, err = net.Listen("tcp", addr); err == nil || i == 20 {
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	return ln, err
}

// Listen serves the panel on two loopback ports: local (this machine, no sign-in, full owner
// control) and admin (what tailscale serve publishes; every request needs a signed-in account).
func (s *Server) Listen(localAddr, adminAddr, publicAddr string) error {
	h := s.Handler()
	errs := make(chan error, 3)
	if publicAddr != "" && s.Public != nil {
		pln, err := listen(publicAddr)
		if err != nil {
			return err
		}
		go func() {
			errs <- (&http.Server{Handler: s.PublicHandler(), ReadHeaderTimeout: 10 * time.Second, WriteTimeout: 30 * time.Second}).Serve(pln)
		}()
	}
	lln, err := listen(localAddr)
	if err != nil {
		return err
	}
	aln, err := listen(adminAddr)
	if err != nil {
		return err
	}
	go func() {
		errs <- (&http.Server{Handler: localOnly(h), ReadHeaderTimeout: 10 * time.Second}).Serve(lln)
	}()
	go func() {
		errs <- (&http.Server{Handler: h, ReadHeaderTimeout: 10 * time.Second}).Serve(aln)
	}()
	return <-errs
}

// InstancesInfo says where the instance list lives and which entry is this Warden.
type InstancesInfo struct {
	MainData string
	Self     string
}
