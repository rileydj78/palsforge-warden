package nightly

import (
	"context"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/news"
	"github.com/rileydj78/palsforge-warden/internal/notify"
)

// UpdateSoon looks for a new game build 2 minutes after Warden starts and then every 30 minutes
// (every 5 for 6 hours after Pocketpair announces a patch on Steam), and when one is out runs the
// routine with updates (warnings, save, stop, backup, update, start) instead of waiting for the
// night. Holds while mods are on still apply, and a failed update isn't retried for 6 hours.
func (r *Runner) UpdateSoon(ctx context.Context, feed *news.Feed) {
	if !r.Cfg.Update.Enabled || !r.Cfg.Update.Soon || r.Cfg.Update.SteamCMD == "" {
		return
	}
	var lastTry time.Time
	var lastTried string
	wait := 2 * time.Minute // soon after start: Warden restarts whenever its settings are saved
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		maxAge := 20 * time.Minute
		wait = 30 * time.Minute
		if feed != nil {
			if fresh, err := feed.Fetch(ctx); err == nil {
				for _, it := range fresh {
					if !it.Patch {
						continue
					}
					r.Store.AddEvent(time.Now(), "news", "Pocketpair announced: "+it.Title)
					if r.Notify != nil {
						r.Notify.Send(ctx, notify.Info, "", "Palworld update announced", it.Title+"\n"+it.URL+"\nWarden updates the server as soon as Steam has the new build.")
					}
				}
			}
			if feed.PatchSince(time.Now().Add(-6 * time.Hour)) { // a patch is out or imminent: look often
				wait, maxAge = 5*time.Minute, 4*time.Minute
			}
		}
		if busy, _ := r.Busy(); busy || r.Host.State().PausedTill.After(time.Now()) || !r.Host.State().Wanted {
			continue
		}
		latest, err := r.Host.LatestBuildWithin(ctx, maxAge)
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
