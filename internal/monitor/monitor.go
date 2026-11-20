// Package monitor samples the server once a minute: history for the graphs, and the roster of
// every player ever seen (first and last seen, play time).
package monitor

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/host"
	"github.com/rileydj78/palsforge-warden/internal/palworld"
	"github.com/rileydj78/palsforge-warden/internal/store"
)

// Every is how often the server is sampled.
const Every = time.Minute

// Platform guesses a player's platform from the userId the game reports.
func Platform(userID string) string {
	switch {
	case strings.HasPrefix(userID, "steam_"):
		return "Steam"
	case strings.HasPrefix(userID, "xbox"), strings.HasPrefix(userID, "gdk_"):
		return "Xbox"
	case strings.HasPrefix(userID, "ps5"), strings.HasPrefix(userID, "psn_"):
		return "PlayStation"
	case strings.HasPrefix(userID, "mac"):
		return "Mac"
	}
	return "Other"
}

// SampleOnce takes one sample now.
func SampleOnce(ctx context.Context, game *palworld.Client, st *store.Store) {
	now := time.Now().Unix()
	m, err := game.Metrics(ctx)
	if err != nil {
		ml := host.MachineLoad()
		st.AddSample(store.Sample{TS: now, Online: false, CPU: ml.CPUPercent, Mem: pct(ml.MemUsed, ml.MemTotal), NetIn: ml.NetIn / 1024, NetOut: ml.NetOut / 1024})
		return
	}
	ml := host.MachineLoad()
	st.AddSample(store.Sample{TS: now, Online: true, Players: m.CurrentPlayers, FPS: m.ServerFPS, FrameTime: m.ServerFrameTime, CPU: ml.CPUPercent, Mem: pct(ml.MemUsed, ml.MemTotal), NetIn: ml.NetIn / 1024, NetOut: ml.NetOut / 1024})
	ps, err := game.Players(ctx)
	if err != nil {
		return
	}
	for _, p := range ps {
		if p.UserID == "" {
			continue
		}
		st.SawPlayer(p.UserID, p.PlayerID, p.Name, Platform(p.UserID), p.Level, now, int64(3*Every/time.Second))
	}
}

// Run samples until ctx ends.
func Run(ctx context.Context, game *palworld.Client, st *store.Store) {
	log.Printf("monitor: sampling the server every %s", Every)
	t := time.NewTicker(Every)
	defer t.Stop()
	for {
		c, cancel := context.WithTimeout(ctx, 20*time.Second)
		SampleOnce(c, game, st)
		cancel()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func pct(a, b uint64) float64 {
	if b == 0 {
		return 0
	}
	return 100 * float64(a) / float64(b)
}
