package nightly

import (
	"context"
	"time"
)

// UpdateSoon checks for a new game build every 30 minutes and, when one is out, runs the routine
// with updates (warnings, save, stop, backup, update, start) instead of waiting for the night.
// Holds while mods are on still apply (the routine's update step refuses), and a failed update
// isn't retried for 6 hours.
func (r *Runner) UpdateSoon(ctx context.Context) {
	if !r.Cfg.Update.Enabled || !r.Cfg.Update.Soon || r.Cfg.Update.SteamCMD == "" {
		return
	}
	var lastTry time.Time
	var lastTried string
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Minute):
		}
		if busy, _ := r.Busy(); busy || r.Host.State().PausedTill.After(time.Now()) || !r.Host.State().Wanted {
			continue
		}
		latest, err := r.Host.LatestBuild(ctx)
		installed := r.Host.BuildID()
		if err != nil || latest == "" || installed == "" || latest == installed {
			continue
		}
		if r.Cfg.Update.HoldForMods && r.Host.ModsOn != nil && r.Host.ModsOn() && latest != r.Host.AllowedBuild() {
			continue // held: the admin allows the build first (the routine would refuse anyway)
		}
		if latest == lastTried && time.Since(lastTry) < 6*time.Hour {
			continue // that build didn't install last time; don't restart the server over and over
		}
		lastTried, lastTry = latest, time.Now()
		r.Store.AddEvent(time.Now(), "update", "game build "+latest+" is out (this server has "+installed+"): updating now, with warnings")
		r.RunNow(ctx, "update", []int{10, 5, 1}, true)
	}
}
