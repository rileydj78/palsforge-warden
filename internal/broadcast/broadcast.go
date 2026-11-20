// Package broadcast sends the server's own messages: a rotating set of announcements every few
// minutes while anyone is online (Discord, website, rules, events), and a welcome to each player
// who joins (privately through PalDefender when it's connected, otherwise to everyone).
package broadcast

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/palworld"
)

// Config is the "broadcasts" part of warden.json.
type Config struct {
	Enabled      bool     `json:"enabled"`
	EveryMinutes int      `json:"everyMinutes"` // between rotating messages (default 20)
	Messages     []string `json:"messages"`     // sent in turn
	Welcome      string   `json:"welcome"`      // {name} is replaced; empty = no welcome
}

// Game is what the broadcaster needs.
type Game interface {
	Players(ctx context.Context) ([]palworld.Player, error)
	Announce(ctx context.Context, msg string) error
}

// Private sends one player a message (PalDefender); nil when not available.
type Private func(ctx context.Context, userID, msg string) error

// Run sends messages until ctx ends. busy reports when the routine or a restart is running.
func Run(ctx context.Context, cfg Config, g Game, private Private, busy func() bool) {
	if !cfg.Enabled || (len(cfg.Messages) == 0 && cfg.Welcome == "") {
		return
	}
	every := time.Duration(max(5, cfg.EveryMinutes)) * time.Minute
	if cfg.EveryMinutes == 0 {
		every = 20 * time.Minute
	}
	seen := map[string]bool{}
	first := true
	next, idx := time.Now().Add(2*time.Minute), 0
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
		if busy != nil && busy() {
			continue
		}
		c, cancel := context.WithTimeout(ctx, 10*time.Second)
		ps, err := g.Players(c)
		cancel()
		if err != nil {
			first = true // after an outage, don't welcome everyone who was already on
			continue
		}
		now := map[string]bool{}
		for _, p := range ps {
			now[p.UserID] = true
			if !first && !seen[p.UserID] && cfg.Welcome != "" {
				msg := clean(strings.ReplaceAll(cfg.Welcome, "{name}", p.Name))
				c, cancel := context.WithTimeout(ctx, 10*time.Second)
				if private != nil {
					err = private(c, p.UserID, msg)
				}
				if private == nil || err != nil {
					err = g.Announce(c, msg)
				}
				cancel()
				if err != nil {
					log.Printf("broadcast: welcome failed: %v", err)
				}
			}
		}
		seen, first = now, false
		if len(ps) > 0 && len(cfg.Messages) > 0 && time.Now().After(next) {
			c, cancel := context.WithTimeout(ctx, 10*time.Second)
			if err := g.Announce(c, clean(cfg.Messages[idx%len(cfg.Messages)])); err != nil {
				log.Printf("broadcast: %v", err)
			}
			cancel()
			idx++
			next = time.Now().Add(every)
		}
	}
}

// clean keeps a message to one line of sensible length.
func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 200 {
		s = string(r[:200])
	}
	return s
}
