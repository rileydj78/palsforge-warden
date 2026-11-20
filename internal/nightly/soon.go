package nightly

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/news"
	"github.com/rileydj78/palsforge-warden/internal/notify"
)

// UpdateSoon looks for a new game build 2 minutes after Warden starts and then every 30 minutes
// (every 5 for 6 hours after Pocketpair announces a patch on Steam), and when one is out updates
// the server instead of waiting for the night:
//   - running: the routine with warnings (save, stop, backup, update, start);
//   - auto-paused (nobody on): updated where it stands, and left paused;
//   - stopped on purpose (by an admin, or outside Warden): left alone.
//
// Holds while mods are on still apply. A build that didn't install is tried again after 6 hours,
// then once a day; that's remembered across restarts, and the admin hears about failures.
func (r *Runner) UpdateSoon(ctx context.Context, feed *news.Feed) {
	if !r.Cfg.Update.Enabled || !r.Cfg.Update.Soon || r.Cfg.Update.SteamCMD == "" {
		return
	}
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
			fresh, err := feed.Fetch(ctx)
			if err != nil && strings.HasPrefix(err.Error(), "can't remember") { // Steam being briefly unreachable isn't news
				r.Store.AddEvent(time.Now(), "news", err.Error())
			}
			{
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
		if busy, _ := r.Busy(); busy || r.Host.State().PausedTill.After(time.Now()) {
			continue
		}
		paused := r.autoPaused.Load()
		if !paused && (!r.Host.State().Wanted || !r.Host.Running()) {
			continue // stopped on purpose: not Warden's to start
		}
		latest, err := r.Host.LatestBuildWithin(ctx, maxAge)
		installed := r.Host.BuildID()
		if err != nil || latest == "" || installed == "" || latest == installed {
			continue
		}
		if r.Cfg.Update.HoldForMods && r.Host.ModsOn != nil && r.Host.ModsOn() && latest != r.Host.AllowedBuild() {
			continue // held: the admin allows the build first
		}
		st := r.loadSoon()
		if st.Build == latest && time.Since(st.At) < st.backoff() {
			continue
		}
		r.Store.AddEvent(time.Now(), "update", "game build "+latest+" is out (this server has "+installed+"): updating now")
		ok := false
		if paused {
			if end, err := r.Begin("an update while paused"); err == nil {
				sum, uerr := r.Host.Update(ctx)
				end()
				ok = uerr == nil && r.Host.BuildID() == latest
				r.Store.AddEvent(time.Now(), "update", "while paused: "+sum+errText(uerr))
			} else {
				continue // someone else holds the server: try next time, nothing was tried
			}
		} else {
			rep, err := r.RunNow(ctx, "update", []int{10, 5, 1}, true)
			if err != nil || rep == nil {
				continue // the lock was taken meanwhile: nothing was tried
			}
			ok = rep.OK && r.Host.BuildID() == latest
		}
		if ok {
			r.saveSoon(soonState{})
			continue
		}
		st = soonState{Build: latest, At: time.Now(), Fails: map[bool]int{true: st.Fails + 1, false: 1}[st.Build == latest]}
		r.saveSoon(st)
		if r.Notify != nil {
			r.Notify.Send(ctx, notify.Warn, "update-soon", "The game update didn't install", "Build "+latest+" didn't install; Warden tries again in "+st.backoff().String()+". The run's report is on the Server page.")
		}
	}
}

type soonState struct {
	Build string    `json:"build"`
	At    time.Time `json:"at"`
	Fails int       `json:"fails"`
}

func (s soonState) backoff() time.Duration {
	if s.Fails >= 2 {
		return 24 * time.Hour
	}
	return 6 * time.Hour
}

func (r *Runner) soonPath() string { return filepath.Join(r.Backups.DataDir, "update-soon.json") }

func (r *Runner) loadSoon() soonState {
	var s soonState
	if b, err := os.ReadFile(r.soonPath()); err == nil {
		json.Unmarshal(b, &s)
	}
	return s
}

func (r *Runner) saveSoon(s soonState) {
	b, _ := json.Marshal(s)
	os.WriteFile(r.soonPath(), b, 0o600)
}

func errText(err error) string {
	if err != nil {
		return " (" + err.Error() + ")"
	}
	return ""
}
