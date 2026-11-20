package nightly

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/rileydj78/palsforge-warden/internal/notify"
)

// Watch keeps an eye on the server once a minute:
//   - tells the admins when it stops answering for 3 minutes and when it's back;
//   - restarts it when the process is still there but hasn't answered for
//     Health.RestartFrozenMinutes (a frozen server never comes back by itself);
//   - restarts it with warnings when it uses more than Health.MemoryLimitGB (Palworld leaks).
//
// It stands aside during the routine, a restore, or a stop an admin asked for.
func (r *Runner) Watch(ctx context.Context) {
	misses, down := 0, false
	frozen := r.Cfg.Health.RestartFrozenMinutes
	limit := int64(r.Cfg.Health.MemoryLimitGB * (1 << 30))
	send := func(lvl notify.Level, key, title, body string) {
		if r.Notify != nil {
			r.Notify.Send(ctx, lvl, key, title, body)
		}
	}
	var autoRestarts []time.Time // automatic restarts in the last 6 hours (capped: then only alerts)
	var lastMemRestart time.Time
	mayRestart := func(why string) bool {
		now := time.Now()
		keep := autoRestarts[:0]
		for _, t := range autoRestarts {
			if now.Sub(t) < 6*time.Hour {
				keep = append(keep, t)
			}
		}
		autoRestarts = keep
		if h := r.Host.State(); h.PausedTill.After(now) {
			return false // the crash-loop pause wins
		}
		if len(autoRestarts) >= 3 {
			send(notify.Urgent, "auto-cap", "Warden stopped restarting the server on its own", "It already restarted it 3 times in 6 hours ("+why+" this time). Have a look at the Server page.")
			return false
		}
		autoRestarts = append(autoRestarts, now)
		return true
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Minute):
		}
		busy, _ := r.Busy()
		planned := busy || !r.Host.State().Wanted
		if r.Game.Alive(ctx) {
			if down {
				send(notify.Info, "", "The server is back", "It is answering again.")
			}
			misses, down = 0, false
			r.checkSaves(ctx, send)
			if limit > 0 && !planned && time.Since(lastMemRestart) > 6*time.Hour {
				if mem := r.Host.Memory(); mem > limit && mayRestart("memory") {
					lastMemRestart = time.Now()
					msg := fmt.Sprintf("the server uses %.1f GB of memory (limit %.0f GB)", float64(mem)/(1<<30), r.Cfg.Health.MemoryLimitGB)
					r.Store.AddEvent(time.Now(), "memory", msg+"; restarting with warnings")
					send(notify.Warn, "memory", "Restarting: memory limit", msg+". Players get 10 minutes' warning.")
					r.Restart(ctx, "memory", []int{10, 5, 1})
				}
			}
			continue
		}
		if planned || !r.Host.Running() { // not running: a crash, which the restart logic handles
			misses = 0
			continue
		}
		misses++
		if misses == 3 && !down {
			down = true
			send(notify.Urgent, "down", "The server isn't answering", "No answer for 3 minutes.")
		}
		if frozen > 0 && misses >= frozen && mayRestart("frozen") {
			r.PinLatest("the last backup before the server froze at " + time.Now().Format("Jan 2 15:04"))
			r.Store.AddEvent(time.Now(), "frozen", fmt.Sprintf("no answer for %d minutes but still running: restarting it", misses))
			send(notify.Urgent, "frozen", "Restarting a frozen server", fmt.Sprintf("It was running but hadn't answered for %d minutes.", misses))
			r.Restart(ctx, "frozen", nil)
			misses = 0
		}
	}
}

// Restart stops the server (with in-game warnings if given, saving first) and starts it again.
// It holds the one-operation lock, so it never overlaps the routine, a restore or a mod change.
func (r *Runner) Restart(ctx context.Context, why string, warn []int) error {
	end, err := r.Begin("a restart (" + why + ")")
	if err != nil {
		return err
	}
	defer end()
	for i, m := range warn {
		r.Game.Announce(ctx, fmt.Sprintf("The server restarts in %d minute%s. The world is saved first.", m, map[bool]string{true: "", false: "s"}[m == 1]))
		next := 0
		if i+1 < len(warn) {
			next = warn[i+1]
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(m-next) * time.Minute):
		}
	}
	r.Host.PlanStop(ctx)
	if len(warn) > 0 {
		r.Game.Save(ctx)
	}
	if err := r.Game.Shutdown(ctx, 10, "Restarting now. Back in a minute."); err != nil || !r.Host.WaitStopped(ctx, 3*time.Minute) {
		r.Host.ForceStop(ctx)
		r.Host.WaitStopped(ctx, time.Minute)
	}
	if sum, werr := r.WhileStopped(ctx); werr != nil {
		r.Store.AddEvent(time.Now(), "settings", fmt.Sprintf("%s; %v", sum, werr))
		if r.Notify != nil {
			r.Notify.Send(ctx, notify.Warn, "", "Some waiting changes weren't applied", werr.Error())
		}
	}
	err = r.Host.Start(ctx)
	r.Store.AddEvent(time.Now(), "restart", fmt.Sprintf("restarted (%s): %v", why, err))
	return err
}

// checkSaves raises the alarm when players are online but the world file hasn't been written for
// 45 minutes: a server can keep running while its saves silently stop, and then a restart loses
// everything since.
func (r *Runner) checkSaves(ctx context.Context, send func(notify.Level, string, string, string)) {
	samples, err := r.Store.History(time.Now().Add(-10 * time.Minute))
	if err != nil || len(samples) == 0 || samples[len(samples)-1].Players == 0 {
		return
	}
	matches, _ := filepath.Glob(filepath.Join(r.Cfg.ServerDir, "Pal", "Saved", "SaveGames", "*", "*", "Level.sav"))
	var newest time.Time
	for _, m := range matches {
		if st, err := os.Stat(m); err == nil && st.ModTime().After(newest) {
			newest = st.ModTime()
		}
	}
	if newest.IsZero() || time.Since(newest) < 45*time.Minute {
		return
	}
	msg := fmt.Sprintf("Players are online but the world hasn't been saved for %d minutes. Warden is asking the server to save now; if that doesn't help, restart it before the world is lost.", int(time.Since(newest).Minutes()))
	r.Store.AddEvent(time.Now(), "saves", msg)
	send(notify.Urgent, "saves", "The world isn't being saved", msg)
	r.Game.Save(ctx)
}
