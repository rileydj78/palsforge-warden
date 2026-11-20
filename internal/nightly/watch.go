package nightly

import (
	"context"
	"fmt"
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
			if limit > 0 && !planned {
				if mem := r.Host.Memory(); mem > limit {
					msg := fmt.Sprintf("the server uses %.1f GB of memory (limit %.0f GB)", float64(mem)/(1<<30), r.Cfg.Health.MemoryLimitGB)
					r.Store.AddEvent(time.Now(), "memory", msg+"; restarting with warnings")
					send(notify.Warn, "memory", "Restarting: memory limit", msg+". Players get 10 minutes' warning.")
					r.Restart(ctx, "memory", []int{10, 5, 1})
				}
			}
			continue
		}
		if planned {
			misses = 0
			continue
		}
		misses++
		if misses == 3 && !down {
			down = true
			send(notify.Urgent, "down", "The server isn't answering", "No answer for 3 minutes.")
		}
		if frozen > 0 && misses >= frozen && r.Host.Running() {
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
	r.WhileStopped(ctx)
	err = r.Host.Start(ctx)
	r.Store.AddEvent(time.Now(), "restart", fmt.Sprintf("restarted (%s): %v", why, err))
	return err
}
