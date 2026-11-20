// Package web serves Warden's panel and its JSON API.
// It listens on 127.0.0.1 only: local use is the warden.exe window, remote use is through
// `tailscale serve`, which proxies the tailnet to this loopback address.
package web

import (
	"context"
	"embed"
	"encoding/json"
	"io/fs"
	"log"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/palworld"
)

//go:embed static
var staticFiles embed.FS

// Server is the panel.
type Server struct {
	Game    *palworld.Client
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

func platformOf(userID string) string {
	switch {
	case len(userID) >= 6 && userID[:6] == "steam_":
		return "Steam"
	case len(userID) >= 4 && userID[:4] == "xbox":
		return "Xbox"
	case len(userID) >= 3 && userID[:3] == "ps5":
		return "PS5"
	case len(userID) >= 3 && userID[:3] == "mac":
		return "Mac"
	}
	return "Other"
}

func (s *Server) status(ctx context.Context) *Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.last != nil && time.Since(s.lastAt) < 3*time.Second { // many tabs, one question to the game
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
				st.Players = append(st.Players, PlayerView{Name: p.Name, Level: p.Level, Ping: p.Ping, Platform: platformOf(p.UserID), UserID: p.UserID})
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

// Handler is the panel's HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	sub, _ := fs.Sub(staticFiles, "static")
	mux.Handle("GET /", http.FileServerFS(sub))
	mux.HandleFunc("GET /api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		json.NewEncoder(w).Encode(s.status(r.Context()))
	})
	return secure(mux)
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
