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

// Seen is told when players join and leave (for notifications); nil to skip.
type Seen func(joined, left []string)

// Gate decides whether a joining player may stay; a non-empty message kicks them with it.
type Gate func(userID, name string) (kickMessage string)

// Kicker removes a player (the REST API's kick).
type Kicker func(ctx context.Context, userID, msg string) error

// Run sends messages until ctx ends. busy reports when the routine or a restart is running.
func Run(ctx context.Context, cfg Config, g Game, private Private, busy func() bool, seenFn Seen, gate Gate, kick Kicker) {
	msgsOn := cfg.Enabled && (len(cfg.Messages) > 0 || cfg.Welcome != "")
	if !msgsOn && seenFn == nil && gate == nil {
		return
	}
	if !cfg.Enabled {
		cfg.Messages, cfg.Welcome = nil, ""
	}
	every := time.Duration(max(5, cfg.EveryMinutes)) * time.Minute
	if cfg.EveryMinutes == 0 {
		every = 20 * time.Minute
	}
	seen := map[string]string{}
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
		now := map[string]string{}
		var joined, left []string
		for _, p := range ps {
			if gate != nil && kick != nil {
				if msg := gate(p.UserID, p.Name); msg != "" { // not on the allow-list: out, at once
					c, cancel := context.WithTimeout(ctx, 10*time.Second)
					if err := kick(c, p.UserID, msg); err != nil {
						log.Printf("allow-list: couldn't kick %s: %v", p.Name, err)
					}
					cancel()
					continue
				}
			}
			now[p.UserID] = p.Name
			if _, was := seen[p.UserID]; !first && !was {
				joined = append(joined, p.Name)
			}
			if _, was := seen[p.UserID]; !first && !was && cfg.Welcome != "" {
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
		if !first {
			for id, name := range seen {
				if _, still := now[id]; !still {
					left = append(left, name)
				}
			}
		}
		if seenFn != nil && (len(joined) > 0 || len(left) > 0) {
			seenFn(joined, left)
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
